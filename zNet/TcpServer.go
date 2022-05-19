package zNet

import (
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
	onRemoveSession  RemoveSessionCallBackFunc
}

type RemoveSessionCallBackFunc func(sid SessionIdType)

func NewTcpServer(address string, maxClientCount int32, cb RemoveSessionCallBackFunc) *TcpServer {
	svr := TcpServer{
		maxClientCount:   maxClientCount,
		clientSIDAtomic:  10000,
		address:          address,
		clientSessionMap: zMap.NewMap(),
		onRemoveSession:  cb,
		sessionPool: sync.Pool{
			New: func() interface{} {
				var s = &Session{}
				return s
			},
		},
	}

	return &svr
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
				log.Printf("Connects over max maxClientCount %d", svr.maxClientCount)
				time.Sleep(10 * time.Millisecond)
				continue
			}
			conn, err := svr.listener.AcceptTCP()
			if err != nil {
				log.Printf(err.Error())
				break
			}

			svr.AddSession(conn)
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
		sid := SessionIdType(atomic.AddInt64(&svr.clientSIDAtomic, 1))
		newSession.Init(conn, sid, svr.RemoveSession)

		svr.clientSessionMap.Store(sid, newSession)
		newSession.Start()
		return newSession
	}
	return nil
}

func (svr *TcpServer) RemoveSession(cli *Session) {
	if svr.onRemoveSession != nil {
		svr.onRemoveSession(cli.sid)
	}
	svr.clientSessionMap.Delete(cli.sid)
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

func (svr *TcpServer) BroadcastToClient(protoId int32, data interface{}) {
	svr.clientSessionMap.Range(func(key, value interface{}) bool {
		session := value.(*Session)
		_ = session.Send(protoId, data)
		return true
	})
}
