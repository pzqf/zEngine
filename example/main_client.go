package main

import (
	"flag"
	"fmt"
	"log"
	"sync"
	"time"

	"github.com/pzqf/zEngine/zNet"
)

func main() {

	address := flag.String("a", "127.0.0.1", "server address")
	count := flag.Int("n", 1000, "client count")
	flag.Parse()

	var wg sync.WaitGroup
	failedCount := 0
	begin := time.Now()
	clientCount := *count

	err := zNet.RegisterHandler(1, HandlerLoginRes)
	if err != nil {
		log.Printf("RegisterHandler error %d", 1)
		return
	}

	zNet.InitPacket(zNet.PacketCodeJson, zNet.MaxNetPacketDataSize)
	wg.Add(clientCount)
	for i := 0; i < clientCount; i++ {
		time.Sleep(1 * time.Microsecond)
		go func(x int) {
			defer func() {
				wg.Done()
			}()
			cli := zNet.TcpClient{}

			err = cli.ConnectToServer(*address, 9106)
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

			for i := 0; i < 5; i++ {
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
				time.Sleep(time.Microsecond * 10)
			}

			time.Sleep(time.Second * 1)

		}(i)
	}

	wg.Wait()
	fmt.Printf("==============clientCount:%d, failedCount:%d, cost:%s \n",
		clientCount, failedCount, time.Now().Sub(begin).String())

}

func HandlerLoginRes(session *zNet.Session, packet *zNet.NetPacket) {
	type PlayerInfo struct {
		Id    int32  `json:"id"`
		Name  string `json:"name"`
		Level int32  `json:"level"`
		Time  int64  `json:"time"`
	}

	var data PlayerInfo
	err := packet.DecodeData(&data)
	if err != nil {
		log.Printf("receive:%s, %s", data.Name, data.Time)
		return
	}
	mill := time.Duration(time.Now().UnixNano()-data.Time) * time.Nanosecond
	fmt.Println(fmt.Sprintf("receive player data:%d, %v, time:%s", packet.ProtoId, data, mill.String()))
}
