package config

import (
	"crypto/tls"
	"time"

	"go.uber.org/zap"
)

// Config 全局配置
type Config struct {
	Server ServerConfig `yaml:"server" mapstructure:"server" comment:"server服务器配置"`
	logger LoggerConfig `yaml:"logger" mapstructure:"logger" comment:"logger日志配置"`
}

type ServerConfig struct {
	NodeName      string        `yaml:"node_name" env:"NODE_NAME" flag:"node-name"`                // 节点名（pg1/pg2/pg3）
	LogLevel      string        `yaml:"log_level" env:"LOG_LEVEL" flag:"log-level"`                // 日志级别（debug/info/warn/error）
	HTTPAddr      string        `yaml:"http_addr" env:"HTTP_ADDR" flag:"http-addr"`                // HTTP API 监听地址（0.0.0.0:8080）
	Scope         string        `yaml:"scope" env:"SCOPE" flag:"scope"`                            // 集群作用域（pg-ha-cluster）
	CheckInterval time.Duration `yaml:"check_interval" env:"CHECK_INTERVAL" flag:"check-interval"` // 健康检测间隔
}

type LoggerConfig struct {
	Logger *zap.Logger `yaml:"-"` // 日志实例
}

type TLSConfig struct {
	TLSConfig *tls.Config `yaml:"-"` // etcd TLS 配置
}

// PostgreSQL 配置
type PGConfig struct {
	Host         string        `yaml:"host" env:"PG_HOST" flag:"pg-host"`                         // PG 容器/主机名
	Port         int           `yaml:"port" env:"PG_PORT" flag:"pg-port"`                         // PG 端口（默认5432）
	User         string        `yaml:"user" env:"PG_USER" flag:"pg-user"`                         // 超级用户
	Password     string        `yaml:"password" env:"PG_PASSWORD" flag:"pg-password"`             // 超级用户密码
	ReplUser     string        `yaml:"repl_user" env:"REPL_USER" flag:"repl-user"`                // 复制用户
	ReplPassword string        `yaml:"repl_password" env:"REPL_PASSWORD" flag:"repl-password"`    // 复制密码
	DataDir      string        `yaml:"data_dir" env:"PG_DATA_DIR" flag:"pg-data-dir"`             // PG 数据目录
	MaxConn      int           `yaml:"max_conn" env:"PG_MAX_CONN" flag:"pg-max-conn"`             // PG 连接池大小
	ReplSlotName string        `yaml:"repl_slot_name" env:"REPL_SLOT_NAME" flag:"repl-slot-name"` // 复制槽名
	MaxReplDelay time.Duration `yaml:"max_repl_delay" env:"MAX_REPL_DELAY" flag:"max-repl-delay"` // 最大复制延迟（触发告警）
}

// Etcd 配置
type EtcdConfig struct {
	Endpoints   []string      `yaml:"endpoints" env:"ETCD_ENDPOINTS" flag:"etcd-endpoints"`          // etcd 端点（逗号分隔）
	CAFile      string        `yaml:"ca_file" env:"ETCD_CA_FILE" flag:"etcd-ca-file"`                // CA 证书路径
	CertFile    string        `yaml:"cert_file" env:"ETCD_CERT_FILE" flag:"etcd-cert-file"`          // 客户端证书
	KeyFile     string        `yaml:"key_file" env:"ETCD_KEY_FILE" flag:"etcd-key-file"`             // 客户端私钥
	LeaseTTL    int64         `yaml:"lease_ttl" env:"ETCD_LEASE_TTL" flag:"etcd-lease-ttl"`          // 租约TTL（秒）
	DialTimeout time.Duration `yaml:"dial_timeout" env:"ETCD_DIAL_TIMEOUT" flag:"etcd-dial-timeout"` // 连接超时
}

// 监控配置
type MetricsConfig struct {
	Enabled bool   `yaml:"enabled" env:"METRICS_ENABLED" flag:"metrics-enabled"` // 是否开启监控
	Addr    string `yaml:"addr" env:"METRICS_ADDR" flag:"metrics-addr"`          // 监控指标监听地址（0.0.0.0:9090）
}
