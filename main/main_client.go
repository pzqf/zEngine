package main

import (
	"fmt"
	"sync"
	"time"
	"zEngine/zNet/NetServer"
)

// for test
func main() {
	wg := sync.WaitGroup{}
	failedCount := 0
	begin := time.Now()

	for i := 0; i < 10000; i++ {
		time.Sleep(100 * time.Nanosecond)
		wg.Add(1)
		go func(x int) {
			defer wg.Done()
			cli := NetServer.NetClient{}
			err := cli.Connect("192.168.50.206", 9106)
			//err := cli.Connect("127.0.0.1", 9106)
			if err != nil {
				fmt.Printf("Connect:%d, err:%s \n", x, err.Error())
				failedCount += 1
				return
			}
			defer cli.Close()
			fmt.Println("Connect success :", x)

			type loginDataInfo struct {
				UserName string `json:"user_name"`
				Password string `json:"password"`
				Time     int64  `json:"time"`
			}

			newData := loginDataInfo{
				UserName: fmt.Sprintf("pppp-%d", x),
				Password: "123456",
				Time:     time.Now().UnixNano(),
			}

			err = cli.Send(1, newData)
			if err != nil {
				fmt.Println(err)
				return
			}
			fmt.Println("Send NetPacket, ProtoId:", 1, newData)

			netPacket, err := cli.Receive()
			if err != nil {
				fmt.Println(err)
				return
			}

			type PlayerInfo struct {
				Id    int32  `json:"id"`
				Name  string `json:"name"`
				Level int32  `json:"level"`
				Time  int64  `json:"time"`
			}

			var receiveData PlayerInfo
			err = netPacket.DecodeData(&receiveData)
			if err != nil {
				fmt.Println(err)
				return
			}
			mill := time.Duration(time.Now().UnixNano()-receiveData.Time) * time.Nanosecond
			fmt.Println(fmt.Sprintf("receive player data:%v, time:%s", receiveData, mill.String()))
			//select {}
		}(i)

	}
	wg.Wait()
	fmt.Printf("========================failedCount:%d, cost:%s \n", failedCount, time.Now().Sub(begin).String())

}
