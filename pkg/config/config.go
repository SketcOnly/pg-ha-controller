package config

import (
	"crypto/tls"
	"fmt"
	"github.com/caarlos0/env/v10"
	"time"
)

// Config 全局配置
type Config struct {
	Serverc  ServerConfig  `yaml:"serverc" mapstructure:"serverc" comment:"serverc服务器配置"`
	Loggerc  LoggerConfig  `yaml:"logc" mapstructure:"logc" comment:"logc日志配置"`
	TLSc     TLSConfig     `yaml:"tlsc" mapstructure:"tlsc" comment:"tlsc配置"`
	Pgc      PGConfig      `yaml:"pgc" mapstructure:"pgc" comment:"pgc配置"`
	Etcdc    EtcdConfig    `yaml:"etcdc" mapstructure:"etcdc" comment:"etcdc配置"`
	Metricsc MetricsConfig `yaml:"metricsc" mapstructure:"metricsc" comment:"metricsc配置"`
}

type ServerConfig struct {
	NodeName      string        `yaml:"node_name" mapstructure:"node_name" env:"NODE_NAME" flag:"node-name"`                     // 节点名（pg1/pg2/pg3）
	HTTPAddr      string        `yaml:"http_addr" mapstructure:"http_addr" env:"HTTP_ADDR" flag:"http-addr"`                     // HTTP API 监听地址（0.0.0.0:8080）
	Scope         string        `yaml:"scope" mapstructure:"scope" env:"SCOPE" flag:"scope"`                                     // 集群作用域（pg-ha-cluster）
	CheckInterval time.Duration `yaml:"check_interval" mapstructure:"check_interval" env:"CHECK_INTERVAL" flag:"check-interval"` // 健康检测间隔
}

type LoggerConfig struct {
	Level      string `yaml:"level" mapstructure:"level" env:"LOG_LEVEL" flag:"log-level"` // 日志级别（debug/info/warn/error）
	Format     string `yaml:"format" mapstructure:"format" env:"LOG_FORMAT" flag:"log-format"`
	Path       string `yaml:"path" mapstructure:"path" env:"LOG_PATH" comment:"日志存储路径" flag:"log-path"`
	OutputType string `yaml:"output_type" mapstructure:"output_type" env:"LOG_OUTPUT_TYPE" flag:"log-output-type" comment:"输出类型：console（控制台，带颜色）/file（文件，纯文本无乱码），二选一"`
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
	TLSc        TLSConfig
}

// 监控配置
type MetricsConfig struct {
	Enabled bool   `yaml:"enabled" env:"METRICS_ENABLED" flag:"metrics-enabled"` // 是否开启监控
	Addr    string `yaml:"addr" env:"METRICS_ADDR" flag:"metrics-addr"`          // 监控指标监听地址（0.0.0.0:9090）
}

// DefaultConfig 默认配置
func DefaultConfig() *Config {
	cfg := &Config{
		Serverc: ServerConfig{
			NodeName:      "pg1",
			HTTPAddr:      "0.0.0.0:8080",
			Scope:         "pg-ha-Cluster",
			CheckInterval: 5 * time.Second,
		},
		Loggerc: LoggerConfig{
			Level:      "debug",
			Format:     "text",
			OutputType: "console", // 默认控制台输出
			Path:       "./pg-ha-controller.log",
		},
		TLSc: TLSConfig{
			TLSConfig: nil,
		},
		Pgc: PGConfig{
			Host:         "localhost",
			Port:         5432,
			User:         "postgres",
			Password:     "postgres",
			ReplUser:     "postgres",
			ReplPassword: "postgres",
			DataDir:      "/var/lib/postgresql/data",
			MaxConn:      10,
			ReplSlotName: "pg_ha_repl_slot",
			MaxReplDelay: 30 * time.Second,
		},
		Etcdc: EtcdConfig{
			Endpoints:   []string{"http://127.0.0.1:2379"},
			CAFile:      "ca.pem",
			CertFile:    "cert.pem",
			KeyFile:     "key.pem",
			LeaseTTL:    10,
			DialTimeout: 5 * time.Second,
		},
		Metricsc: MetricsConfig{
			Enabled: true,
			Addr:    "0.0.0.0:8080",
		},
	}
	return cfg
}

// LoadConfigWitchCLi 加载配置，(优先级: 命令行 > 环境变量 > 配置文件 > default)
func LoadConfigWitchCLi(configFile string) (*Config, error) {
	// 1，加载默认配置
	cfg := DefaultConfig()
	// 2. 从配置文件加载
	if configFile != "" {
		if err := LoadFromFile(configFile, cfg); err != nil {
			return nil, fmt.Errorf("failed to load config from file: %w", err)
		}
	}
	//	3,加载环境变量
	if err := env.Parse(cfg); err != nil {
		return nil, fmt.Errorf("failed to parse config: %w", err)
	}
	//	4,加载命令行参数
	if err := LoadFromFlags(cfg); err != nil {
		return nil, fmt.Errorf("failed to parse config: %w", err)
	}
	//	5,初始化TLS配置(etcd)
	if err := InitTLSConfig(cfg); err != nil {
		return nil, fmt.Errorf("failed to init tls config: %w", err)
	}
	////	6,初始化日志
	//initLogger, err := logger.InitLogger(cfg)
	//if err != nil {
	//	return nil, fmt.Errorf("failed to init initLogger: %w", err)
	//}
	//cfg.Loggerc.Logger = initLogger
	return cfg, nil
}
