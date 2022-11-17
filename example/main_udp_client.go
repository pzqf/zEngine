package main

import (
	"encoding/json"
	"flag"
	"fmt"
	"log"
	"sync"
	"time"

	"github.com/pzqf/zEngine/zNet"
)

func main() {
	address := flag.String("a", "127.0.0.1", "server address")
	count := flag.Int("n", 1, "client count")
	flag.Parse()

	var wg sync.WaitGroup
	failedCount := 0
	begin := time.Now()
	clientCount := *count

	//PS:Same as the server
	zNet.InitPacket(zNet.DefaultPacketDataSize)
	wg.Add(clientCount)
	for i := 0; i < clientCount; i++ {
		time.Sleep(1 * time.Millisecond)
		go func(x int) {
			defer func() {
				wg.Done()
			}()
			cli := zNet.UdpClient{}
			err := cli.RegisterHandler(1, HandlerUdpTestRes)
			if err != nil {
				log.Printf("RegisterHandler error %d", 1)
				return
			}

			err = cli.ConnectToServer(*address, 9160)
			if err != nil {
				fmt.Printf("Connect:%d, err:%s \n", x, err.Error())
				failedCount += 1
				return
			}

			defer cli.Close()

			type loginDataInfo struct {
				UserName string   `json:"user_name"`
				Password string   `json:"password"`
				Time     int64    `json:"time"`
				Over     []string `json:"over"`
			}

			for n := 0; n < 100; n++ {
				newData := loginDataInfo{
					UserName: fmt.Sprintf("pppp-%d", x),
					Password: "123456",
					Time:     time.Now().UnixNano(),
				}
				// test
				for s := 0; s < 5; s++ {
					newData.Over = append(newData.Over, "ddddddddddddddddddddddddddddddddddddddddddddddddddddddddddddddddddddddddddddddd")
				}

				marshal, err := json.Marshal(newData)
				if err != nil {
					return
				}

				err = cli.Send(1, marshal)
				if err != nil {
					fmt.Println(err)
					return
				}
				time.Sleep(time.Microsecond * 10)
			}
			time.Sleep(time.Second * 5)
		}(i)
	}
	wg.Wait()
	fmt.Printf("==============clientCount:%d, failedCount:%d, cost:%s \n",
		clientCount, failedCount, time.Now().Sub(begin).String())

}

func HandlerUdpTestRes(si zNet.Session, protoId int32, data []byte) {
	type PlayerInfo struct {
		Id    int32  `json:"id"`
		Name  string `json:"name"`
		Level int32  `json:"level"`
		Time  int64  `json:"time"`
	}

	var loginResData PlayerInfo
	err := json.Unmarshal(data, &loginResData)
	if err != nil {
		fmt.Println(err)
		return
	}
	mill := time.Duration(time.Now().UnixNano()-loginResData.Time) * time.Nanosecond
	if mill > time.Millisecond*1 {
		fmt.Println(fmt.Sprintf("receive player data:%d, %v, time:%s, loooooooong", protoId, loginResData, mill.String()))
	} else {
		fmt.Println(fmt.Sprintf("receive player data:%d, %v, time:%s", protoId, loginResData, mill.String()))
	}

}
