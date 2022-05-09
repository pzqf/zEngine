package main

import (
	"log"
	"net/http"
	_ "net/http/pprof"
	"runtime"
	"zEngine/zLog"
	"zEngine/zNet"
	"zEngine/zSignal"
)

//for tests
func main() {
	zLog.SetLogger(`{
			"log_dir": "./logs",
			"log_file_prefix": "server"
		}`)
	zLog.Info("Init tcp server ... ")
	zNet.InitDefaultTcpServer(":9106", 100000)

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
	zLog.InfoF("Tcp server listing on %d ", 9106)

	//pprof
	runtime.SetBlockProfileRate(1)     // 开启对阻塞操作的跟踪，block
	runtime.SetMutexProfileFraction(1) // 开启对锁调用的跟踪，mutex
	_ = http.ListenAndServe(":9107", nil)

	zSignal.GracefulExit()
	zLog.InfoF("server will be shut off")
	zNet.CloseTcpServer()
	zLog.Close()
	log.Printf("====>>> FBI warning , server exit <<<=====")
}

func HandlerLogin(session *zNet.Session, packet *zNet.NetPacket) {
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
		Level: int32(session.GetSid()),
		Time:  data.Time,
	}

	netPacket := zNet.NetPacket{}
	netPacket.ProtoId = 1

	err = netPacket.JsonEncodeData(sendData)
	if err != nil {
		return
	}

	err = session.Send(&netPacket)
	if err != nil {
		return
	}
}
