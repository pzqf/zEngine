package zSignal

import (
	"os"
	"os/signal"
	"syscall"
	"zEngine/zLog"
)

func GracefulExit() {
	sc := make(chan os.Signal, 1)
	signal.Notify(sc, syscall.SIGHUP, syscall.SIGINT, syscall.SIGTERM, syscall.SIGQUIT, syscall.SIGKILL)
	select {
	case s := <-sc:
		zLog.InfoF("receive signal %d:%v, app quit", s, s)
		switch s {
		case syscall.SIGHUP:
		case syscall.SIGINT,
			syscall.SIGTERM,
			syscall.SIGQUIT,
			syscall.SIGKILL:
		}
	}
}
