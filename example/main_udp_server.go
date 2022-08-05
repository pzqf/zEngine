package main

import (
	"encoding/json"
	"fmt"
	"log"

	"github.com/pkg/profile"
	"github.com/pzqf/zEngine/zLog"
	"github.com/pzqf/zEngine/zNet"
	"github.com/pzqf/zEngine/zSignal"
	"go.uber.org/zap"
)

func main() {
	stopper := profile.Start(profile.CPUProfile, profile.ProfilePath("."), profile.NoShutdownHook)
	defer stopper.Stop()
	// go tool pprof -http=:9999 cpu.pprof

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
	port := 9160

	netCfg := zNet.Config{
		MaxPacketDataSize: zNet.DefaultPacketDataSize,
		Udp: &zNet.UdpConfig{
			ListenAddress: fmt.Sprintf(":%d", port),
		},
	}
	zNet.InitPacket(netCfg.MaxPacketDataSize)
	udpServer := zNet.NewUdpServer(netCfg.Udp)

	err = zNet.RegisterHandler(1, HandlerUdpTest)
	if err != nil {
		zLog.Error("RegisterHandler error", zap.Error(err))
		return
	}

	err = udpServer.Start()
	if err != nil {
		log.Printf(err.Error())
		return
	}

	zSignal.GracefulExit()
	log.Printf("server will be shut off")
	udpServer.Close()
	log.Printf("====>>> FBI warning, server exit <<<=====")
}

func HandlerUdpTest(si zNet.Session, protoId int32, data []byte) {
	type loginDataInfo struct {
		UserName string   `json:"user_name"`
		Password string   `json:"password"`
		Time     int64    `json:"time"`
		Over     []string `json:"over"`
	}

	var loginData loginDataInfo
	err := json.Unmarshal(data, &loginData)
	if err != nil {
		zLog.Error(fmt.Sprintf("receive err: %v", err))
		return
	}

	//mill := time.Duration(time.Now().UnixNano()-loginData.Time) * time.Nanosecond
	//zLog.Info(fmt.Sprintf("received:%#v, %s", loginData, mill.String()))
	//fmt.Printf("received:%#v, %s", loginData, mill.String())

	type PlayerInfo struct {
		Id    int32  `json:"id"`
		Name  string `json:"name"`
		Level int32  `json:"level"`
		Time  int64  `json:"time"`
	}

	sendData := PlayerInfo{
		Id:    2,
		Name:  loginData.UserName,
		Level: 100,
		Time:  loginData.Time,
	}

	marshal, err := json.Marshal(sendData)
	if err != nil {
		log.Println(err)
		return
	}

	_ = si.Send(1, marshal)
}
