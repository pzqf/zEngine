package zLog

import (
	"os"
	"time"

	"go.uber.org/zap"
	"go.uber.org/zap/zapcore"
	"gopkg.in/natefinch/lumberjack.v2"
)

type Config struct {
	Level    int    `toml:"level" json:"level"`
	Console  bool   `toml:"console" json:"console"`
	Filename string `toml:"filename" json:"filename"`
	MaxSize  int    `toml:"max-size" json:"max-size"`
	MaxDays  int    `toml:"max-days" json:"max-days"`
}

const (
	DebugLevel = iota - 1
	InfoLevel
	WarnLevel
	ErrorLevel
	DPanicLevel
	PanicLevel
	FatalLevel
)

func NewLogger(cfg *Config, options ...zap.Option) (*zap.Logger, error) {
	level := zap.NewAtomicLevel()
	level.SetLevel(zapcore.Level(cfg.Level))

	var cores []zapcore.Core

	timeEncoder := func(t time.Time, enc zapcore.PrimitiveArrayEncoder) {
		enc.AppendString(t.Format("2006-01-02 15:04:05.000"))
	}

	if cfg.Console {
		output := zapcore.Lock(os.Stdout)
		encoderConfig := zap.NewProductionEncoderConfig()
		encoderConfig.EncodeTime = timeEncoder
		encoderConfig.EncodeCaller = zapcore.ShortCallerEncoder

		consoleCore := zapcore.NewCore(zapcore.NewConsoleEncoder(encoderConfig), output, level)

		cores = append(cores, consoleCore)
	}

	if len(cfg.Filename) > 0 {
		if st, err := os.Stat(cfg.Filename); err == nil {
			if st.IsDir() {
				return nil, err
			}
		}

		if cfg.MaxSize == 0 {
			cfg.MaxSize = 1024 //mb
		}

		output := zapcore.AddSync(&lumberjack.Logger{
			Filename:  cfg.Filename,
			MaxSize:   cfg.MaxSize,
			MaxAge:    cfg.MaxDays,
			LocalTime: true,
		})

		encoderConfig := zap.NewProductionEncoderConfig()
		encoderConfig.EncodeTime = timeEncoder
		encoderConfig.EncodeCaller = zapcore.ShortCallerEncoder

		fileCore := zapcore.NewCore(zapcore.NewJSONEncoder(encoderConfig), output, level)
		cores = append(cores, fileCore)
	}

	options = append(options, zap.AddCaller())
	core := zapcore.NewTee(cores...)

	return zap.New(core, options...), nil
}
