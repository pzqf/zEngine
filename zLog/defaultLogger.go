package zLog

import (
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
		_ = InitLogger(&cfg)
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
