package main

import (
	"fmt"
	"log"
	"sync"
	"time"

	"github.com/pzqf/zEngine/zNet"
)

func main() {
	wg := sync.WaitGroup{}
	failedCount := 0
	begin := time.Now()
	clientCount := 10000

	err := zNet.RegisterHandler(1, HandlerLoginRes)
	if err != nil {
		log.Printf("RegisterHandler error %d", 1)
		return
	}

	zNet.InitPacketCodeType(zNet.PacketCodeJson)

	for i := 0; i < clientCount; i++ {
		time.Sleep(1 * time.Microsecond)
		go func(x int) {
			wg.Add(1)
			defer func() {
				wg.Done()
			}()
			cli := zNet.TcpClient{}

			err = cli.ConnectToServer("192.168.50.206", 9106)
			//err := cli.ConnectToServer("127.0.0.1", 9106)
			if err != nil {
				fmt.Printf("Connect:%d, err:%s \n", x, err.Error())
				failedCount += 1
				return
			}

			defer cli.Close()
			//fmt.Println("Connect success :", x)

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

			err = cli.Send(1, &newData)
			if err != nil {
				fmt.Println(err)
				return
			}

			time.Sleep(time.Second * 2)

		}(i)
	}

	wg.Wait()
	fmt.Printf("========================failedCount:%d, cost:%s \n", failedCount, time.Now().Sub(begin).String())

}

func HandlerLoginRes(session *zNet.Session, packet *zNet.NetPacket) {
	type receiveData struct {
		Id    int32  `json:"id"`
		Name  string `json:"name"`
		Level int32  `json:"level"`
		Time  int64  `json:"time"`
	}

	var data receiveData
	err := packet.DecodeData(&data)
	if err != nil {
		log.Printf("receive:%s, %s", data.Name, data.Time)
		return
	}
	mill := time.Duration(time.Now().UnixNano()-data.Time) * time.Nanosecond
	fmt.Println(fmt.Sprintf("receive player data:%d, %v, time:%s", packet.ProtoId, data, mill.String()))

}
