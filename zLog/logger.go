package zLog

import (
	"os"
	"sync"
	"time"

	"go.uber.org/zap"
	"go.uber.org/zap/zapcore"
	"gopkg.in/natefinch/lumberjack.v2"
)

// AsyncWriter 异步日志写入器
// 将日志写入操作异步化，提高日志记录性能
type AsyncWriter struct {
	writer        zapcore.WriteSyncer // 底层写入器
	buffer        chan []byte         // 写入缓冲区
	stopChan      chan struct{}       // 停止信号通道
	flushInterval time.Duration       // 刷新间隔
	wg            sync.WaitGroup      // 等待组
	mu            sync.Mutex          // 互斥锁
	closed        bool                // 是否已关闭
}

// NewAsyncWriter 创建一个新的异步写入器
// 参数:
//   - writer: 底层写入器
//   - bufferSize: 缓冲区大小
//   - flushInterval: 刷新间隔
//
// 返回:
//   - *AsyncWriter: 异步写入器实例
func NewAsyncWriter(writer zapcore.WriteSyncer, bufferSize int, flushInterval time.Duration) *AsyncWriter {
	aw := &AsyncWriter{
		writer:        writer,
		buffer:        make(chan []byte, bufferSize),
		stopChan:      make(chan struct{}),
		flushInterval: flushInterval,
	}
	aw.wg.Add(1)
	go aw.run()
	return aw
}

// run 异步写入循环
// 在独立goroutine中运行，处理写入、定时刷新和停止信号
func (aw *AsyncWriter) run() {
	defer aw.wg.Done()
	ticker := time.NewTicker(aw.flushInterval)
	defer ticker.Stop()

	for {
		select {
		case data := <-aw.buffer:
			aw.write(data)
		case <-ticker.C:
			aw.flushBuffer()
		case <-aw.stopChan:
			aw.flushBuffer()
			return
		}
	}
}

// write 写入数据到底层写入器
// 参数:
//   - data: 要写入的数据
func (aw *AsyncWriter) write(data []byte) {
	aw.mu.Lock()
	defer aw.mu.Unlock()
	_, _ = aw.writer.Write(data)
}

// flushBuffer 刷新缓冲区中所有数据
// 将缓冲区中所有待写入数据写入到底层写入器
func (aw *AsyncWriter) flushBuffer() {
	for {
		select {
		case data := <-aw.buffer:
			aw.write(data)
		default:
			return
		}
	}
}

// Write 实现 io.Writer 接口
// 将数据写入缓冲区，如果缓冲区满则直接写入
//
// 参数:
//   - p: 要写入的数据
//
// 返回:
//   - n: 写入的字节数
//   - error: 写入失败时返回错误
func (aw *AsyncWriter) Write(p []byte) (n int, err error) {
	data := make([]byte, len(p))
	copy(data, p)

	select {
	case aw.buffer <- data:
		return len(p), nil
	default:
		aw.write(data)
		return len(p), nil
	}
}

// Sync 实现 zapcore.WriteSyncer 接口
// 刷新所有缓冲区数据
//
// 返回:
//   - error: 同步失败时返回错误
func (aw *AsyncWriter) Sync() error {
	aw.Flush()
	return nil
}

// Flush 刷新缓冲区
// 将缓冲区中所有待写入数据写入到底层写入器
func (aw *AsyncWriter) Flush() {
	aw.flushBuffer()
}

// Close 关闭异步写入器
// 停止异步写入循环，刷新所有缓冲区，关闭底层写入器
//
// 返回:
//   - error: 关闭失败时返回错误
func (aw *AsyncWriter) Close() error {
	aw.mu.Lock()
	if aw.closed {
		aw.mu.Unlock()
		return nil
	}
	aw.closed = true
	aw.mu.Unlock()

	close(aw.stopChan)
	aw.wg.Wait()
	aw.flushBuffer()
	return aw.writer.Sync()
}

// Logger 标准日志接口
// 定义日志记录的基本方法
type Logger interface {
	Debug(format string, args ...interface{}) // 调试级别日志
	Info(format string, args ...interface{})  // 信息级别日志
	Warn(format string, args ...interface{})  // 警告级别日志
	Error(format string, args ...interface{}) // 错误级别日志
	Fatal(format string, args ...interface{}) // 致命级别日志
}

// Config 日志配置结构体
// 配置日志系统的各项参数
type Config struct {
	// Level 日志级别，可选值：DebugLevel(-1), InfoLevel(0), WarnLevel(1), ErrorLevel(2), DPanicLevel(3), PanicLevel(4), FatalLevel(5)
	Level int `toml:"level" json:"level"`
	// Console 是否输出到控制台
	Console bool `toml:"console" json:"console"`
	// Filename 日志文件路径
	Filename string `toml:"filename" json:"filename"`
	// MaxSize 单个日志文件最大大小（MB），默认1024MB
	MaxSize int `toml:"max-size" json:"max-size"`
	// MaxDays 日志文件最大保留天数
	MaxDays int `toml:"max-days" json:"max-days"`
	// MaxBackups 日志文件最大备份数，默认5个
	MaxBackups int `toml:"max-backups" json:"max-backups"`
	// Compress 是否压缩日志文件
	Compress bool `toml:"compress" json:"compress"`
	// ShowCaller 是否显示调用者信息
	ShowCaller bool `toml:"show-caller" json:"show-caller"`
	// Stacktrace 在什么级别添加堆栈跟踪，可选值同Level，默认ErrorLevel(2)
	Stacktrace int `toml:"stacktrace" json:"stacktrace"`
	// Sampling 是否启用日志采样
	Sampling bool `toml:"sampling" json:"sampling"`
	// SamplingInitial 采样初始数量，默认100
	SamplingInitial int `toml:"sampling-initial" json:"sampling-initial"`
	// SamplingThereafter 采样后续数量，默认10
	SamplingThereafter int `toml:"sampling-thereafter" json:"sampling-thereafter"`
	// Async 是否启用异步写入
	Async bool `toml:"async" json:"async"`
	// AsyncBufferSize 异步写入缓冲区大小，默认1024
	AsyncBufferSize int `toml:"async-buffer-size" json:"async-buffer-size"`
	// AsyncFlushInterval 异步刷新间隔（毫秒），默认100
	AsyncFlushInterval int `toml:"async-flush-interval" json:"async-flush-interval"`
}

// 日志级别常量
const (
	DebugLevel  = iota - 1 // 调试级别
	InfoLevel              // 信息级别
	WarnLevel              // 警告级别
	ErrorLevel             // 错误级别
	DPanicLevel            // 开发模式Panic级别
	PanicLevel             // Panic级别
	FatalLevel             // 致命级别
)

// ZapLoggerAdapter 适配zap.Logger到标准Logger接口
// 将zap.Logger适配为Logger接口，提供格式化日志方法
type ZapLoggerAdapter struct {
	logger *zap.Logger // zap日志器实例
}

// NewZapLoggerAdapter 创建一个新的ZapLoggerAdapter
// 参数:
//   - logger: zap.Logger实例
//
// 返回:
//   - *ZapLoggerAdapter: 适配器实例
func NewZapLoggerAdapter(logger *zap.Logger) *ZapLoggerAdapter {
	return &ZapLoggerAdapter{logger: logger}
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
	level := zap.NewAtomicLevel()
	level.SetLevel(zapcore.Level(cfg.Level))

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

		consoleCore := zapcore.NewCore(zapcore.NewConsoleEncoder(encoderConfig), output, level)
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

		fileCore := zapcore.NewCore(zapcore.NewJSONEncoder(encoderConfig), output, level)
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
