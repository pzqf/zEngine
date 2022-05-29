package main

import (
	"fmt"
	"log"
	_ "net/http/pprof"
	"time"

	"go.uber.org/zap"

	"github.com/pzqf/zEngine/zLog"

	"github.com/pzqf/zEngine/zNet"
	"github.com/pzqf/zEngine/zSignal"
)

func main() {

	cfg := zLog.Config{
		Level:    zLog.InfoLevel,
		Console:  true,
		Filename: "./logs/server.log",
		MaxSize:  1024,
	}
	err := zLog.InitLogger(&cfg)
	if err != nil {
		fmt.Println(err)
		return
	}

	port := 9106
	zNet.InitDefaultTcpServer(fmt.Sprintf(":%d", port),
		zNet.WithMaxClientCount(10000),
		zNet.WithSidInitio(10000),
		zNet.WithPacketCodeType(zNet.PacketCodeJson),
		zNet.WithMaxPacketDataSize(zNet.MaxNetPacketDataSize),
		zNet.WithDispatcherPoolSize(10000),
	)

	err = zNet.RegisterHandler(1, HandlerLogin)
	if err != nil {
		log.Printf("RegisterHandler error %d", 1)
		return
	}

	err = zNet.GetDefaultTcpServer().Start()
	if err != nil {
		log.Printf(err.Error())
		return
	}
	log.Printf("Tcp server listing on %d ", port)

	zSignal.GracefulExit()
	log.Printf("server will be shut off")
	zNet.GetDefaultTcpServer().Close()
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
	zLog.Info("receive:", zap.Int32("proto_id", packet.ProtoId), zap.Any("data", data),
		zap.Int64("cost", (time.Now().UnixNano()-data.Time)/100000))

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
