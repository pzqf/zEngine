package zNet

import (
	"log"
	"net"
	"sync"
	"sync/atomic"
	"time"
)

type TcpServer struct {
	maxClientCount      int32
	address             string
	sessionPool         sync.Pool
	clientSIDAtomic     int64
	listener            *net.TCPListener
	clientSessionMap    sync.Map
	clientSessionCount  int32
	clientSessionOpChan chan sessionOption
	exitChan            chan bool
	wg                  sync.WaitGroup
}

type sessionOption struct {
	option  int32 //1 add ,-1 del
	session *Session
}

func NewTcpServer(address string, maxClientCount int32) *TcpServer {
	svr := TcpServer{
		maxClientCount:      maxClientCount,
		clientSIDAtomic:     10000,
		address:             address,
		clientSessionCount:  0,
		clientSessionOpChan: make(chan sessionOption, 512),
		exitChan:            make(chan bool, 1),
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

	go func() {
		svr.wg.Add(1)
		running := true
		for {
			select {
			case op := <-svr.clientSessionOpChan:
				if op.option == 1 {
					svr.clientSessionMap.Store(op.session.sid, op.session)
				} else if op.option == -1 {
					svr.clientSessionMap.Delete(op.session.sid)
				}
			case <-svr.exitChan:
				running = false
				break
			}
			if !running {
				break
			}
		}
		svr.wg.Done()
	}()

	return nil
}

func (svr *TcpServer) Close() {
	log.Printf("Close tcp server, session count %d", svr.clientSessionCount)

	_ = svr.listener.Close()

	svr.clientSessionMap.Range(func(key, value interface{}) bool {
		session := value.(*Session)
		session.Close()
		log.Printf("Close  session  %d", session.sid)
		svr.clientSessionMap.Delete(session.sid)
		return true
	})

	atomic.StoreInt32(&svr.clientSessionCount, 0)

	svr.exitChan <- true
	svr.wg.Done()
}

func (svr *TcpServer) AddClient(conn *net.TCPConn) *Session {
	newSession := svr.sessionPool.Get().(*Session)
	if newSession != nil {
		newSession.Init(conn, atomic.AddInt64(&svr.clientSIDAtomic, 1), svr)

		svr.clientSessionOpChan <- sessionOption{option: 1, session: newSession}
		atomic.AddInt32(&svr.clientSessionCount, 1)

		newSession.Start()
		return newSession
	}
	return nil
}

func (svr *TcpServer) DelClient(cli *Session) bool {
	svr.clientSessionOpChan <- sessionOption{option: -1, session: cli}
	atomic.AddInt32(&svr.clientSessionCount, -1)
	return true
}

func (svr *TcpServer) GetSession(sid int64) *Session {
	if client, ok := svr.clientSessionMap.Load(sid); ok {
		return client.(*Session)
	}
	return nil
}

func (svr *TcpServer) BroadcastToClient(netPacket *NetPacket) {
	TcpServerInstance.clientSessionMap.Range(func(key, value interface{}) bool {
		session := value.(*Session)
		_ = session.Send(netPacket)
		return true
	})
}
