package zLog

import "io"

// Logger 标准日志接口
// 定义日志记录的基本方法
type Logger interface {
	Debug(format string, args ...interface{}) // 调试级别日志
	Info(format string, args ...interface{})  // 信息级别日志
	Warn(format string, args ...interface{})  // 警告级别日志
	Error(format string, args ...interface{}) // 错误级别日志
	Fatal(format string, args ...interface{}) // 致命级别日志
	Writer() io.Writer                        // 获取日志写入器，用于集成其他日志框架
}
