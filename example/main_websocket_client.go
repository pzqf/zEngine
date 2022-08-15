package main

import (
	"encoding/json"
	"fmt"
	"strconv"
	"time"

	"github.com/pzqf/zEngine/zSignal"

	"github.com/pzqf/zEngine/zNet"
	"golang.org/x/net/websocket"
)

func main() {

	wsUrl := "ws://127.0.0.1:9160/ws"
	origin := "ws://127.0.0.1:9160"

	ws, err := websocket.Dial(wsUrl, "", origin)
	if err != nil {
		fmt.Println(err)
		return
	}

	type loginDataInfo struct {
		UserName string   `json:"user_name"`
		Password string   `json:"password"`
		Time     int64    `json:"time"`
		Over     []string `json:"over"`
	}

	go func() {
		for i := 0; i < 100; i++ {
			var loginData = loginDataInfo{
				UserName: "abc" + strconv.Itoa(i),
				Password: "def",
				Time:     time.Now().Unix(),
			}

			data, err := json.Marshal(loginData)

			p := zNet.NetPacket{
				ProtoId:  1001,
				DataSize: int32(len(data)),
				Data:     data,
			}
			_, err = ws.Write(p.Marshal())
			if err != nil {
				fmt.Println(err)
				return
			}
			fmt.Println("---send message:", string(data))
			time.Sleep(time.Microsecond * 100)
		}
	}()

	go func() {
		for {
			pb := make([]byte, 300)

			_, err = ws.Read(pb)
			if err != nil {
				fmt.Println(err)
				return
			}

			fmt.Println("===receive message:", string(pb))
		}
	}()

	zSignal.GracefulExit()
	_ = ws.Close()
}
