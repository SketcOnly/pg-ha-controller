package pgmanager

import (
	"context"
	"fmt"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/pg-ha-controller/pkg/config"
	"github.com/pg-ha-controller/pkg/logger"
	"go.uber.org/zap"
	"time"
)

// PGManager PG实例管理器
type PGManager struct {
	cfg    *config.PGConfig
	pool   *pgxpool.Pool
	logger *zap.Logger
}

// NewPGManager 创建PG管理器
func NewPGManager(cfg *config.PGConfig) *PGManager {
	return &PGManager{
		cfg: cfg,
	}
}

// InitConnPool 初始化连接池
func (p *PGManager) InitConnPool() error {
	//	 构建连接字符串
	sprintf := fmt.Sprintf("postgres://%s:%s@%s:%d/postgres?sslmode=disable", p.cfg.User, p.cfg.Password, p.cfg.Host, p.cfg.Port)
	// 配置连接池
	parseConfig, err := pgxpool.ParseConfig(sprintf)
	if err != nil {
		return fmt.Errorf("failed to parse config: %w", err)
	}
	parseConfig.MaxConns = int32(p.cfg.MaxConn)
	parseConfig.HealthCheckPeriod = 5 * time.Second
	parseConfig.ConnConfig.ConnectTimeout = 5 * time.Second
	
	// 创建连接池
	newWithConfig, err := pgxpool.NewWithConfig(context.Background(), parseConfig)
	if err != nil {
		return fmt.Errorf("failed to connect to pgxpool: %w", err)
	}
	// 测试连接池
	timeout, cancelFunc := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancelFunc()
	if err := newWithConfig.Ping(timeout); err != nil {
		return fmt.Errorf("failed to ping pgxpool: %w", err)
	}
	p.pool = newWithConfig
	logger.Info("pg connection pool initialized successfully",
		zap.String("host", p.cfg.Host),
		zap.Int("port", p.cfg.Port),
		zap.Int("max_conn", p.cfg.MaxConn),
	)
	return nil
}

// Close 关闭连接池
func (p *PGManager) Close() {
	if p.pool != nil {
		p.pool.Close()
		logger.Info("pg connection pool closed")
	}
}

// GetPool 获取连接池
func (p *PGManager) GetPool() *pgxpool.Pool {
	return p.pool
}

// Ping 测试PG连接
func (p *PGManager) Ping() error {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	return p.pool.Ping(ctx)
}
