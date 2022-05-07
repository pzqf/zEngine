package main

import (
	"log"
	"zEngine/zLog"
	"zEngine/zNet"
	"zEngine/zSignal"
)

//for test
func main() {
	zLog.SetLogger(`{
			"log_dir": "./logs",
			"log_file_prefix": "server"
		}`)
	zLog.Info("Init tcp server ... ")
	port := 9106
	zNet.InitTcpServer("", port, 100000)

	err := zNet.RegisterHandler(1, HandlerLogin)
	if err != nil {
		zLog.ErrorF("RegisterHandler error %d", 1)
		return
	}
	err = zNet.StartTcpServer()
	if err != nil {
		zLog.Error(err.Error())
		zLog.Close()
		return
	}
	zLog.InfoF("Tcp server listing on %d ", port)

	zSignal.GracefulExit()
	zLog.InfoF("server will be shut off")
	zNet.CloseTcpServer()
	zLog.Close()
	log.Printf("====>>> FBI warning , server exit <<<=====")
}

func HandlerLogin(sid int64, packet *zNet.NetPacket) {
	type loginDataInfo struct {
		UserName string `json:"user_name"`
		Password string `json:"password"`
		Time     int64  `json:"time"`
	}

	var data loginDataInfo
	err := packet.JsonDecodeData(&data)
	if err != nil {
		zLog.InfoF("receive:%s, %s", data.UserName, data.Password)
		return
	}
	zLog.InfoF("receive:%s, %s, %d", data.UserName, data.Password, data.Time)

	type PlayerInfo struct {
		Id    int32  `json:"id"`
		Name  string `json:"name"`
		Level int32  `json:"level"`
		Time  int64  `json:"time"`
	}

	sendData := PlayerInfo{
		Id:    2,
		Name:  data.UserName,
		Level: int32(sid),
		Time:  data.Time,
	}

	netPacket := zNet.NetPacket{}
	netPacket.ProtoId = 1

	err = netPacket.JsonEncodeData(sendData)
	if err != nil {
		return
	}

	zNet.SendToClient(sid, 1, &netPacket)
}
