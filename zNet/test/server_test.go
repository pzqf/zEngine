package test

import (
	"fmt"
	"testing"
	"zEngine/zLog"
	"zEngine/zNet/NetServer"
	"zEngine/zSignal"
)

func Test_Server(t *testing.T) {
	zLog.SetLogger(`{
			"log_dir": "./logs"
		}`)
	NetServer.InitTcpServer("", 916, 10000)
	err := NetServer.RegisterHandler(1, HandlerLogin)
	if err != nil {
		zLog.ErrorF("RegisterHandler error %d", 1)
		return
	}
	err = NetServer.Start()
	if err != nil {
		zLog.Close()
		return
	}

	zSignal.GracefulExit()
	zLog.InfoF("server will be shut off")
	NetServer.Close()
	zLog.Close()
	fmt.Println("====>>> FBI warning , server exit <<<=====")
}

func HandlerLogin(sid int64, packet *NetServer.NetPacket) {
	type loginDataInfo struct {
		UserName string `json:"user_name"`
		Password string `json:"password"`
	}

	var data loginDataInfo
	err := packet.DecodeData(&data)
	if err != nil {
		zLog.InfoF("receive:%s, %s", data.UserName, data.Password)
		return
	}
	zLog.InfoF("receive:%s, %s", data.UserName, data.Password)

	type PlayerInfo struct {
		Id    int32  `json:"id"`
		Name  string `json:"name"`
		Level int32  `json:"level"`
	}

	sendData := PlayerInfo{
		Id:    2,
		Name:  data.UserName,
		Level: int32(sid),
	}

	NetServer.SendToClientBySID(sid, 1, &sendData)
}
