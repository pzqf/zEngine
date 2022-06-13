package zNet

import (
	"crypto/x509"
	"encoding/pem"
	"io"
	"os"
)

func WithMaxClientCount(maxClientCount int32) Options {
	return func(svr *TcpServer) {
		GConfig.MaxClientCount = maxClientCount
	}
}
func WithSidInitio(sidInitio int64) Options {
	return func(svr *TcpServer) {
		svr.clientSIDAtomic = sidInitio
	}
}

func WithMaxPacketDataSize(size int32) Options {
	return func(svr *TcpServer) {
		GConfig.MaxPacketDataSize = size
		if GConfig.MaxPacketDataSize == 0 {
			GConfig.MaxPacketDataSize = DefaultPacketDataSize
		}
		InitPacket(GConfig.MaxPacketDataSize)
	}
}

func WithRsaEncrypt(rsaPrivateFile string) Options {
	return func(svr *TcpServer) {
		if rsaPrivateFile != "" {
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

			svr.privateKey = prkI //.(*rsa.PrivateKey)
			LogPrint("rsa encrypt opened")
		}
	}
}

func WithChanSize(chanSize int32) Options {
	return func(svr *TcpServer) {
		if chanSize > 0 {
			GConfig.ChanSize = chanSize
		}
	}
}

func WithHeartbeat(duration int) Options {
	return func(svr *TcpServer) {
		GConfig.HeartbeatDuration = duration
	}
}

func WithLogPrintFunc(lpf LogPrintFunc) Options {
	return func(svr *TcpServer) {
		LogPrint = lpf
	}
}
