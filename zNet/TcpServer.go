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
	maxClientCount   int
	address          string
	ClientSessionMap map[int64]*Session
	locker           *sync.RWMutex
	sessionPool      sync.Pool
	clientSIDAtomic  int64
	listener         *net.TCPListener
}

func NewTcpServer(address string, maxClientCount int) *TcpServer {
	svr := TcpServer{
		maxClientCount:   maxClientCount,
		ClientSessionMap: make(map[int64]*Session),
		locker:           &sync.RWMutex{},
		clientSIDAtomic:  10000,
		address:          address,
		sessionPool: sync.Pool{
			New: func() interface{} {
				var s = &Session{}
				return s
			},
		},
	}

	return &svr
}

func InitDefaultTcpServer(address string, maxClientCount int) {
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
			if len(svr.ClientSessionMap) >= svr.maxClientCount {
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
	log.Printf("Close tcp server, session count %d", len(TcpServerInstance.ClientSessionMap))

	_ = svr.listener.Close()

	var list []*Session
	svr.locker.Lock()
	for _, v := range svr.ClientSessionMap {
		list = append(list, v)
	}
	svr.locker.Unlock()

	for _, v := range list {
		v.Close()
	}
}

func (svr *TcpServer) AddClient(conn *net.TCPConn) *Session {
	newSession := svr.sessionPool.Get().(*Session)
	if newSession != nil {
		newSession.Init(conn, atomic.AddInt64(&svr.clientSIDAtomic, 1))

		svr.locker.Lock()
		svr.ClientSessionMap[newSession.sid] = newSession
		svr.locker.Unlock()

		newSession.Start()
		return newSession
	}
	return nil
}

func (svr *TcpServer) DelClient(cli *Session) bool {
	svr.locker.Lock()
	if _, ok := svr.ClientSessionMap[cli.sid]; !ok {
		return false
	}
	svr.sessionPool.Put(cli)
	delete(svr.ClientSessionMap, cli.sid)
	svr.locker.Unlock()

	return true
}

func (svr *TcpServer) GetSession(sid int64) *Session {
	svr.locker.Lock()
	defer svr.locker.Unlock()
	if client, ok := svr.ClientSessionMap[sid]; ok {
		return client
	}

	return nil
}
