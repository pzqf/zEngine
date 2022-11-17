package main

import (
	"encoding/json"
	"fmt"
	"log"
	"time"

	"go.uber.org/zap"

	//_ "net/http/pprof"

	"github.com/pzqf/zEngine/zLog"

	"github.com/pzqf/zEngine/zNet"
	"github.com/pzqf/zEngine/zSignal"

	"github.com/pkg/profile"
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
		MaxPacketDataSize: zNet.DefaultPacketDataSize * 100,

		Tcp: &zNet.TcpConfig{
			ListenAddress:     fmt.Sprintf(":%d", port),
			HeartbeatDuration: 0,
		},
	}

	zNet.InitPacket(netCfg.MaxPacketDataSize)
	zNet.InitTcpServerDefault(netCfg.Tcp,
		zNet.WithMaxClientCount(100000),
		zNet.WithMaxPacketDataSize(zNet.DefaultPacketDataSize),
		zNet.WithRsaEncrypt("rsa_private.key"),
		zNet.WithHeartbeat(30),
		zNet.WithAddSessionCallBack(func(sid zNet.SessionIdType) {
			zLog.Info("add session", zap.Any("session id", sid))
		}),
		zNet.WithRemoveSessionCallBack(func(sid zNet.SessionIdType) {
			zLog.Info("remove session", zap.Any("session id", sid))
		}),
	)

	zNet.SetLogPrintFunc(func(v ...any) {
		zLog.Info("zNet info", zap.Any("info", v))
	})

	err = zNet.GetTcpServerDefault().RegisterHandler(1, HandlerLogin)
	if err != nil {
		zLog.Error("RegisterHandler error", zap.Error(err))
		return
	}

	err = zNet.GetTcpServerDefault().Start()
	if err != nil {
		zLog.Error(err.Error())
		return
	}

	zSignal.GracefulExit()
	log.Printf("server will be shut off")
	zNet.GetTcpServerDefault().Close()
	log.Printf("====>>> FBI warning, server exit <<<=====")
}

func HandlerLogin(si zNet.Session, protoId int32, data []byte) {
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

	mill := time.Duration(time.Now().UnixNano()-loginData.Time) * time.Nanosecond
	zLog.Info(fmt.Sprintf("received:%#v, %s", loginData, mill.String()))

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
