package zNet

import (
	"log"
	"runtime/debug"
)

func Recover() {
	if err := recover(); err != nil {
		log.Println("panic:", err)
		log.Println(string(debug.Stack()))
	}
}
