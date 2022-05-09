package zNet

import (
	"log"
	"net"
	"sync"
	"sync/atomic"
	"time"
)

var TcpServerInstance *TcpServer

type TcpServer struct {
	maxClientCount     int32
	address            string
	sessionPool        sync.Pool
	clientSIDAtomic    int64
	listener           *net.TCPListener
	ClientSessionMap   sync.Map
	clientSessionCount int32
}

func NewTcpServer(address string, maxClientCount int32) *TcpServer {
	svr := TcpServer{
		maxClientCount:  maxClientCount,
		clientSIDAtomic: 10000,
		address:         address,
		sessionPool: sync.Pool{
			New: func() interface{} {
				var s = &Session{}
				return s
			},
		},
		clientSessionCount: 0,
	}

	return &svr
}

func InitDefaultTcpServer(address string, maxClientCount int32) {
	TcpServerInstance = NewTcpServer(address, maxClientCount)
	return
}

func StartTcpServer() error {
	return TcpServerInstance.Start()
}

func CloseTcpServer() {
	TcpServerInstance.Close()
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
		for {
			if svr.clientSessionCount >= svr.maxClientCount {
				log.Printf("Connects over max maxClientCount %d", svr.maxClientCount)
				time.Sleep(10 * time.Millisecond)
				continue
			}
			conn, err := svr.listener.AcceptTCP()
			if err != nil {
				log.Printf(err.Error())
				break
			}

			svr.AddClient(conn)
		}
	}()
	return nil
}

func (svr *TcpServer) Close() {
	log.Printf("Close tcp server, session count %d", svr.clientSessionCount)

	_ = svr.listener.Close()

	svr.ClientSessionMap.Range(func(key, value interface{}) bool {
		session := value.(*Session)
		session.Close()
		return true
	})
	atomic.StoreInt32(&svr.clientSessionCount, 0)
}

func (svr *TcpServer) AddClient(conn *net.TCPConn) *Session {
	newSession := svr.sessionPool.Get().(*Session)
	if newSession != nil {
		newSession.Init(conn, atomic.AddInt64(&svr.clientSIDAtomic, 1))

		svr.ClientSessionMap.Store(newSession.sid, newSession)
		atomic.AddInt32(&svr.clientSessionCount, 1)

		newSession.Start()
		return newSession
	}
	return nil
}

func (svr *TcpServer) DelClient(cli *Session) bool {

	svr.ClientSessionMap.Delete(cli.sid)
	atomic.AddInt32(&svr.clientSessionCount, -1)
	return true
}

func (svr *TcpServer) GetSession(sid int64) *Session {
	if client, ok := svr.ClientSessionMap.Load(sid); ok {
		return client.(*Session)
	}

	return nil
}
