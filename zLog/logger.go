package zLog

import (
	"bufio"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log"
	"os"
	"path/filepath"
	"runtime"
	"sync"
	"time"
)

type LogLevel int

const (
	LevelNil     LogLevel = 0
	LevelDebug   LogLevel = 1
	LevelInfo    LogLevel = 2
	LevelWarning LogLevel = 3
	LevelError   LogLevel = 4
)

var mapLevelStr = map[LogLevel]string{
	LevelNil:     "Nil",
	LevelDebug:   "Debug",
	LevelInfo:    "Info",
	LevelWarning: "Warn",
	LevelError:   "Error",
}

type LogMessage struct {
	Level string `json:"level"`
	Msg   string `json:"message"`
	When  string `json:"time"`
	File  string `json:"file"`
	Line  int    `json:"line"`
}

type Logger struct {
	msgChan    chan *LogMessage
	logMsgPool *sync.Pool
	config     *LogConfig
	fileIndex  int
	exitChan   chan bool
	wg         sync.WaitGroup
	running    bool
}

// NewLogger create a new custom logger,
/* "config string" such as SetLogger
ps: must be call Close
*/
func NewLogger(confStr string) *Logger {
	config := &LogConfig{
		Level:       0,
		LogDir:      "",
		Console:     true,
		CallerDepth: 3,
		MsgChanLen:  409600,
		Daily:       true,
		MaxLine:     100000000,
		MaxSize:     1 << 31,
		OutputJson:  false,
	}

	if confStr != "" {
		err := json.Unmarshal([]byte(confStr), config)
		if err != nil {
			panic(err)
			return nil
		}
	}

	if config.LogDir != "" {
		_, err := os.Stat(config.LogDir)
		if err != nil && os.IsNotExist(err) {
			fmt.Println("create log directory", config.LogDir)
			err = os.MkdirAll(config.LogDir, os.ModePerm)
			if err != nil {
				panic(fmt.Sprintf("create log directory error %v", err))
				return nil
			}
		}
	}

	newLogger := Logger{
		config:  config,
		msgChan: make(chan *LogMessage, config.MsgChanLen),
		logMsgPool: &sync.Pool{
			New: func() interface{} {
				return &LogMessage{}
			},
		},
		fileIndex: 1,
		exitChan:  make(chan bool, 1),
		running:   true,
	}
	newLogger.wg.Add(1)
	go newLogger.process()

	return &newLogger
}

func (l *Logger) Close() {
	l.running = false
	l.exitChan <- true
	l.wg.Wait()
}

func (l *Logger) receiveMsg(level LogLevel, msg string) {
	if !l.running {
		return
	}
	if level < LevelNil {
		level = LevelNil
	}
	if level > LevelError {
		level = LevelError
	}
	if LogLevel(l.config.Level) > level {
		return
	}

	lm := l.logMsgPool.Get().(*LogMessage)

	lm.Level = mapLevelStr[level]
	lm.Msg = msg
	lm.When = time.Now().Format("2006-01-02 15:04:05.000")
	_, file, line, _ := runtime.Caller(l.config.CallerDepth)
	lm.File = file
	lm.Line = line
	str := l.format(lm)
	fmt.Println(str)
	//l.msgChan <- lm
	return
}

func (l *Logger) process() {
	for {
		select {
		case msg := <-l.msgChan:
			l.output(msg)
			l.logMsgPool.Put(msg)
		case ec := <-l.exitChan:
			if ec == true {
				for {
					if len(l.msgChan) > 0 {
						msg := <-l.msgChan
						l.output(msg)
						l.logMsgPool.Put(msg)
						continue
					}
					break
				}
				close(l.msgChan)
				close(l.exitChan)
				l.wg.Done()
				return
			}
		}
	}
}

func (l *Logger) format(msg *LogMessage) string {
	if l.config.OutputJson {
		mb, _ := json.Marshal(msg)
		return string(mb)
	} else {
		format := "%s [%s]\t [%s:%d] %s"
		return fmt.Sprintf(format,
			msg.When,
			msg.Level,
			msg.File,
			msg.Line,
			msg.Msg,
		)
	}
}

func (l *Logger) getLogFilename() string {
	if l.config.LogDir != "" {
		logFilePrefix := filepath.Base(os.Args[0])
		if l.config.LogFilePrefix != "" {
			logFilePrefix = l.config.LogFilePrefix
		}
		filename := logFilePrefix
		day := ""
		if l.config.Daily {
			day = time.Now().Format("2006-01-02")
			filename += "_" + day
		}

		filename += fmt.Sprintf("_%04d", l.fileIndex)
		filename += ".log"
		fp := filepath.Join(l.config.LogDir, filename)

		return fp
	}
	return ""
}

func (l *Logger) checkLogFile(fp string) error {
	fi, err := os.Stat(fp)
	if err != nil {
		if os.IsNotExist(err) {
			return nil
		}
		return err
	}

	if fi.Size() >= l.config.MaxSize {
		return errors.New("over max size")
	}
	f, err := os.Open(fp)
	if err != nil {
		return err
	}
	fileBuf := bufio.NewReader(f)
	lineCount := int64(0)
	for {
		_, err := fileBuf.ReadString(byte('\n'))
		lineCount++
		if err == io.EOF {
			break
		}
	}
	_ = f.Close()

	if lineCount >= l.config.MaxLine {
		return errors.New("over max line")
	}

	return nil
}

func (l *Logger) writeMsgToFile(fp string, msg string) error {
	msg += "\r\n"
	f, err := os.OpenFile(fp, os.O_WRONLY|os.O_CREATE|os.O_APPEND, 0666)
	if err != nil {
		log.Println(err.Error())
	}

	_, err = f.Write([]byte(msg))
	if err != nil {
		log.Println(err.Error())
	}
	_ = f.Close()
	return nil
}

func (l *Logger) output(msg *LogMessage) {
	line := l.format(msg)

	if l.config.Console {
		_, _ = fmt.Fprintln(os.Stdout, line)
	}

	if l.config.LogDir != "" {
		for {
			if l.fileIndex > 9 {
				fmt.Println(fmt.Sprintf(`[E] log file count over 9999`))
				break
			}
			fp := l.getLogFilename()

			if err := l.checkLogFile(fp); err != nil {
				l.fileIndex++
				fmt.Println(fmt.Sprintf(`[E] check log file error, %v`, err))
				continue
			}
			if err := l.writeMsgToFile(fp, line); err != nil {
				fmt.Println(fmt.Sprintf(`[E] write msg "%s" to log file "%s" error, %v`, line, fp, err))
			}

			break
		}

	}
}

/////////////////////////////////////////////////////////////////////////////////
//external access interface

func (l *Logger) Debug(msg string) {
	l.receiveMsg(LevelDebug, msg)
}
func (l *Logger) DebugF(format string, a ...interface{}) {
	l.receiveMsg(LevelDebug, fmt.Sprintf(format, a...))
}

func (l *Logger) Info(msg string) {
	l.receiveMsg(LevelInfo, msg)
}
func (l *Logger) InfoF(format string, a ...interface{}) {
	l.receiveMsg(LevelInfo, fmt.Sprintf(format, a...))
}

func (l *Logger) Warning(msg string) {
	l.receiveMsg(LevelWarning, msg)
}
func (l *Logger) WarningF(format string, a ...interface{}) {
	l.receiveMsg(LevelWarning, fmt.Sprintf(format, a...))
}

func (l *Logger) Error(msg string) {
	l.receiveMsg(LevelError, msg)
}
func (l *Logger) ErrorF(format string, a ...interface{}) {
	l.receiveMsg(LevelError, fmt.Sprintf(format, a...))
}
