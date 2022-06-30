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
	sessionPool      sync.Pool
	clientSIDAtomic  SessionIdType
	listener         *net.TCPListener
	clientSessionMap zMap.Map
	wg               sync.WaitGroup
	onAddSession     SessionCallBackFunc
	onRemoveSession  SessionCallBackFunc
	privateKey       *rsa.PrivateKey
}

type SessionCallBackFunc func(sid SessionIdType)

type Options func(*TcpServer)

func NewTcpServer(cfg *Config, opts ...Options) *TcpServer {
	if cfg.ListenAddress == "" {
		cfg.ListenAddress = ":9160"
	}
	if cfg.MaxPacketDataSize == 0 {
		cfg.MaxPacketDataSize = DefaultPacketDataSize
	}
	if cfg.ChanSize <= 0 {
		cfg.ChanSize = 2048
	}
	if cfg.MaxClientCount <= 0 {
		cfg.MaxClientCount = 10000
	}
	GConfig = cfg
	InitPacket(cfg.MaxPacketDataSize)

	svr := &TcpServer{
		clientSIDAtomic:  10000,
		clientSessionMap: zMap.NewMap(),
		sessionPool: sync.Pool{
			New: func() interface{} {
				var s = &TcpServerSession{}
				return s
			},
		},
	}

	for _, opt := range opts {
		opt(svr)
	}

	return svr
}

func (svr *TcpServer) Start() error {
	tcpAddr, err := net.ResolveTCPAddr("tcp4", GConfig.ListenAddress)
	if err != nil {
		return err
	}
	listener, err := net.ListenTCP("tcp4", tcpAddr)
	if err != nil {
		return err
	}
	svr.listener = listener

	LogPrint(fmt.Sprintf("Tcp server listing on %s", GConfig.ListenAddress))

	go func() {
		svr.wg.Add(1)
		defer svr.wg.Done()
		for {
			if svr.clientSessionMap.Len() >= GConfig.MaxClientCount {
				LogPrint(fmt.Sprintf("Maximum connections exceeded, max:%d", GConfig.MaxClientCount))
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

func (svr *TcpServer) AddSession(conn *net.TCPConn) *TcpServerSession {
	newSession := svr.sessionPool.Get().(*TcpServerSession)
	if newSession != nil {
		var aesKey []byte
		if svr.privateKey != nil {
			_, err := conn.Write([]byte("hello"))
			if err != nil {
				LogPrint(err)
				_ = conn.Close()
				return nil
			}

			rsaBuf := make([]byte, 256)
			_, _ = io.ReadFull(conn, rsaBuf)

			aesKey, err = rsa.DecryptPKCS1v15(rand.Reader, svr.privateKey, rsaBuf)
			if err != nil {
				LogPrint("Decrypt aes key failed", err)
				_ = conn.Close()
				return nil
			}
		} else {
			_, err := conn.Write([]byte("nokey"))
			if err != nil {
				LogPrint(err)
				_ = conn.Close()
				return nil
			}
		}

		sid := atomic.AddUint64(&svr.clientSIDAtomic, 1)
		newSession.Init(conn, sid, svr.RemoveSession, aesKey)

		svr.clientSessionMap.Store(sid, newSession)

		if svr.onAddSession != nil {
			svr.onAddSession(newSession.sid)
		}

		newSession.Start()
		return newSession
	}
	return nil
}

func (svr *TcpServer) SetAddSessionCallBack(cb SessionCallBackFunc) {
	svr.onAddSession = cb
}

func (svr *TcpServer) RemoveSession(cli *TcpServerSession) {
	if svr.onRemoveSession != nil {
		svr.onRemoveSession(cli.sid)
	}
	svr.clientSessionMap.Delete(cli.sid)
	svr.sessionPool.Put(cli)
}

func (svr *TcpServer) SetRemoveSessionCallBack(cb SessionCallBackFunc) {
	svr.onRemoveSession = cb
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
