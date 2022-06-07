package zNet

import (
	"crypto/rand"
	"crypto/rsa"
	"io"
	"log"
	"net"
	"sync"
	"sync/atomic"
	"time"

	"github.com/pzqf/zUtil/zMap"
)

type TcpServer struct {
	maxClientCount   int32
	address          string
	sessionPool      sync.Pool
	clientSIDAtomic  int64
	listener         *net.TCPListener
	clientSessionMap zMap.Map
	wg               sync.WaitGroup
	onAddSession     SessionCallBackFunc
	onRemoveSession  SessionCallBackFunc
	privateKey       *rsa.PrivateKey
}

type SessionCallBackFunc func(sid SessionIdType)

type Options func(*TcpServer)

func NewTcpServer(address string, opts ...Options) *TcpServer {
	svr := &TcpServer{
		maxClientCount:   10000,
		clientSIDAtomic:  10000,
		address:          address,
		clientSessionMap: zMap.NewMap(),
		sessionPool: sync.Pool{
			New: func() interface{} {
				var s = &Session{}
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
	tcpAddr, err := net.ResolveTCPAddr("tcp4", svr.address)
	if err != nil {
		return err
	}
	listener, err := net.ListenTCP("tcp4", tcpAddr)
	if err != nil {
		return err
	}
	svr.listener = listener

	go func() {
		svr.wg.Add(1)
		defer svr.wg.Done()
		for {
			if svr.clientSessionMap.Len() >= svr.maxClientCount {
				log.Printf("Maximum connections exceeded, max:%d", svr.maxClientCount)
				time.Sleep(10 * time.Millisecond)
				continue
			}
			conn, err := svr.listener.AcceptTCP()
			if err != nil {
				log.Printf(err.Error())
				break
			}

			go svr.AddSession(conn)
		}
	}()

	return nil
}

func (svr *TcpServer) Close() {
	log.Printf("Close tcp server, session count %d", svr.clientSessionMap.Len())

	_ = svr.listener.Close()

	svr.clientSessionMap.Range(func(key, value interface{}) bool {
		session := value.(*Session)
		session.Close()
		svr.clientSessionMap.Delete(session.sid)
		return true
	})

	svr.wg.Wait()
}

func (svr *TcpServer) AddSession(conn *net.TCPConn) *Session {
	newSession := svr.sessionPool.Get().(*Session)
	if newSession != nil {
		var aesKey []byte
		if svr.privateKey != nil {
			_, err := conn.Write([]byte("hello"))
			if err != nil {
				return nil
			}

			rsaBuf := make([]byte, 256)
			_, _ = io.ReadFull(conn, rsaBuf)

			aesKey, err = rsa.DecryptPKCS1v15(rand.Reader, svr.privateKey, rsaBuf)
			if err != nil {
				log.Println("Decrypt aes key failed")
				return nil
			}
		} else {
			_, err := conn.Write([]byte("nokey"))
			if err != nil {
				return nil
			}
		}

		sid := SessionIdType(atomic.AddInt64(&svr.clientSIDAtomic, 1))
		newSession.Init(conn, sid, svr.RemoveSession, aesKey)
		//fmt.Println(sid, "aesKey", string(aesKey))

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

func (svr *TcpServer) RemoveSession(cli *Session) {
	if svr.onRemoveSession != nil {
		svr.onRemoveSession(cli.sid)
	}
	svr.clientSessionMap.Delete(cli.sid)
}

func (svr *TcpServer) SetRemoveSessionCallBack(cb SessionCallBackFunc) {
	svr.onRemoveSession = cb
}

func (svr *TcpServer) GetSession(sid int64) *Session {
	if client, ok := svr.clientSessionMap.Get(sid); ok {
		return client.(*Session)
	}
	return nil
}

func (svr *TcpServer) GetAllSession() []*Session {
	var sessionList []*Session
	svr.clientSessionMap.Range(func(key, value interface{}) bool {
		sessionList = append(sessionList, value.(*Session))
		return true
	})

	return sessionList
}

func (svr *TcpServer) BroadcastToClient(protoId int32, data []byte) {
	svr.clientSessionMap.Range(func(key, value interface{}) bool {
		session := value.(*Session)
		_ = session.Send(protoId, data)
		return true
	})
}
