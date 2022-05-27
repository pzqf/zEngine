package zLog

import (
	"go.uber.org/zap"
)

var gl *zap.Logger

func InitLogger(cfg *Config, options ...zap.Option) error {
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
		_ = InitLogger(&cfg)
	}

	return gl
}

func Debug(msg string, fields ...zap.Field) {
	getDefaultLogger().WithOptions(zap.AddCallerSkip(1)).Debug(msg, fields...)
}

func Info(msg string, fields ...zap.Field) {
	getDefaultLogger().WithOptions(zap.AddCallerSkip(1)).Info(msg, fields...)
}

func Warn(msg string, fields ...zap.Field) {
	getDefaultLogger().WithOptions(zap.AddCallerSkip(1)).Warn(msg, fields...)
}

func Error(msg string, fields ...zap.Field) {
	getDefaultLogger().WithOptions(zap.AddCallerSkip(1)).Error(msg, fields...)
}

func Panic(msg string, fields ...zap.Field) {
	getDefaultLogger().WithOptions(zap.AddCallerSkip(1)).Panic(msg, fields...)
}

func Fatal(msg string, fields ...zap.Field) {
	getDefaultLogger().WithOptions(zap.AddCallerSkip(1)).Fatal(msg, fields...)
}
