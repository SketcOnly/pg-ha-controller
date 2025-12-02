package pgmanager

import (
	"context"
	"fmt"
	"github.com/jackc/pgconn"
	"github.com/pg-ha-controller/pkg/logger"
	"go.uber.org/zap"
	"time"
)

// IsAlive 检查PG是否存活
func (p *PGManager) IsAlive() bool {
	if err := p.Ping(); err != nil {
		logger.Error("pg instance is not alive", zap.Error(err))
		return false
	}
	return true
}

// IsReplica 判断是否为从库（recovery模式）
func (p *PGManager) IsReplica() (bool, error) {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	
	var isReplica bool
	err := p.pool.QueryRow(ctx, "SELECT pg_is_in_recovery()").Scan(&isReplica)
	if err != nil {
		return false, fmt.Errorf("check replica status failed: %w", err)
	}
	
	return isReplica, nil
}

// GetReplDelay 获取复制延迟（秒）
func (p *PGManager) GetReplDelay() (time.Duration, error) {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	
	// 从库才会有复制延迟
	isReplica, err := p.IsReplica()
	if err != nil {
		return 0, err
	}
	if !isReplica {
		return 0, nil
	}
	
	// 查询复制延迟
	var delaySeconds float64
	query := `
		SELECT EXTRACT(EPOCH FROM (now() - pg_last_xact_replay_timestamp())) AS repl_delay;
	`
	err = p.pool.QueryRow(ctx, query).Scan(&delaySeconds)
	if err != nil {
		return 0, fmt.Errorf("query repl delay failed: %w", err)
	}
	
	delay := time.Duration(delaySeconds) * time.Second
	logger.Debug("get replication delay", zap.Duration("delay", delay))
	
	return delay, nil
}

// GetPGVersion 获取PG版本
func (p *PGManager) GetPGVersion() (string, error) {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	
	var version string
	err := p.pool.QueryRow(ctx, "SELECT version()").Scan(&version)
	if err != nil {
		return "", fmt.Errorf("get pg version failed: %w", err)
	}
	
	return version, nil
}

// GetLeaderHost 获取主库地址（从库执行）
func (p *PGManager) GetLeaderHost() (string, error) {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	
	isReplica, err := p.IsReplica()
	if err != nil {
		return "", err
	}
	if !isReplica {
		return p.cfg.Host, nil
	}
	
	// 从recovery.conf（PG12+为postgresql.auto.conf）获取主库地址
	var leaderHost string
	query := `
		SELECT setting FROM pg_settings WHERE name = 'primary_conninfo' LIMIT 1;
	`
	err = p.pool.QueryRow(ctx, query).Scan(&leaderHost)
	if err != nil {
		return "", fmt.Errorf("get primary conninfo failed: %w", err)
	}
	
	// 解析primary_conninfo中的主机名（示例：host=pg1 port=5432 user=repluser）
	// 简化解析，实际可使用正则
	if leaderHost != "" {
		parts, err := splitConnInfo(leaderHost)
		if err != nil {
			if host, ok := parts["host"]; ok {
				return host, nil
			}
		}
	}
	
	return "", fmt.Errorf("parse primary host failed: %s", leaderHost)
}

// splitConnInfo 解析primary_conninfo
func splitConnInfo(connInfo string) (map[string]string, error) {
	parts := make(map[string]string)
	// 用pgconn,ParseConfig
	parseConfig, err := pgconn.ParseConfig(connInfo)
	if err != nil {
		return nil, fmt.Errorf("parse config failed: %w", err)
	}
	
	// 从 Config 中提取常用参数
	for k, v := range parseConfig.RuntimeParams {
		parts[k] = v
	}
	return parts, nil
}
