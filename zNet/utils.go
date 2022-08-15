package zNet

import (
	"log"
	"runtime/debug"
)

type SessionCallBackFunc func(sid SessionIdType)

type SessionIdType = uint64

func Recover() {
	if err := recover(); err != nil {
		LogPrint("panic:", err)
		LogPrint(string(debug.Stack()))
	}
}

type LogPrintFunc func(v ...any)

var LogPrint LogPrintFunc = log.Println

func SetLogPrintFunc(f LogPrintFunc) {
	LogPrint = f
}

var DefaultChanSize = 512
var DefaultMaxClientCount = 10000
