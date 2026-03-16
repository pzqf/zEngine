package zLog

// Config 日志配置结构体
// 配置日志系统的各项参数
type Config struct {
	// Level 日志级别，可选值：DebugLevel(-1), InfoLevel(0), WarnLevel(1), ErrorLevel(2), DPanicLevel(3), PanicLevel(4), FatalLevel(5)
	Level int `toml:"level" json:"level"`
	// Console 是否输出到控制台
	Console bool `toml:"console" json:"console"`
	// ConsoleLevel 控制台日志级别，可选值同 Level，默认使用 Level 的值
	// 如果设置，控制台将使用此级别，实现控制台和文件日志级别分离
	ConsoleLevel int `toml:"console-level" json:"console-level"`
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
