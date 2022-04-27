package zLog

import (
	"fmt"
	"testing"
	"time"
)

func TestMain(m *testing.M) {

	SetLogger(`{"log_dir": "./logs", "output_json":false }`)

	for i := 0; i < 3; i++ {
		Info("test info")
		Debug("test debug")
		Warning("test warning")
		Error("test error")
		time.Sleep(1 * time.Second)
	}

	Close()
	fmt.Println("Fbi waring... server exist")
}
