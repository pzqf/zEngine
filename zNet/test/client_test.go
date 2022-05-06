package test

import (
	"fmt"
	"sync"
	"testing"
	"time"
	"zEngine/zLog"
	"zEngine/zNet/NetServer"
)

func Test_Client(t *testing.T) {
	zLog.SetLogger(`{
			"log_dir": "./logs"
		}`)

	wg := sync.WaitGroup{}
	failedCount := 0

	for i := 0; i < 500; i++ {
		time.Sleep(10 * time.Millisecond)
		wg.Add(1)
		go func(x int) {
			defer wg.Done()
			cli := NetServer.NetClient{}
			err := cli.Connect("127.0.0.1", 916)
			if err != nil {
				zLog.ErrorF("Connect:%d, err:%s", x, err.Error())
				failedCount += 1
				return
			}
			zLog.InfoF("Connect success :%d", x)

			type loginDataInfo struct {
				UserName string `json:"user_name"`
				Password string `json:"password"`
			}

			newData := loginDataInfo{
				UserName: fmt.Sprintf("pppp%d", x),
				Password: "123456",
			}

			_ = cli.Send(1, newData)

			netPacket := cli.Receive()
			type PlayerInfo struct {
				Id    int32  `json:"id"`
				Name  string `json:"name"`
				Level int32  `json:"level"`
			}

			var receiveData PlayerInfo
			err = netPacket.DecodeData(&receiveData)
			if err != nil {
				zLog.Error(err.Error())
				return
			}
			//zLog.Info(fmt.Sprintf("receive player data:%v", receiveData))
			cli.Close()
		}(i)

	}
	wg.Wait()
	zLog.InfoF("========================failedCount:%d", failedCount)

}
