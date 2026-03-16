package zLog

import (
	"io"
	"os"
	"sync"
	"time"

	"go.uber.org/zap"
	"go.uber.org/zap/zapcore"
	"gopkg.in/natefinch/lumberjack.v2"
)

// ZapLoggerAdapter 适配zap.Logger到标准Logger接口
// 将zap.Logger适配为Logger接口，提供格式化日志方法
type ZapLoggerAdapter struct {
	logger *zap.Logger // zap日志器实例
	writer *zapWriter  // io.Writer 适配器
}

// zapWriter 实现 io.Writer 接口，用于集成其他日志框架
type zapWriter struct {
	logger *zap.Logger
}

// Write 实现 io.Writer 接口
// 将写入的数据作为 Info 级别日志记录
func (w *zapWriter) Write(p []byte) (n int, err error) {
	w.logger.Info(string(p))
	return len(p), nil
}

// NewZapLoggerAdapter 创建一个新的ZapLoggerAdapter
// 参数:
//   - logger: zap.Logger实例
//
// 返回:
//   - *ZapLoggerAdapter: 适配器实例
func NewZapLoggerAdapter(logger *zap.Logger) *ZapLoggerAdapter {
	return &ZapLoggerAdapter{
		logger: logger,
		writer: &zapWriter{logger: logger},
	}
}

// LoggerManager 日志管理器
// 管理多个日志器实例，支持异步写入和级别控制
type LoggerManager struct {
	loggers      map[string]*zap.Logger // 日志器映射表
	levelManager zap.AtomicLevel        // 原子级别管理器
	asyncWriters []*AsyncWriter         // 异步写入器列表
	mu           sync.RWMutex           // 读写锁
	asyncMu      sync.Mutex             // 异步写入器锁
}

// registerAsyncWriter 注册异步写入器
// 参数:
//   - writer: 异步写入器实例
func (lm *LoggerManager) registerAsyncWriter(writer *AsyncWriter) {
	lm.asyncMu.Lock()
	defer lm.asyncMu.Unlock()
	lm.asyncWriters = append(lm.asyncWriters, writer)
}

// FlushAll 刷新所有异步写入器
// 将所有异步写入器的缓冲区数据写入到底层
func (lm *LoggerManager) FlushAll() {
	lm.asyncMu.Lock()
	writers := make([]*AsyncWriter, len(lm.asyncWriters))
	copy(writers, lm.asyncWriters)
	lm.asyncMu.Unlock()

	for _, writer := range writers {
		writer.Flush()
	}
}

// CloseAll 关闭所有异步写入器
// 关闭所有异步写入器并等待其完成
//
// 返回:
//   - error: 关闭失败时返回错误
func (lm *LoggerManager) CloseAll() error {
	lm.asyncMu.Lock()
	writers := make([]*AsyncWriter, len(lm.asyncWriters))
	copy(writers, lm.asyncWriters)
	lm.asyncWriters = nil
	lm.asyncMu.Unlock()

	var firstErr error
	for _, writer := range writers {
		if err := writer.Close(); err != nil && firstErr == nil {
			firstErr = err
		}
	}
	return firstErr
}

// 全局日志管理器
var loggerManager *LoggerManager
var once sync.Once

// Debug 记录调试级别的日志
// 参数:
//   - format: 格式化字符串
//   - args: 格式化参数
func (l *ZapLoggerAdapter) Debug(format string, args ...interface{}) {
	l.logger.Sugar().Debugf(format, args...)
}

// Info 记录信息级别的日志
// 参数:
//   - format: 格式化字符串
//   - args: 格式化参数
func (l *ZapLoggerAdapter) Info(format string, args ...interface{}) {
	l.logger.Sugar().Infof(format, args...)
}

// Warn 记录警告级别的日志
// 参数:
//   - format: 格式化字符串
//   - args: 格式化参数
func (l *ZapLoggerAdapter) Warn(format string, args ...interface{}) {
	l.logger.Sugar().Warnf(format, args...)
}

// Error 记录错误级别的日志
// 参数:
//   - format: 格式化字符串
//   - args: 格式化参数
func (l *ZapLoggerAdapter) Error(format string, args ...interface{}) {
	l.logger.Sugar().Errorf(format, args...)
}

// Fatal 记录致命级别的日志
// 参数:
//   - format: 格式化字符串
//   - args: 格式化参数
func (l *ZapLoggerAdapter) Fatal(format string, args ...interface{}) {
	l.logger.Sugar().Fatalf(format, args...)
}

// Writer 获取日志写入器
// 返回一个 io.Writer，用于集成其他日志框架（如 Echo 的 Logger 中间件）
// 返回:
//   - io.Writer: 日志写入器
func (l *ZapLoggerAdapter) Writer() io.Writer {
	return l.writer
}

// GetStandardLogger 获取标准日志接口实例
// 返回:
//   - Logger: 标准日志接口实例
func GetStandardLogger() Logger {
	return NewZapLoggerAdapter(GetLogger())
}

// InitLoggerManager 初始化日志管理器
// 使用sync.Once确保只初始化一次
func InitLoggerManager() {
	once.Do(func() {
		loggerManager = &LoggerManager{
			loggers:      make(map[string]*zap.Logger),
			levelManager: zap.NewAtomicLevel(),
		}
	})
}

// GetLoggerManager 获取日志管理器实例
// 如果管理器未初始化则先初始化
//
// 返回:
//   - *LoggerManager: 日志管理器实例
func GetLoggerManager() *LoggerManager {
	if loggerManager == nil {
		InitLoggerManager()
	}
	return loggerManager
}

// GetLogger 获取指定名称的日志器
// 参数:
//   - name: 日志器名称
//
// 返回:
//   - *zap.Logger: 日志器实例，不存在时返回默认日志器
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
// 参数:
//   - name: 日志器名称
//   - logger: zap.Logger实例
func (lm *LoggerManager) AddLogger(name string, logger *zap.Logger) {
	lm.mu.Lock()
	defer lm.mu.Unlock()

	lm.loggers[name] = logger
}

// SetLevel 设置全局日志级别
// 参数:
//   - level: 日志级别
func (lm *LoggerManager) SetLevel(level int) {
	lm.levelManager.SetLevel(zapcore.Level(level))
}

// GetLevel 获取当前全局日志级别
// 返回:
//   - int: 当前日志级别
func (lm *LoggerManager) GetLevel() int {
	return int(lm.levelManager.Level())
}

// NewLogger 创建一个新的日志器
// 根据配置创建支持控制台输出、文件输出、异步写入等功能的日志器
//
// 参数:
//   - cfg: 日志配置
//   - options: zap选项
//
// 返回:
//   - *zap.Logger: 日志器实例
//   - error: 创建失败时返回错误
func NewLogger(cfg *Config, options ...zap.Option) (*zap.Logger, error) {
	// 文件日志级别
	fileLevel := zap.NewAtomicLevel()
	fileLevel.SetLevel(zapcore.Level(cfg.Level))

	// 控制台日志级别（如果未设置，使用文件日志级别）
	consoleLevel := fileLevel
	if cfg.ConsoleLevel != 0 || cfg.ConsoleLevel == DebugLevel {
		consoleLevel = zap.NewAtomicLevel()
		consoleLevel.SetLevel(zapcore.Level(cfg.ConsoleLevel))
	}

	var cores []zapcore.Core

	// 时间编码器
	timeEncoder := func(t time.Time, enc zapcore.PrimitiveArrayEncoder) {
		enc.AppendString(t.Format("2006-01-02 15:04:05.000"))
	}

	// 控制台输出
	if cfg.Console {
		output := zapcore.Lock(os.Stdout)
		encoderConfig := zap.NewProductionEncoderConfig()
		encoderConfig.EncodeTime = timeEncoder
		encoderConfig.EncodeCaller = zapcore.ShortCallerEncoder

		consoleCore := zapcore.NewCore(zapcore.NewConsoleEncoder(encoderConfig), output, consoleLevel)
		cores = append(cores, consoleCore)
	}

	// 文件输出
	if len(cfg.Filename) > 0 {
		if st, err := os.Stat(cfg.Filename); err == nil {
			if st.IsDir() {
				return nil, err
			}
		}

		// 设置默认值
		if cfg.MaxSize == 0 {
			cfg.MaxSize = 1024 // MB
		}

		if cfg.MaxBackups == 0 {
			cfg.MaxBackups = 5 // 默认保留5个备份
		}

		// 创建日志轮转写入器
		fileWriter := &lumberjack.Logger{
			Filename:   cfg.Filename,
			MaxSize:    cfg.MaxSize,
			MaxAge:     cfg.MaxDays,
			MaxBackups: cfg.MaxBackups,
			Compress:   cfg.Compress,
			LocalTime:  true,
		}

		var output zapcore.WriteSyncer
		if cfg.Async {
			// 异步写入
			bufferSize := cfg.AsyncBufferSize
			if bufferSize <= 0 {
				bufferSize = 1024
			}
			flushInterval := time.Duration(cfg.AsyncFlushInterval) * time.Millisecond
			if flushInterval <= 0 {
				flushInterval = 100 * time.Millisecond
			}
			asyncWriter := NewAsyncWriter(zapcore.AddSync(fileWriter), bufferSize, flushInterval)
			GetLoggerManager().registerAsyncWriter(asyncWriter)
			output = asyncWriter
		} else {
			output = zapcore.AddSync(fileWriter)
		}

		encoderConfig := zap.NewProductionEncoderConfig()
		encoderConfig.EncodeTime = timeEncoder
		encoderConfig.EncodeCaller = zapcore.ShortCallerEncoder

		fileCore := zapcore.NewCore(zapcore.NewJSONEncoder(encoderConfig), output, fileLevel)
		cores = append(cores, fileCore)
	}

	// 根据配置决定是否添加调用者信息
	if cfg.ShowCaller {
		options = append(options, zap.AddCaller())
		options = append(options, zap.AddCallerSkip(1))
	}

	// 根据配置决定在什么级别添加堆栈跟踪
	stacktraceLevel := zapcore.ErrorLevel
	if cfg.Stacktrace >= DebugLevel && cfg.Stacktrace <= FatalLevel {
		stacktraceLevel = zapcore.Level(cfg.Stacktrace)
	}
	options = append(options, zap.AddStacktrace(stacktraceLevel))

	// 根据配置决定是否启用采样
	if cfg.Sampling {
		samplingInitial := 100
		samplingThereafter := 10
		if cfg.SamplingInitial > 0 {
			samplingInitial = cfg.SamplingInitial
		}
		if cfg.SamplingThereafter > 0 {
			samplingThereafter = cfg.SamplingThereafter
		}
		options = append(options, zap.WrapCore(func(c zapcore.Core) zapcore.Core {
			return zapcore.NewSamplerWithOptions(c, time.Second, samplingInitial, samplingThereafter)
		}))
	}

	core := zapcore.NewTee(cores...)

	logger := zap.New(core, options...)

	// 添加到日志管理器
	GetLoggerManager().AddLogger("default", logger)

	return logger, nil
}

// SetGlobalLogLevel 设置全局日志级别
// 参数:
//   - level: 日志级别
func SetGlobalLogLevel(level int) {
	GetLoggerManager().SetLevel(level)
}

// GetGlobalLogLevel 获取当前全局日志级别
// 返回:
//   - int: 当前日志级别
func GetGlobalLogLevel() int {
	return GetLoggerManager().GetLevel()
}

// FlushAll 刷新所有异步写入器的缓冲区
// 全局函数，调用日志管理器的FlushAll
func FlushAll() {
	GetLoggerManager().FlushAll()
}

// CloseAll 关闭所有异步写入器
// 全局函数，调用日志管理器的CloseAll
//
// 返回:
//   - error: 关闭失败时返回错误
func CloseAll() error {
	return GetLoggerManager().CloseAll()
}
