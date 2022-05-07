package zNet

import (
	"fmt"
	"log"
	"net"
	"sync"
	"sync/atomic"
	"time"
)

var TcpServerInstance *TcpServer

type TcpServer struct {
	maxClientCount   int
	ListenPort       int
	ListenIp         string
	ClientSessionMap map[int64]*Session
	locker           *sync.RWMutex
	sessionPool      sync.Pool
	clientSIDAtomic  int64
	listener         *net.TCPListener
}

func InitTcpServer(ip string, port int, maxClientCount int) {
	svr := TcpServer{
		maxClientCount:   maxClientCount,
		ClientSessionMap: make(map[int64]*Session),
		locker:           &sync.RWMutex{},
		clientSIDAtomic:  10000,
		ListenPort:       port,
		ListenIp:         ip,
		sessionPool: sync.Pool{
			New: func() interface{} {
				var s = &Session{}
				return s
			},
		},
	}
	TcpServerInstance = &svr
	return
}

func StartTcpServer() error {
	var strRemote = fmt.Sprintf("%s:%d", TcpServerInstance.ListenIp, TcpServerInstance.ListenPort)
	tcpAddr, err := net.ResolveTCPAddr("tcp4", strRemote)
	if err != nil {
		return err
	}
	listener, err := net.ListenTCP("tcp4", tcpAddr)
	if err != nil {
		return err
	}
	TcpServerInstance.listener = listener

	go func() {
		for {
			if len(TcpServerInstance.ClientSessionMap) >= TcpServerInstance.maxClientCount {
				log.Printf("Connects over max maxClientCount %d", TcpServerInstance.maxClientCount)
				time.Sleep(10 * time.Millisecond)
				continue
			}
			conn, err := TcpServerInstance.listener.AcceptTCP()
			if err != nil {
				log.Printf(err.Error())
				break
			}

			TcpServerInstance.AddClient(conn)
		}
	}()
	return nil
}

func CloseTcpServer() {
	log.Printf("Close tcp server, session count %d", len(TcpServerInstance.ClientSessionMap))

	_ = TcpServerInstance.listener.Close()

	var list []*Session
	TcpServerInstance.locker.Lock()
	for _, v := range TcpServerInstance.ClientSessionMap {
		list = append(list, v)
	}
	TcpServerInstance.locker.Unlock()

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
