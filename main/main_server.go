package main

import (
	"log"
	"zEngine/zLog"
	"zEngine/zNet/NetServer"
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
	NetServer.InitTcpServer("", port, 100000)

	err := NetServer.RegisterHandler(1, HandlerLogin)
	if err != nil {
		zLog.ErrorF("RegisterHandler error %d", 1)
		return
	}
	err = NetServer.Start()
	if err != nil {
		zLog.Error(err.Error())
		zLog.Close()
		return
	}
	zLog.InfoF("Tcp server listing on %d ", port)

	zSignal.GracefulExit()
	zLog.InfoF("server will be shut off")
	NetServer.Close()
	zLog.Close()
	log.Printf("====>>> FBI warning , server exit <<<=====")
}

func HandlerLogin(sid int64, packet *NetServer.NetPacket) {
	type loginDataInfo struct {
		UserName string `json:"user_name"`
		Password string `json:"password"`
		Time     int64  `json:"time"`
	}

	var data loginDataInfo
	err := packet.DecodeData(&data)
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

	NetServer.SendToClient(sid, 1, &sendData)
}
