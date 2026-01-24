package zNet

import (
	"crypto/x509"
	"encoding/pem"
	"io"
	"os"
	"reflect"
)

type Options func(Server)

func WithMaxClientCount(count int) Options {
	return func(svr Server) {
		switch reflect.TypeOf(svr).String() {
		case "*zNet.TcpServer":
			svr.(*TcpServer).config.MaxClientCount = count
		case "*zNet.UdpServer":
			svr.(*UdpServer).config.MaxClientCount = count
		case "*zNet.WebSocketServer":
			svr.(*WebSocketServer).config.MaxClientCount = count
		case "*zNet.HttpServer":
			svr.(*HttpServer).config.MaxClientCount = count
		}
	}
}

func WithMaxPacketDataSize(size int32) Options {
	return func(svr Server) {
		switch reflect.TypeOf(svr).String() {
		case "*zNet.TcpServer":
			svr.(*TcpServer).config.MaxPacketDataSize = size
		case "*zNet.UdpServer":
			svr.(*UdpServer).config.MaxPacketDataSize = size
		case "*zNet.WebSocketServer":
			svr.(*WebSocketServer).config.MaxPacketDataSize = size
		case "*zNet.HttpServer":
			svr.(*HttpServer).config.MaxPacketDataSize = size
		}
	}
}

func WithRsaEncrypt(rsaPrivateFile string) Options {
	return func(svr Server) {
		if rsaPrivateFile != "" && reflect.TypeOf(svr).String() == "*zNet.TcpServer" {
			f, err := os.Open(rsaPrivateFile)
			if err != nil {
				return
			}
			all, err := io.ReadAll(f)
			if err != nil {
				return
			}

			block, _ := pem.Decode(all)
			if block == nil {
				//LogPrint("public key error")
				return
			}

			//x509.ParsePKCS8PrivateKey()
			prkI, err := x509.ParsePKCS1PrivateKey(block.Bytes)
			if err != nil {
				//LogPrint("ParsePKCS1PrivateKey error", err)
				return
			}

			svr.(*TcpServer).privateKey = prkI //.(*rsa.PrivateKey)
			//LogPrint("rsa encrypt opened", prkI)
		}
	}
}

func WithChanSize(chanSize int) Options {
	return func(svr Server) {
		if chanSize <= 0 {
			return
		}
		switch reflect.TypeOf(svr).String() {
		case "*zNet.TcpServer":
			svr.(*TcpServer).config.ChanSize = chanSize
		case "*zNet.UdpServer":
			svr.(*UdpServer).config.ChanSize = chanSize
		case "*zNet.WebSocketServer":
			svr.(*WebSocketServer).config.ChanSize = chanSize
		}
	}
}

func WithHeartbeat(duration int) Options {
	return func(svr Server) {
		switch reflect.TypeOf(svr).String() {
		case "*zNet.TcpServer":
			svr.(*TcpServer).config.HeartbeatDuration = duration
		case "*zNet.UdpServer":
			svr.(*UdpServer).config.HeartbeatDuration = duration
		case "*zNet.WebSocketServer":
			svr.(*WebSocketServer).config.HeartbeatDuration = duration
		}
	}
}

func WithAddSessionCallBack(cb SessionCallBackFunc) Options {
	return func(svr Server) {
		switch reflect.TypeOf(svr).String() {
		case "*zNet.TcpServer":
			svr.(*TcpServer).onAddSession = cb
		case "*zNet.UdpServer":
			svr.(*UdpServer).onAddSession = cb
		case "*zNet.WebSocketServer":
			svr.(*WebSocketServer).onAddSession = cb
		case "*zNet.HttpServer":
			svr.(*HttpServer).onAddSession = cb
		}
	}
}

func WithRemoveSessionCallBack(cb SessionCallBackFunc) Options {
	return func(svr Server) {
		switch reflect.TypeOf(svr).String() {
		case "*zNet.TcpServer":
			svr.(*TcpServer).onRemoveSession = cb
		case "*zNet.UdpServer":
			svr.(*UdpServer).onRemoveSession = cb
		case "*zNet.WebSocketServer":
			svr.(*WebSocketServer).onRemoveSession = cb
		case "*zNet.HttpServer":
			svr.(*HttpServer).onRemoveSession = cb
		}
	}
}

func WithWorkerPoolSize(size int) Options {
	return func(svr Server) {
		// Worker pool has been removed to ensure FIFO packet processing
		// This function is kept for backward compatibility
	}
}

func WithLogger(logger Logger) Options {
	return func(svr Server) {
		switch reflect.TypeOf(svr).String() {
		case "*zNet.TcpServer":
			svr.(*TcpServer).logger = logger
		case "*zNet.UdpServer":
			svr.(*UdpServer).logger = logger
		case "*zNet.WebSocketServer":
			svr.(*WebSocketServer).logger = logger
		case "*zNet.HttpServer":
			svr.(*HttpServer).logger = logger
		}
	}
}

// WithDDoSConfig 设置DDoS保护配置
func WithDDoSConfig(cfg *DDoSConfig) Options {
	return func(svr Server) {
		switch reflect.TypeOf(svr).String() {
		case "*zNet.TcpServer":
			svr.(*TcpServer).ddosProtection = NewDDoSProtection(cfg)
		case "*zNet.UdpServer":
			svr.(*UdpServer).ddosProtection = NewDDoSProtection(cfg)
		case "*zNet.WebSocketServer":
			svr.(*WebSocketServer).ddosProtection = NewDDoSProtection(cfg)
		case "*zNet.HttpServer":
			svr.(*HttpServer).ddosProtection = NewDDoSProtection(cfg)
		}
	}
}
