package zNet

import (
	"crypto/rand"
	"crypto/rsa"
	"fmt"
	"io"
	"net"
	"sync"
	"sync/atomic"
	"time"

	"github.com/pzqf/zUtil/zMap"
)

type TcpServer struct {
	clientSIDAtomic  SessionIdType
	listener         *net.TCPListener
	clientSessionMap zMap.Map
	wg               sync.WaitGroup
	onAddSession     SessionCallBackFunc
	onRemoveSession  SessionCallBackFunc
	privateKey       *rsa.PrivateKey
	config           *TcpConfig
}

func NewTcpServer(cfg *TcpConfig, opts ...Options) *TcpServer {
	if cfg.ChanSize <= 0 {
		cfg.ChanSize = DefaultChanSize
	}
	if cfg.MaxClientCount <= 0 {
		cfg.MaxClientCount = DefaultMaxClientCount
	}

	svr := &TcpServer{
		clientSIDAtomic:  10000,
		clientSessionMap: zMap.NewMap(),
		config:           cfg,
	}

	for _, opt := range opts {
		opt(svr)
	}

	return svr
}

func (svr *TcpServer) Start() error {
	tcpAddr, err := net.ResolveTCPAddr("tcp4", svr.config.ListenAddress)
	if err != nil {
		return err
	}
	listener, err := net.ListenTCP("tcp4", tcpAddr)
	if err != nil {
		return err
	}
	svr.listener = listener

	LogPrint(fmt.Sprintf("Tcp server listing on %s", svr.config.ListenAddress))

	go func() {
		svr.wg.Add(1)
		defer svr.wg.Done()
		for {
			if int(svr.clientSessionMap.Len()) >= svr.config.MaxClientCount {
				LogPrint(fmt.Sprintf("Maximum connections exceeded, max:%d", svr.config.MaxClientCount))
				time.Sleep(5 * time.Millisecond)
				continue
			}
			conn, err := svr.listener.AcceptTCP()
			if err != nil {
				LogPrint(err)
				break
			}

			go svr.AddSession(conn)
		}
	}()

	return nil
}

func (svr *TcpServer) Close() {
	LogPrint("Close tcp server, session count ", svr.clientSessionMap.Len())

	_ = svr.listener.Close()

	svr.clientSessionMap.Range(func(key, value interface{}) bool {
		session := value.(*TcpServerSession)
		session.Close()
		svr.clientSessionMap.Delete(session.sid)
		return true
	})

	svr.wg.Wait()
}

func (svr *TcpServer) AddSession(conn *net.TCPConn) {
	var aesKey []byte
	if svr.privateKey != nil {
		_, err := conn.Write([]byte("hello"))
		if err != nil {
			LogPrint(err)
			_ = conn.Close()
			return
		}

		rsaBuf := make([]byte, 256)
		_, _ = io.ReadFull(conn, rsaBuf)

		aesKey, err = rsa.DecryptPKCS1v15(rand.Reader, svr.privateKey, rsaBuf)
		if err != nil {
			LogPrint("Decrypt aes key failed", err)
			_ = conn.Close()
			return
		}
	} else {
		_, err := conn.Write([]byte("nokey"))
		if err != nil {
			LogPrint(err)
			_ = conn.Close()
			return
		}
	}

	sid := atomic.AddUint64(&svr.clientSIDAtomic, 1)
	newSession := NewTcpServerSession(svr.config, conn, sid, svr.RemoveSession, aesKey)

	svr.clientSessionMap.Store(sid, newSession)

	if svr.onAddSession != nil {
		svr.onAddSession(newSession.sid)
	}

	newSession.Start()
}

func (svr *TcpServer) RemoveSession(cli *TcpServerSession) {
	if svr.onRemoveSession != nil {
		svr.onRemoveSession(cli.sid)
	}
	svr.clientSessionMap.Delete(cli.sid)
}

func (svr *TcpServer) GetSession(sid int64) *TcpServerSession {
	if client, ok := svr.clientSessionMap.Get(sid); ok {
		return client.(*TcpServerSession)
	}
	return nil
}

func (svr *TcpServer) GetAllSession() []*TcpServerSession {
	var sessionList []*TcpServerSession
	svr.clientSessionMap.Range(func(key, value interface{}) bool {
		sessionList = append(sessionList, value.(*TcpServerSession))
		return true
	})

	return sessionList
}
