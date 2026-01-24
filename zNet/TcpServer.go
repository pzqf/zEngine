package zNet

import (
	"crypto/rsa"
	"net"
	"sync"
	"sync/atomic"
	"time"

	"github.com/pzqf/zUtil/zMap"
)

type TcpServer struct {
	clientSIDAtomic  SessionIdType
	listener         *net.TCPListener
	clientSessionMap *zMap.Map
	wg               sync.WaitGroup
	onAddSession     SessionCallBackFunc
	onRemoveSession  SessionCallBackFunc
	privateKey       *rsa.PrivateKey
	config           *TcpConfig
	dispatcher       HandlerFun
	logger           Logger
	// 防DDoS相关
	ddosProtection *DDoSProtection
}

func NewTcpServer(cfg *TcpConfig, opts ...Options) *TcpServer {
	if cfg.ChanSize <= 0 {
		cfg.ChanSize = DefaultChanSize
	}

	svr := &TcpServer{
		clientSIDAtomic:  10000,
		clientSessionMap: zMap.NewMap(),
		config:           cfg,
		// 初始化防DDoS攻击机制
		ddosProtection: NewDDoSProtection(),
	}

	for _, opt := range opts {
		opt(svr)
	}

	return svr
}

func (svr *TcpServer) Start() error {
	tcpAddr, err := net.ResolveTCPAddr("tcp4", svr.config.ListenAddress)
	if err != nil {
		if svr.logger != nil {
			svr.logger.Error("Failed to resolve TCP address: %v", err)
		}
		return err
	}
	listener, err := net.ListenTCP("tcp4", tcpAddr)
	if err != nil {
		if svr.logger != nil {
			svr.logger.Error("Failed to listen TCP: %v", err)
		}
		return err
	}
	svr.listener = listener

	if svr.logger != nil {
		svr.logger.Info("Tcp server listing on %s", svr.config.ListenAddress)
	}

	go func() {
		svr.wg.Add(1)
		defer svr.wg.Done()
		for {
			if svr.config.MaxClientCount > 0 && int(svr.clientSessionMap.Len()) >= svr.config.MaxClientCount {
				if svr.logger != nil {
					svr.logger.Warn("Maximum connections exceeded, max:%d", svr.config.MaxClientCount)
				}
				time.Sleep(5 * time.Millisecond)
				continue
			}
			conn, err := svr.listener.AcceptTCP()
			if err != nil {
				if svr.logger != nil {
					svr.logger.Error("Failed to accept TCP connection: %v", err)
				}
				break
			}

			// 防DDoS攻击检查
			clientIP := conn.RemoteAddr().(*net.TCPAddr).IP.String()

			// 检查是否允许新连接
			if !svr.ddosProtection.AllowConnection(clientIP) {
				if svr.logger != nil {
					svr.logger.Warn("Connection denied by DDoS protection: %s", clientIP)
				}
				conn.Close()
				continue
			}

			go svr.AddSession(conn)
		}
	}()

	return nil
}

func (svr *TcpServer) Close() {
	if svr.logger != nil {
		svr.logger.Info("Close tcp server, session count: %d", svr.clientSessionMap.Len())
	}

	if err := svr.listener.Close(); err != nil {
		if svr.logger != nil {
			svr.logger.Error("Failed to close listener: %v", err)
		}
	}

	svr.clientSessionMap.Range(func(key, value interface{}) bool {
		session := value.(*TcpServerSession)
		session.Close()
		svr.clientSessionMap.Delete(session.sid)
		return true
	})

	svr.wg.Wait()

	if svr.logger != nil {
		svr.logger.Info("Tcp server closed")
	}
}

func (svr *TcpServer) AddSession(conn *net.TCPConn) {
	var aesKey []byte
	sid := atomic.AddUint64(&svr.clientSIDAtomic, 1)
	newSession := NewTcpServerSession(svr, conn, sid, svr.RemoveSession, aesKey)
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

func (svr *TcpServer) RegisterDispatcher(fun HandlerFun) {
	svr.dispatcher = fun
}
