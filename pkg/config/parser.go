package config

import (
	"crypto/tls"
	"encoding/json"
	"flag"
	"os"
	"strings"
)

func LoadFromFile(filePath string, cfg *Config) error {
	file, err := os.ReadFile(filePath)
	if err != nil {
		return err
	}
	return json.Unmarshal(file, cfg)
}

func LoadFromFlags(cfg *Config) error {
	flagSet := flag.NewFlagSet(os.Args[0], flag.ContinueOnError)
	
	// 基础配置
	flagSet.StringVar(&cfg.Serverc.NodeName, "node-name", cfg.Serverc.NodeName, "Node name (pg1/pg2/pg3)")
	flagSet.StringVar(&cfg.Serverc.HTTPAddr, "http-addr", cfg.Serverc.HTTPAddr, "HTTP listen address")
	flagSet.StringVar(&cfg.Serverc.Scope, "scope", cfg.Serverc.Scope, "Cluster scope name")
	flagSet.DurationVar(&cfg.Serverc.CheckInterval, "check-interval", cfg.Serverc.CheckInterval, "Health check interval (e.g. 5s)")
	
	flagSet.StringVar(&cfg.Loggerc.Format, "log-format", cfg.Loggerc.Format, "Log format (default: json)")
	flagSet.StringVar(&cfg.Loggerc.Path, "log-path", cfg.Loggerc.Path, "Log file path (default: stderr)")
	flagSet.StringVar(&cfg.Loggerc.Level, "log-level", cfg.Loggerc.Level, "Log level")
	
	flagSet.StringVar(&cfg.Pgc.Host, "pgc-host", cfg.Pgc.Host, "Host used for pg1")
	flagSet.IntVar(&cfg.Pgc.Port, "pgc-port", cfg.Pgc.Port, "Port used for pg1")
	flagSet.StringVar(&cfg.Pgc.User, "pgc-user", cfg.Pgc.User, "User used for pg1")
	flagSet.StringVar(&cfg.Pgc.Password, "pgc-password", cfg.Pgc.Password, "Password used for pg1")
	flagSet.StringVar(&cfg.Pgc.ReplUser, "pgc-repl_user", cfg.Pgc.ReplUser, "User used for pg1")
	flagSet.StringVar(&cfg.Pgc.ReplPassword, "pgc-repl_password", cfg.Pgc.ReplPassword, "Password used for pg1")
	flagSet.StringVar(&cfg.Pgc.DataDir, "pgc-data-dir", cfg.Pgc.DataDir, "Data directory")
	flagSet.IntVar(&cfg.Pgc.MaxConn, "pgc-max-conn", cfg.Pgc.MaxConn, "Max connections")
	flagSet.StringVar(&cfg.Pgc.ReplSlotName, "slot-name", cfg.Pgc.ReplSlotName, "Repl slot name")
	flagSet.DurationVar(&cfg.Pgc.MaxReplDelay, "max-repl-delay", cfg.Pgc.MaxReplDelay, "Max delay between repl slots")
	
	var etcdEndpoints string
	flagSet.StringVar(&etcdEndpoints, "etcd-endpoints", strings.Join(cfg.Etcdc.Endpoints, ","), "Etcd endpoints (comma separated)")
	flagSet.StringVar(&cfg.Etcdc.CAFile, "etcd-ca-file", cfg.Etcdc.CAFile, "etcd ca file")
	flagSet.StringVar(&cfg.Etcdc.CertFile, "etcd-cert-file", cfg.Etcdc.CertFile, "etcd cert file")
	flagSet.StringVar(&cfg.Etcdc.KeyFile, "etcd-key-file", cfg.Etcdc.KeyFile, "etcd key file")
	flagSet.Int64Var(&cfg.Etcdc.LeaseTTL, "etcd-lease-ttl", cfg.Etcdc.LeaseTTL, "Etcd lease ttl")
	flagSet.DurationVar(&cfg.Etcdc.DialTimeout, "etcd-dial-timeout", cfg.Etcdc.DialTimeout, "Etcd dial timeout (e.g. 5s)")
	
	flagSet.BoolVar(&cfg.Metricsc.Enabled, "metrics-enable", cfg.Metricsc.Enabled, "Metrics enable")
	flagSet.StringVar(&cfg.Metricsc.Addr, "metrics-addr", cfg.Metricsc.Addr, "Metrics listen address")
	
	//	解析命令行
	if err := flagSet.Parse(os.Args[1:]); err != nil {
		return err
	}
	
	//	处理etcd端点，逗号分隔转切片
	if etcdEndpoints != "" {
		cfg.Etcdc.Endpoints = strings.Split(etcdEndpoints, ",")
		for i := range cfg.Etcdc.Endpoints {
			cfg.Etcdc.Endpoints[i] = strings.TrimSpace(cfg.Etcdc.Endpoints[i])
		}
	}
	return nil
	
}

// 初始化TLS配置
func InitTLSConfig(cfg *Config) error {
	if cfg.Etcdc.CAFile != "" || cfg.Etcdc.CertFile != "" || cfg.Etcdc.KeyFile != "" {
		return nil
	}
	loadX509KeyPair, err := tls.LoadX509KeyPair(cfg.Etcdc.CertFile, cfg.Etcdc.KeyFile)
	if err != nil {
		return err
	}
	cfg.TLSc.TLSConfig = &tls.Config{
		Certificates: []tls.Certificate{loadX509KeyPair},
		MinVersion:   tls.VersionTLS12,
	}
	return nil
}
