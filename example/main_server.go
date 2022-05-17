package main

import (
	"fmt"
	"log"
	_ "net/http/pprof"

	"github.com/pzqf/zEngine/zNet"
	"github.com/pzqf/zEngine/zSignal"
)

func main() {
	port := 9106
	zNet.InitDefaultTcpServer(fmt.Sprintf(":%d", port), 100000)
	zNet.InitPacket(zNet.PacketCodeJson, zNet.MaxNetPacketDataSize)

	err := zNet.RegisterHandler(1, HandlerLogin)
	if err != nil {
		log.Printf("RegisterHandler error %d", 1)
		return
	}

	err = zNet.StartDefaultTcpServer()
	if err != nil {
		log.Printf(err.Error())
		return
	}
	log.Printf("Tcp server listing on %d ", port)

	zSignal.GracefulExit()
	log.Printf("server will be shut off")
	zNet.CloseDefaultTcpServer()
	log.Printf("====>>> FBI warning, server exit <<<=====")
}

func HandlerLogin(session *zNet.Session, packet *zNet.NetPacket) {
	type loginDataInfo struct {
		UserName string `json:"user_name"`
		Password string `json:"password"`
		Time     int64  `json:"time"`
	}

	var data loginDataInfo
	err := packet.DecodeData(&data)
	if err != nil {
		log.Printf("receive:%s, %s", data.UserName, data.Password)
		return
	}
	log.Printf("receive:%d, %s, %s, %d", packet.ProtoId, data.UserName, data.Password, data.Time)

	type PlayerInfo struct {
		Id    int32  `json:"id"`
		Name  string `json:"name"`
		Level int32  `json:"level"`
		Time  int64  `json:"time"`
	}

	sendData := PlayerInfo{
		Id:    2,
		Name:  data.UserName,
		Level: int32(session.GetSid()),
		Time:  data.Time,
	}
	_ = session.Send(1, sendData)

}
