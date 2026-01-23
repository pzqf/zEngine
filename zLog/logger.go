package zLog

import (
	"os"
	"sync"
	"time"

	"go.uber.org/zap"
	"go.uber.org/zap/zapcore"
	"gopkg.in/natefinch/lumberjack.v2"
)

// Logger 标准日志接口
type Logger interface {
	Debug(format string, args ...interface{})
	Info(format string, args ...interface{})
	Warn(format string, args ...interface{})
	Error(format string, args ...interface{})
	Fatal(format string, args ...interface{})
}

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

// ZapLoggerAdapter 适配zap.Logger到标准Logger接口
type ZapLoggerAdapter struct {
	logger *zap.Logger
}

// NewZapLoggerAdapter 创建一个新的ZapLoggerAdapter
func NewZapLoggerAdapter(logger *zap.Logger) *ZapLoggerAdapter {
	return &ZapLoggerAdapter{logger: logger}
}

// LoggerManager 日志管理器
type LoggerManager struct {
	loggers      map[string]*zap.Logger
	levelManager zap.AtomicLevel
	mu           sync.RWMutex
}

// 全局日志管理器
var loggerManager *LoggerManager
var once sync.Once

// Debug 记录调试级别的日志
func (l *ZapLoggerAdapter) Debug(format string, args ...interface{}) {
	l.logger.Sugar().Debugf(format, args...)
}

// Info 记录信息级别的日志
func (l *ZapLoggerAdapter) Info(format string, args ...interface{}) {
	l.logger.Sugar().Infof(format, args...)
}

// Warn 记录警告级别的日志
func (l *ZapLoggerAdapter) Warn(format string, args ...interface{}) {
	l.logger.Sugar().Warnf(format, args...)
}

// Error 记录错误级别的日志
func (l *ZapLoggerAdapter) Error(format string, args ...interface{}) {
	l.logger.Sugar().Errorf(format, args...)
}

// Fatal 记录致命级别的日志
func (l *ZapLoggerAdapter) Fatal(format string, args ...interface{}) {
	l.logger.Sugar().Fatalf(format, args...)
}

// GetStandardLogger 获取标准日志接口实例
func GetStandardLogger() Logger {
	return NewZapLoggerAdapter(GetLogger())
}

// InitLoggerManager 初始化日志管理器
func InitLoggerManager() {
	once.Do(func() {
		loggerManager = &LoggerManager{
			loggers:      make(map[string]*zap.Logger),
			levelManager: zap.NewAtomicLevel(),
		}
	})
}

// GetLoggerManager 获取日志管理器实例
func GetLoggerManager() *LoggerManager {
	if loggerManager == nil {
		InitLoggerManager()
	}
	return loggerManager
}

// GetLogger 获取指定名称的日志器
func (lm *LoggerManager) GetLogger(name string) *zap.Logger {
	lm.mu.RLock()
	defer lm.mu.RUnlock()

	if logger, ok := lm.loggers[name]; ok {
		return logger
	}

	// 如果不存在，返回默认日志器
	return GetLogger()
}

// AddLogger 添加一个新的日志器
func (lm *LoggerManager) AddLogger(name string, logger *zap.Logger) {
	lm.mu.Lock()
	defer lm.mu.Unlock()

	lm.loggers[name] = logger
}

// SetLevel 设置全局日志级别
func (lm *LoggerManager) SetLevel(level int) {
	lm.levelManager.SetLevel(zapcore.Level(level))
}

// GetLevel 获取当前全局日志级别
func (lm *LoggerManager) GetLevel() int {
	return int(lm.levelManager.Level())
}

// NewLogger 创建一个新的日志器
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
	options = append(options, zap.AddCallerSkip(1))
	options = append(options, zap.AddStacktrace(zapcore.ErrorLevel))
	options = append(options, zap.WrapCore(func(c zapcore.Core) zapcore.Core {
		return zapcore.NewSamplerWithOptions(c, time.Second, 100, 10) // 每秒最多采样100条日志，超过后每10条采样1条
	}))

	core := zapcore.NewTee(cores...)

	logger := zap.New(core, options...)

	// 添加到日志管理器
	GetLoggerManager().AddLogger("default", logger)

	return logger, nil
}

// SetGlobalLogLevel 设置全局日志级别
func SetGlobalLogLevel(level int) {
	GetLoggerManager().SetLevel(level)
}

// GetGlobalLogLevel 获取当前全局日志级别
func GetGlobalLogLevel() int {
	return GetLoggerManager().GetLevel()
}
