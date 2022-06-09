package zNet

import (
	"log"
	"runtime/debug"
)

func Recover() {
	if err := recover(); err != nil {
		LogPrint("panic:", err)
		LogPrint(string(debug.Stack()))
	}
}

type LogPrintFunc func(v ...any)

var LogPrint LogPrintFunc = log.Println
