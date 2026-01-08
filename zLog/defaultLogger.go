package zLog

import (
	"fmt"
	"os"

	"go.uber.org/zap"
)

var gl *zap.Logger

func InitLogger(cfg *Config, options ...zap.Option) error {
	options = append(options, zap.AddCallerSkip(1))
	var err error
	gl, err = NewLogger(cfg, options...)
	if err != nil {
		return err
	}
	return nil
}

func getDefaultLogger() *zap.Logger {
	if gl == nil {
		cfg := Config{
			Level:    InfoLevel,
			Console:  true,
			Filename: "./logs/log.log",
		}
		if err := InitLogger(&cfg); err != nil {
			// 处理错误，记录到标准错误
			fmt.Fprintf(os.Stderr, "Failed to initialize logger: %v\n", err)
			// 返回一个无操作的logger，避免nil指针
			return zap.NewNop()
		}
	}

	return gl
}

func Debug(msg string, fields ...zap.Field) {
	getDefaultLogger().Debug(msg, fields...)
}

func Info(msg string, fields ...zap.Field) {
	getDefaultLogger().Info(msg, fields...)
}

func Warn(msg string, fields ...zap.Field) {
	getDefaultLogger().Warn(msg, fields...)
}

func Error(msg string, fields ...zap.Field) {
	getDefaultLogger().Error(msg, fields...)
}

func Panic(msg string, fields ...zap.Field) {
	getDefaultLogger().Panic(msg, fields...)
}

func Fatal(msg string, fields ...zap.Field) {
	getDefaultLogger().Fatal(msg, fields...)
}

// GetLogger 获取全局日志记录器实例
func GetLogger() *zap.Logger {
	return getDefaultLogger()
}
