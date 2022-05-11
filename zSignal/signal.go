package zSignal

import (
	"log"
	"os"
	"os/signal"
	"syscall"
)

func GracefulExit() {
	sc := make(chan os.Signal, 1)
	signal.Notify(sc, syscall.SIGHUP, syscall.SIGINT, syscall.SIGTERM, syscall.SIGQUIT, syscall.SIGKILL)
	select {
	case s := <-sc:
		log.Printf("Receive signal %d:%v, app will be quit", s, s)
		switch s {
		case syscall.SIGHUP:
		case syscall.SIGINT,
			syscall.SIGTERM,
			syscall.SIGQUIT,
			syscall.SIGKILL:
		}
	}
}
