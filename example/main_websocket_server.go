package main

import (
	"encoding/json"
	"fmt"
	"log"
	"time"

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
		MaxPacketDataSize: zNet.DefaultPacketDataSize * 100,
		WebSocket: &zNet.WebSocketConfig{
			ListenAddress: fmt.Sprintf(":%d", port),
		},
	}

	wsServer := zNet.NewWebSocketServer(netCfg.WebSocket,
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

	err = zNet.RegisterHandler(1001, WebSocketHandlerLogin)
	if err != nil {
		zLog.Error("RegisterHandler error", zap.Error(err))
		return
	}

	err = wsServer.Start()
	if err != nil {
		zLog.Error(err.Error())
		return
	}

	log.Printf("websocket server listen on %d", port)

	zSignal.GracefulExit()
	log.Printf("server will be shut off")
	wsServer.Close()
	log.Printf("====>>> FBI warning, server exit <<<=====")
}

func WebSocketHandlerLogin(si zNet.Session, protoId int32, data []byte) {
	fmt.Println("收到登录消息")
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
	zLog.Info(fmt.Sprintf("send:%s", string(marshal)))
	_ = si.Send(1, marshal)
}
