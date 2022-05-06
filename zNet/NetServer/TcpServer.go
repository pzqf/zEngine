package NetServer

import (
	"fmt"
	"net"
	"sync"
	"sync/atomic"
	"time"
	"zEngine/zLog"
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
	zLog.Info("Init tcp server ... ")
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

func Start() error {
	var strRemote = fmt.Sprintf("%s:%d", TcpServerInstance.ListenIp, TcpServerInstance.ListenPort)
	tcpAddr, err := net.ResolveTCPAddr("tcp4", strRemote)
	if err != nil {
		zLog.Error(err.Error())
		return err
	}
	listener, err := net.ListenTCP("tcp4", tcpAddr)
	if err != nil {
		zLog.Error(err.Error())
		return err
	}
	TcpServerInstance.listener = listener

	zLog.InfoF("Tcp server listing on %s ", tcpAddr.String())

	go func() {
		for {
			if len(TcpServerInstance.ClientSessionMap) >= TcpServerInstance.maxClientCount {
				zLog.ErrorF("Connects over max maxClientCount %d", TcpServerInstance.maxClientCount)
				time.Sleep(10 * time.Millisecond)
				continue
			}
			conn, err := TcpServerInstance.listener.AcceptTCP()
			if err != nil {
				zLog.Error(err.Error())
				break
			}

			zLog.InfoF("Accept connect from %s", conn.RemoteAddr().String())

			TcpServerInstance.AddClient(conn)
		}
	}()
	return nil
}

func Close() {
	zLog.InfoF("Close tcp server, session count %d", len(TcpServerInstance.ClientSessionMap))

	_ = TcpServerInstance.listener.Close()
	for _, v := range TcpServerInstance.ClientSessionMap {
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
		zLog.InfoF("Add client session, sid:%d ", newSession.sid)

		newSession.Start()
		return newSession
	} else {
		zLog.Info("Can't create Session")
	}
	return nil
}

func (svr *TcpServer) DelClient(cli *Session) bool {
	zLog.InfoF("Delete client session：%d ", cli.sid)

	svr.sessionPool.Put(cli)
	svr.locker.Lock()
	delete(svr.ClientSessionMap, cli.sid)
	svr.locker.Unlock()
	zLog.InfoF("client count：%d ", len(svr.ClientSessionMap))

	//todo
	//notify player offline

	return true
}
