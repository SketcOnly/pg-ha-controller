package main

import (
	"fmt"
	"github.com/pg-ha-controller/pkg/config"
	"github.com/pg-ha-controller/pkg/logger"
	"go.uber.org/zap"
	"os"
	"os/signal"
	"syscall"
)

func main() {
	
	fmt.Println("Starting Server")
	//	初始化配置
	witchCLi, err := config.LoadConfigWitchCLi("")
	if err != nil {
		fmt.Println("Error loading config:", err)
	}
	
	if err = logger.Init(witchCLi); err != nil {
		fmt.Println("Error initializing logger:", err)
	}
	
	logger.Info("Starting Server")
	
	defer func() {
		err := logger.Sync()
		if err != nil {
			fmt.Println("Error syncing logger:", err)
		}
	}()
	
	logger.Info("pg-ha-controller started",
		zap.String("version", "v1.0.0"),
		zap.String("log_path", witchCLi.Loggerc.Path),
		zap.String("log_format", witchCLi.Loggerc.Format),
	)
	
	// 模拟业务逻辑
	logger.Debug("checking postgres status", zap.String("instance", "pg-1"))
	logger.Warn("postgres connection slow", zap.Float64("latency_ms", 200.5))
	
	//// 捕获退出信号（确保Sync执行）
	sigChan := make(chan os.Signal, 1)
	signal.Notify(sigChan, syscall.SIGINT, syscall.SIGTERM)
	<-sigChan
	
	logger.Info("pg-ha-controller exiting")
	
}
