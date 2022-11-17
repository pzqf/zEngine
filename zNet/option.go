package zNet

import (
	"crypto/x509"
	"encoding/pem"
	"io"
	"os"
	"reflect"
)

type Options func(NetServer)

func WithMaxClientCount(maxClientCount int) Options {
	return func(svr NetServer) {
		switch reflect.TypeOf(svr).String() {
		case "*zNet.TcpServer":
			svr.(*TcpServer).config.MaxClientCount = maxClientCount
		case "*zNet.UdpServer":
			svr.(*UdpServer).config.MaxClientCount = maxClientCount
		}
	}
}

func WithMaxPacketDataSize(size int) Options {
	return func(svr NetServer) {
		if size == 0 {
			size = DefaultPacketDataSize
		}
		InitPacket(size)
	}
}

func WithRsaEncrypt(rsaPrivateFile string) Options {
	return func(svr NetServer) {
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
				LogPrint("public key error")
				return
			}

			//x509.ParsePKCS8PrivateKey()
			prkI, err := x509.ParsePKCS1PrivateKey(block.Bytes)
			if err != nil {
				LogPrint("ParsePKCS1PrivateKey error", err)
				return
			}

			svr.(*TcpServer).privateKey = prkI //.(*rsa.PrivateKey)
			LogPrint("rsa encrypt opened", prkI)
		}
	}
}

func WithChanSize(chanSize int) Options {
	return func(svr NetServer) {
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
	return func(svr NetServer) {
		switch reflect.TypeOf(svr).String() {
		case "*zNet.TcpServer":
			svr.(*TcpServer).config.HeartbeatDuration = duration
		case "*zNet.UdpServer":
			svr.(*UdpServer).config.HeartbeatDuration = duration
		}
	}
}

func WithAddSessionCallBack(cb SessionCallBackFunc) Options {
	return func(svr NetServer) {
		switch reflect.TypeOf(svr).String() {
		case "*zNet.TcpServer":
			svr.(*TcpServer).onAddSession = cb
		case "*zNet.WebSocketServer":
			svr.(*WebSocketServer).onAddSession = cb
		}
	}
}

func WithRemoveSessionCallBack(cb SessionCallBackFunc) Options {
	return func(svr NetServer) {
		switch reflect.TypeOf(svr).String() {
		case "*zNet.TcpServer":
			svr.(*TcpServer).onRemoveSession = cb
		case "*zNet.WebSocketServer":
			svr.(*WebSocketServer).onRemoveSession = cb
		}
	}
}
