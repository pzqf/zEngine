package zLog

// SetLogger
// start a default logger
/* config string :
	"level": log output level, 1:debug, 2:information, 3:warning, 4:error, default 1
	"log_dir": log file storage direction, like "./logs"
	"log_file_prefix": log file prefix, default application name
	"console": log output to console, default true
	"caller_depth": caller depth, default 2
	"msg_chan_len": message channel size, default 512
	"daily": log file output daily, default true
	"max_line": per file max line, default 100000000
	"max_size": per file max size, default 2147483648
	"output_json": log output format to json, default false
}`,

ps: must be call Close
*/
func SetLogger(confStr string) {
	if confStr == "" {
		confStr = "{}"
	}
	DefaultLogger = NewLogger(confStr)
}

func Close() {
	if DefaultLogger != nil {
		DefaultLogger.Close()
	}
}

var DefaultLogger *Logger

func Debug(msg string) {
	DefaultLogger.Debug(msg)
}

func Info(msg string) {
	DefaultLogger.Info(msg)
}

func Warning(msg string) {
	DefaultLogger.Warning(msg)
}

func Error(msg string) {
	DefaultLogger.Error(msg)
}
