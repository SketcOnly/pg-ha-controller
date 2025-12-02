package etcdclient

import (
	"context"
	"errors"
	"fmt"
	"github.com/pg-ha-controller/pkg/config"
	"github.com/pg-ha-controller/pkg/logger"
	clientv3 "go.etcd.io/etcd/client/v3"
	"go.etcd.io/etcd/client/v3/concurrency"
	"go.uber.org/zap"
	"time"
)

// NewClient 创建Etcd客户端（优化点：改用KeepAlive通道替代轮询，增加资源安全管理）
func NewClient(cfg *config.EtcdConfig) (*Client, error) {
	if cfg == nil {
		return nil, errors.New("etcd config is nil")
	}
	if len(cfg.Endpoints) == 0 {
		return nil, errors.New("etcd endpoints is empty")
	}
	if cfg.LeaseTTL <= 0 {
		cfg.LeaseTTL = 30 // 默认30s租约
	}
	
	// 构建Etcd客户端配置（简化默认值，只设置必要参数）
	cliCfg := clientv3.Config{
		Endpoints:   cfg.Endpoints,
		DialTimeout: cfg.DialTimeout,
		TLS:         cfg.TLSc.TLSConfig,
	}
	
	// 创建原始etcd客户端
	cli, err := clientv3.New(cliCfg)
	if err != nil {
		return nil, fmt.Errorf("create etcd client failed: %w", err)
	}
	
	// 初始化租约上下文（核心修正：正确创建上下文和取消函数）
	leaseCtx, cancelFunc := context.WithCancel(context.Background())
	client := &Client{
		cfg:         cfg,
		cli:         cli,
		leaseCtx:    leaseCtx,
		leaseCancel: cancelFunc, // 正确赋值取消函数
	}
	
	// 初始化租约和会话
	if err := client.initLeaseAndSession(); err != nil {
		// 失败时关闭客户端，避免资源泄漏
		err := cli.Close()
		if err != nil {
			return nil, err
		}
		client.leaseCancel() // 释放上下文
		return nil, fmt.Errorf("init lease/session failed: %w", err)
	}
	
	// 启动租约监控（改用etcd原生KeepAlive通道，更高效）
	go client.watchLeaseLoop()
	
	logger.Info("etcd client initialized successfully",
		zap.String("endpoints", fmt.Sprintf("%v", cfg.Endpoints)),
		zap.Int64("lease_ttl", cfg.LeaseTTL),
		zap.Int64("lease_id", int64(client.leaseID)),
	)
	
	return client, nil
}

// initLeaseAndSession 初始化/重建租约和会话（抽离复用，增加锁保护）
func (c *Client) initLeaseAndSession() error {
	c.mu.Lock()
	defer c.mu.Unlock()
	
	// 关闭旧的租约续期和会话（避免资源泄漏）
	if c.leaseCancel != nil {
		c.leaseCancel() // 取消旧上下文
		//	重新创建上下文，重建上下文时重新生成cancel函数
		newCtx, newCancelFunc := context.WithCancel(context.Background())
		c.leaseCtx = newCtx
		c.leaseCancel = newCancelFunc
	}
	if c.session != nil {
		_ = c.session.Close()
	}
	
	// 创建新租约
	grant, err := c.cli.Grant(c.leaseCtx, c.cfg.LeaseTTL)
	if err != nil {
		return fmt.Errorf("grant lease failed: %w", err)
	}
	
	// 创建会话（自动续期租约）
	session, err := concurrency.NewSession(c.cli, concurrency.WithLease(grant.ID))
	if err != nil {
		return fmt.Errorf("create session failed: %w", err)
	}
	
	// 启动租约续期（获取原生续期通道）
	keepAliveCh, err := c.cli.KeepAlive(c.leaseCtx, grant.ID)
	if err != nil {
		_ = session.Close() // 失败时关闭会话
		return fmt.Errorf("start lease keepalive failed: %w", err)
	}
	
	// 更新客户端状态
	c.leaseID = grant.ID
	c.session = session
	c.keepAliveCh = keepAliveCh
	
	return nil
}

// watchLeaseLoop 租约监控循环（优化点：基于通道的事件驱动，替代轮询）
func (c *Client) watchLeaseLoop() {
	for {
		select {
		case resp, ok := <-c.keepAliveCh:
			if !ok || resp == nil {
				// 租约续期失败，重建租约和会话
				logger.Error("lease keepalive channel closed, recreating lease...",
					zap.Int64("lease_id", int64(c.leaseID)))
				
				// 指数退避重试重建
				backoff := time.Second
				maxBackoff := time.Duration(c.cfg.LeaseTTL) * time.Second
				for {
					if err := c.initLeaseAndSession(); err != nil {
						logger.Error("recreate lease failed, retrying...", zap.Error(err), zap.Duration("backoff", backoff))
						break
					}
					logger.Info("lease recreated successfully", zap.Int64("new_lease_id", int64(c.leaseID)))
					time.Sleep(backoff)
					
					// 指数退避（最多不超过租约TTL）
					backoff *= 2
					if backoff > maxBackoff {
						backoff = maxBackoff
					}
				}
			} else {
				logger.Debug("lease keepalive success",
					zap.Int64("lease_id", int64(c.leaseID)),
					zap.Int64("ttl", resp.TTL))
			}
		case <-c.leaseCtx.Done():
			logger.Warn("lease watch context canceled, exiting lease loop")
			return
		}
	}
}

// Close 关闭Etcd客户端（新增：资源优雅释放）
func (c *Client) Close() error {
	c.mu.Lock()
	defer c.mu.Unlock()
	
	// 关闭租约续期
	if c.leaseCancel != nil {
		c.leaseCancel()
	}
	
	// 关闭会话
	if c.session != nil {
		_ = c.session.Close()
	}
	
	// 关闭原始客户端
	if c.cli != nil {
		return c.cli.Close()
	}
	
	return nil
}

// GetClient 获取原始etcd客户端（加读锁保护）
func (c *Client) GetClient() *clientv3.Client {
	c.mu.RLock()
	defer c.mu.RUnlock()
	return c.cli
}

// GetSession 获取etcd会话（加读锁保护）
func (c *Client) GetSession() *concurrency.Session {
	c.mu.RLock()
	defer c.mu.RUnlock()
	return c.session
}

// GetLeaseID 获取租约ID（加读锁保护）
func (c *Client) GetLeaseID() clientv3.LeaseID {
	c.mu.RLock()
	defer c.mu.RUnlock()
	return c.leaseID
}
