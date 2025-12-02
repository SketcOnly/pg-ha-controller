// logger/logger.go
package logger

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	
	"github.com/pg-ha-controller/pkg/config"
	"go.uber.org/zap"
	"go.uber.org/zap/zapcore"
)

// 全局日志实例：默认空实现（避免未初始化时 panic）
var Logger *zap.Logger = zap.NewNop()

// Init 初始化日志（核心入口，支持控制台/文件二选一输出）
func Init(cfg *config.Config) error {
	// 1. 配置兜底（避免字段为空）
	loggerCfg := cfg.Loggerc
	if loggerCfg.OutputType == "" {
		loggerCfg.OutputType = "console" // 默认控制台输出
	}
	if loggerCfg.Level == "" {
		loggerCfg.Level = "info"
	}
	if loggerCfg.Format == "" {
		loggerCfg.Format = "text"
	}
	
	// 2. 打印配置（排查用，输出到标准错误）
	//fmt.Fprintf(os.Stderr, "===== 日志初始化配置 =====\n")
	//fmt.Fprintf(os.Stderr, "Level:      %s\n", loggerCfg.Level)
	//fmt.Fprintf(os.Stderr, "Format:     %s\n", loggerCfg.Format)
	//fmt.Fprintf(os.Stderr, "OutputType: %s\n", loggerCfg.OutputType)
	//fmt.Fprintf(os.Stderr, "Path:       %s\n", loggerCfg.Path)
	//fmt.Fprintf(os.Stderr, "==========================\n")
	
	// 3. 校验输出类型（仅允许 console/file）
	if loggerCfg.OutputType != "console" && loggerCfg.OutputType != "file" {
		return fmt.Errorf("invalid OutputType: %s (only support 'console'/'file')", loggerCfg.OutputType)
	}
	
	// 4. 校验文件输出的路径（OutputType=file 时必须有有效路径）
	if loggerCfg.OutputType == "file" && loggerCfg.Path == "" {
		return fmt.Errorf("OutputType=file but Path is empty")
	}
	
	// 5. 构建 zap core（核心：区分控制台/文件编码器）
	core, err := buildZapCore(loggerCfg)
	if err != nil {
		return fmt.Errorf("build zap core failed: %w", err)
	}
	
	// 6. 构建日志实例（添加调用者信息、堆栈）
	zapLogger := zap.New(
		core,
		zap.AddCaller(),                             // 添加上下文：调用者文件+行号
		zap.AddCallerSkip(1),                        // 跳过 logger 包内部调用栈
		zap.AddStacktrace(zapcore.ErrorLevel),       // 仅 Error 级别打印堆栈
		zap.ErrorOutput(zapcore.AddSync(os.Stderr)), // 日志自身错误输出到标准错误
	)
	if err != nil {
		return fmt.Errorf("build logger instance failed: %w", err)
	}
	
	// 7. 赋值全局实例
	Logger = zapLogger
	zap.ReplaceGlobals(zapLogger) // 兼容 zap.L() 调用
	
	// 8. 初始化成功日志 + 手动刷盘
	Logger.Info("logger initialized successfully",
		zap.String("output_type", loggerCfg.OutputType),
		zap.String("log_path", loggerCfg.Path),
	)
	if syncErr := Logger.Sync(); syncErr != nil && !isIgnorableSyncErr(syncErr) {
		fmt.Fprintf(os.Stderr, "⚠️ logger sync warning: %v\n", syncErr)
	}
	
	return nil
}

// buildZapCore 构建 zap core（区分控制台/文件编码器，解决乱码）
func buildZapCore(loggerCfg config.LoggerConfig) (zapcore.Core, error) {
	// 1. 解析日志级别（大小写兼容）
	levelStr := strings.ToLower(loggerCfg.Level)
	var zapLevel zapcore.Level
	if err := zapLevel.UnmarshalText([]byte(levelStr)); err != nil {
		return nil, fmt.Errorf("invalid log level '%s': %w", loggerCfg.Level, err)
	}
	levelEnabler := zap.LevelEnablerFunc(func(lvl zapcore.Level) bool {
		return lvl >= zapLevel
	})
	
	// 2. 基础编码器配置（统一时间格式、键名等，避免乱码）
	baseEncoderCfg := zapcore.EncoderConfig{
		TimeKey:        "time",
		LevelKey:       "level",
		NameKey:        "logger",
		CallerKey:      "caller",
		MessageKey:     "msg",
		StacktraceKey:  "stacktrace",
		LineEnding:     zapcore.DefaultLineEnding,
		EncodeTime:     zapcore.ISO8601TimeEncoder, // 统一时间格式（无乱码）
		EncodeDuration: zapcore.StringDurationEncoder,
		EncodeCaller:   zapcore.ShortCallerEncoder, // 简化调用者路径（如 pkg/main.go:12）
	}
	
	// 3. 选择编码器（text/json）
	var encoder zapcore.Encoder
	format := strings.ToLower(loggerCfg.Format)
	switch format {
	case "text", "console":
		// 区分输出类型：控制台带颜色，文件纯文本（无乱码）
		if loggerCfg.OutputType == "console" {
			// 控制台：彩色级别（如 INFO 蓝色）
			baseEncoderCfg.EncodeLevel = zapcore.CapitalColorLevelEncoder
		} else {
			// 文件：纯文本级别（无颜色转义符，解决乱码）
			baseEncoderCfg.EncodeLevel = zapcore.CapitalLevelEncoder
		}
		encoder = zapcore.NewConsoleEncoder(baseEncoderCfg)
	case "json":
		// JSON 格式：无颜色，通用
		baseEncoderCfg.EncodeLevel = zapcore.CapitalLevelEncoder
		encoder = zapcore.NewJSONEncoder(baseEncoderCfg)
	default:
		return nil, fmt.Errorf("invalid format '%s': only support 'text'/'json'", loggerCfg.Format)
	}
	
	// 4. 选择输出目标（控制台/文件）
	var writer zapcore.WriteSyncer
	switch loggerCfg.OutputType {
	case "console":
		// 控制台输出：标准输出
		writer = zapcore.AddSync(os.Stdout)
	case "file":
		// 文件输出：创建目录 + 打开文件（追加写入）
		if err := ensureDir(loggerCfg.Path); err != nil {
			return nil, fmt.Errorf("create log dir failed: %w", err)
		}
		file, err := os.OpenFile(
			loggerCfg.Path,
			os.O_CREATE|os.O_WRONLY|os.O_APPEND, // 追加写入，不覆盖
			0644,                                // 文件权限（安全：仅所有者可写，其他可读）
		)
		if err != nil {
			return nil, fmt.Errorf("open log file failed: %w", err)
		}
		writer = zapcore.AddSync(file)
	}
	
	// 5. 构建 core
	return zapcore.NewCore(encoder, writer, levelEnabler), nil
}

// ensureDir 确保日志文件所在目录存在
func ensureDir(filePath string) error {
	dir := filepath.Dir(filePath)
	if dir == "." { // 无目录（如 ./pg1.log），无需创建
		return nil
	}
	return os.MkdirAll(dir, 0755) // 目录权限：所有者可读写执行，其他可读执行
}

// isIgnorableSyncErr 判断是否为可忽略的刷盘错误（如 Windows 下的 bad file descriptor）
func isIgnorableSyncErr(err error) bool {
	return strings.Contains(err.Error(), "bad file descriptor") ||
		strings.Contains(err.Error(), "sync /dev/stdout: invalid argument")
}

// Sync 刷盘（程序退出时必须调用）
func Sync() error {
	if err := Logger.Sync(); err != nil && !isIgnorableSyncErr(err) {
		return fmt.Errorf("logger sync failed: %w", err)
	}
	return nil
}

// ========== 日志方法封装（兼容原有调用方式） ==========
func Debug(msg string, fields ...zap.Field) {
	Logger.Debug(msg, fields...)
}

func Info(msg string, fields ...zap.Field) {
	Logger.Info(msg, fields...)
}

func Warn(msg string, fields ...zap.Field) {
	Logger.Warn(msg, fields...)
}

func Error(msg string, fields ...zap.Field) {
	Logger.Error(msg, fields...)
}

func Fatal(msg string, fields ...zap.Field) {
	Logger.Fatal(msg, fields...)
}

func Panic(msg string, fields ...zap.Field) {
	Logger.Panic(msg, fields...)
}
