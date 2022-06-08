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
		ListenAddress:     fmt.Sprintf(":%d", port),
	}

	zNet.InitTcpServerDefault(&netCfg,
		zNet.WithMaxClientCount(100000),
		zNet.WithSidInitio(10000),
		//zNet.WithMaxPacketDataSize(zNet.DefaultPacketDataSize*100),
		zNet.WithRsaEncrypt("rsa_private.key"),
		zNet.WithHeartbeat(30),
	)

	err = zNet.RegisterHandler(1, HandlerLogin)
	if err != nil {
		zLog.Error("RegisterHandler error", zap.Error(err))
		return
	}

	err = zNet.GetTcpServerDefault().Start()
	if err != nil {
		log.Printf(err.Error())
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
		log.Printf("receive:%s, %s, %v", loginData.UserName, loginData.Password, err)
		return
	}

	mill := time.Duration(time.Now().UnixNano()-loginData.Time) * time.Nanosecond
	fmt.Println(loginData, mill.String())

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
