package zNet

import (
	"crypto/x509"
	"encoding/pem"
	"io"
	"log"
	"os"
)

func WithMaxClientCount(maxClientCount int32) Options {
	return func(svr *TcpServer) {
		svr.maxClientCount = maxClientCount
	}
}
func WithSidInitio(sidInitio int64) Options {
	return func(svr *TcpServer) {
		svr.clientSIDAtomic = sidInitio
	}
}

func WithMaxPacketDataSize(size int32) Options {
	return func(svr *TcpServer) {
		maxPacketDataSize = size
	}
}

func WithDispatcherPoolSize(size int) Options {
	return func(svr *TcpServer) {
		defaultPoolSize = size
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
				log.Println("public key error")
				return
			}

			//x509.ParsePKCS8PrivateKey()
			prkI, err := x509.ParsePKCS1PrivateKey(block.Bytes)
			if err != nil {
				log.Println("ParsePKCS1PrivateKey error", err)
				return
			}

			svr.privateKey = prkI //.(*rsa.PrivateKey)
		}

	}
}
