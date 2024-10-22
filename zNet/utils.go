package zNet

import "log"

func Recover() {
	if err := recover(); err != nil {
		//LogPrint("panic:", err)
		//LogPrint(string(debug.Stack()))
	}
}

type LogPrintFunc func(v ...any)

var LogPrint LogPrintFunc = log.Println

func SetLogPrintFunc(f LogPrintFunc) {
	LogPrint = f
}
