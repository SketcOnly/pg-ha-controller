package etcdclient

import (
	"context"
	"errors"
	"fmt"
	"github.com/pg-ha-controller/pkg/logger"
	clientv3 "go.etcd.io/etcd/client/v3"
	"go.etcd.io/etcd/client/v3/concurrency"
	"go.uber.org/zap"
	"math"
	"strings"
	"time"
)

// NewLeaderElection 创建Leader选举实例（优化点：补全默认配置，修复命名错误）
func NewLeaderElection(cfg *LeaderElectionConfig, client *Client) (*LeaderElection, error) {
	if cfg == nil {
		return nil, errors.New("leader election config is nil")
	}
	if client == nil {
		return nil, errors.New("etcd client is nil")
	}
	if cfg.Scope == "" {
		return nil, errors.New("leader election scope is empty")
	}
	if cfg.NodeName == "" {
		return nil, errors.New("node name is empty")
	}
	
	// 设置默认配置
	if cfg.CheckInterval <= 0 {
		cfg.CheckInterval = 2 * time.Second
	}
	if cfg.RetryInterval <= 0 {
		cfg.RetryInterval = time.Second
	}
	if cfg.MaxRetryBackoff <= 0 {
		cfg.MaxRetryBackoff = 10 * time.Second
	}
	
	// 创建分布式锁（路径规范：/scope/leader）
	mutex := concurrency.NewMutex(client.GetSession(), fmt.Sprintf("/%s/leader", cfg.Scope))
	
	return &LeaderElection{
		cfg:        cfg,
		client:     client,
		mutex:      mutex,
		LeaderChan: make(chan bool, 2), // 缓冲2个事件，避免阻塞
		stopChan:   make(chan struct{}),
	}, nil
}

// Start 启动Leader选举（优化点：支持优雅退出，指数退避重试，并发安全）
func (le *LeaderElection) Start() error {
	logger.Info("starting leader election",
		zap.String("scope", le.cfg.Scope),
		zap.String("node", le.cfg.NodeName))
	
	// 先更新当前节点状态为healthy
	if err := le.UpdateNodeStatus("healthy"); err != nil {
		logger.Warn("update node status failed before election",
			zap.String("node", le.cfg.NodeName), zap.Error(err))
	}
	
	// 选举循环（支持优雅退出）
	retryCount := 0
	for {
		select {
		case <-le.stopChan:
			logger.Info("leader election stopped gracefully", zap.String("node", le.cfg.NodeName))
			le.releaseLeader(context.Background())
			return nil
		default:
			// 尝试获取锁
			ctx, cancel := context.WithTimeout(context.Background(), le.cfg.CheckInterval)
			err := le.mutex.Lock(ctx)
			cancel()
			
			if err != nil {
				// 指数退避重试（避免频繁重试）
				backoff := le.calcBackoff(retryCount)
				logger.Error("failed to acquire leader lock, retrying...",
					zap.String("node", le.cfg.NodeName),
					zap.Error(err),
					zap.Duration("backoff", backoff))
				
				time.Sleep(backoff)
				retryCount++
				continue
			}
			
			// 成功获取锁，成为Leader
			retryCount = 0 // 重置重试计数
			le.setIsLeader(true)
			le.LeaderChan <- true
			logger.Info("successfully acquired leader lock",
				zap.String("scope", le.cfg.Scope),
				zap.String("node", le.cfg.NodeName))
			
			// 存储Leader信息到etcd
			if err := le.setLeader(context.Background(), le.cfg.NodeName); err != nil {
				logger.Error("failed to set leader info", zap.Error(err))
			}
			
			// 持有锁直到会话失效或主动退出
			select {
			case <-le.stopChan:
				le.releaseLeader(context.Background())
				return nil
			case <-le.client.GetSession().Done():
				logger.Warn("etcd session expired, losing leader lock", zap.String("node", le.cfg.NodeName))
				le.releaseLeader(context.Background())
				le.setIsLeader(false)
				le.LeaderChan <- false
				
				// 重建会话和锁
				if err := le.client.initLeaseAndSession(); err != nil {
					logger.Error("failed to recreate lease/session", zap.Error(err))
					time.Sleep(le.cfg.RetryInterval)
				}
				le.mutex = concurrency.NewMutex(le.client.GetSession(), fmt.Sprintf("/%s/leader", le.cfg.Scope))
			}
		}
	}
}

// Stop 优雅停止Leader选举（新增：支持主动退出）
func (le *LeaderElection) Stop() {
	close(le.stopChan)
	// 关闭LeaderChan避免阻塞
	close(le.LeaderChan)
}

// IsLeader 判断当前是否为Leader（加读锁保护）
func (le *LeaderElection) IsLeader() bool {
	le.mu.RLock()
	defer le.mu.RUnlock()
	return le.isLeader
}

// setIsLeader 设置Leader状态（加写锁保护）
func (le *LeaderElection) setIsLeader(isLeader bool) {
	le.mu.Lock()
	defer le.mu.Unlock()
	le.isLeader = isLeader
}

// releaseLeader 释放Leader锁（优化点：幂等操作，并发安全）
func (le *LeaderElection) releaseLeader(ctx context.Context) {
	if !le.IsLeader() {
		return
	}
	
	// 解锁
	if err := le.mutex.Unlock(ctx); err != nil {
		logger.Error("failed to release leader lock",
			zap.String("node", le.cfg.NodeName), zap.Error(err))
	}
	
	// 清空Leader信息
	if err := le.setLeader(ctx, ""); err != nil {
		logger.Error("failed to clear leader info", zap.Error(err))
	}
	
	// 更新节点状态为unhealthy
	_ = le.UpdateNodeStatus("unhealthy")
	
	// 更新状态并通知
	le.setIsLeader(false)
	le.LeaderChan <- false
	logger.Info("leader lock released", zap.String("node", le.cfg.NodeName))
}

// ForceSwitchover 强制切换Leader（优化点：更严谨的目标节点检查，超时控制）
func (le *LeaderElection) ForceSwitchover(targetNode string) error {
	if targetNode == "" {
		return errors.New("target node is empty")
	}
	if targetNode == le.cfg.NodeName {
		logger.Info("target node is current node, no need to switchover", zap.String("node", targetNode))
		return nil
	}
	
	logger.Info("starting force leader switchover",
		zap.String("current_node", le.cfg.NodeName),
		zap.String("target_node", targetNode),
		zap.String("scope", le.cfg.Scope))
	
	// 检查目标节点状态
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	
	nodeStatus, err := le.getNodeStatus(ctx, targetNode)
	if err != nil {
		return fmt.Errorf("check target node status failed: %w", err)
	}
	if nodeStatus != "healthy" {
		return fmt.Errorf("target node %s status is %s (expected healthy)", targetNode, nodeStatus)
	}
	
	// 当前节点是Leader，主动释放锁
	if le.IsLeader() {
		le.releaseLeader(ctx)
	}
	
	// 等待目标节点成为Leader
	timeoutCtx, timeoutCancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer timeoutCancel()
	
	ticker := time.NewTicker(le.cfg.CheckInterval)
	defer ticker.Stop()
	
	for {
		select {
		case <-timeoutCtx.Done():
			return fmt.Errorf("switchover timeout: target node %s did not become leader within 30s", targetNode)
		case <-ticker.C:
			currentLeader, err := le.GetCurrentLeader()
			if err != nil {
				logger.Error("get current leader failed during switchover", zap.Error(err))
				continue
			}
			if currentLeader == targetNode {
				logger.Info("force switchover success",
					zap.String("new_leader", targetNode),
					zap.String("scope", le.cfg.Scope))
				return nil
			}
		}
	}
}

// --------------------------- 辅助方法 ---------------------------

// calcBackoff 计算指数退避时间（避免频繁重试）
func (le *LeaderElection) calcBackoff(retryCount int) time.Duration {
	if retryCount == 0 {
		return le.cfg.RetryInterval
	}
	// 指数退避公式：backoff = min(retryInterval * 2^retryCount, maxBackoff)
	backoff := le.cfg.RetryInterval * time.Duration(math.Pow(2, float64(retryCount)))
	if backoff > le.cfg.MaxRetryBackoff {
		backoff = le.cfg.MaxRetryBackoff
	}
	return backoff
}

// setLeader 存储/清空Leader信息（封装重复逻辑）
func (le *LeaderElection) setLeader(ctx context.Context, nodeName string) error {
	key := fmt.Sprintf("/%s/leader/info", le.cfg.Scope)
	cli := le.client.GetClient()
	
	if nodeName == "" {
		_, err := cli.Delete(ctx, key)
		return wrapError("delete leader info", err)
	}
	
	_, err := cli.Put(ctx, key, nodeName, clientv3.WithLease(le.client.GetLeaseID()))
	return wrapError("put leader info", err)
}

// GetCurrentLeader 获取当前Leader节点（优化点：封装错误，超时控制）
func (le *LeaderElection) GetCurrentLeader() (string, error) {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	
	key := fmt.Sprintf("/%s/leader/info", le.cfg.Scope)
	resp, err := le.client.GetClient().Get(ctx, key)
	if err != nil {
		return "", wrapError("get leader info", err)
	}
	
	if len(resp.Kvs) == 0 {
		return "", nil
	}
	
	return string(resp.Kvs[0].Value), nil
}

// getNodeStatus 获取节点状态（封装错误）
func (le *LeaderElection) getNodeStatus(ctx context.Context, nodeName string) (string, error) {
	key := fmt.Sprintf("/%s/nodes/%s", le.cfg.Scope, nodeName)
	resp, err := le.client.GetClient().Get(ctx, key)
	if err != nil {
		return "", wrapError("get node status", err)
	}
	
	if len(resp.Kvs) == 0 {
		return "", fmt.Errorf("node %s status not found in scope %s", nodeName, le.cfg.Scope)
	}
	
	return string(resp.Kvs[0].Value), nil
}

// UpdateNodeStatus 更新节点状态到etcd（优化点：封装错误，带租约）
func (le *LeaderElection) UpdateNodeStatus(status string) error {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	
	key := fmt.Sprintf("/%s/nodes/%s", le.cfg.Scope, le.cfg.NodeName)
	_, err := le.client.GetClient().Put(ctx, key, status, clientv3.WithLease(le.client.GetLeaseID()))
	return wrapError("update node status", err)
}

// GetAllNodes 获取集群所有节点（优化点：去重，错误封装）
func (le *LeaderElection) GetAllNodes() ([]string, error) {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	
	prefix := fmt.Sprintf("/%s/nodes/", le.cfg.Scope)
	resp, err := le.client.GetClient().Get(ctx, prefix, clientv3.WithPrefix())
	if err != nil {
		return nil, wrapError("get all nodes", err)
	}
	
	nodeSet := make(map[string]struct{})
	for _, kv := range resp.Kvs {
		node := strings.TrimPrefix(string(kv.Key), prefix)
		if node != "" {
			nodeSet[node] = struct{}{}
		}
	}
	
	// 转换为切片
	nodes := make([]string, 0, len(nodeSet))
	for node := range nodeSet {
		nodes = append(nodes, node)
	}
	
	return nodes, nil
}

// wrapError 统一错误包装（优化点：标准化错误信息）
func wrapError(operation string, err error) error {
	if err == nil {
		return nil
	}
	return fmt.Errorf("%s failed: %w", operation, err)
}

// GetLeaderChan 获取Leader状态变更通道（返回只读通道）
func (le *LeaderElection) GetLeaderChan() <-chan bool {
	return le.LeaderChan
}
