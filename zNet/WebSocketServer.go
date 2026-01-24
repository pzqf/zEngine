package zNet

import (
	"crypto/rsa"
	"net/http"
	"sync"
	"sync/atomic"

	"github.com/gorilla/websocket"

	"github.com/pzqf/zUtil/zMap"
)

type WebSocketServer struct {
	clientSIDAtomic  SessionIdType
	upgrader         websocket.Upgrader
	clientSessionMap *zMap.Map
	wg               sync.WaitGroup
	onAddSession     SessionCallBackFunc
	onRemoveSession  SessionCallBackFunc
	privateKey       *rsa.PrivateKey
	config           *WebSocketConfig
	dispatcher       HandlerFun
	logger           Logger
	// 防DDoS相关
	ddosProtection *DDoSProtection
}

func NewWebSocketServer(cfg *WebSocketConfig, opts ...Options) *WebSocketServer {
	if cfg.ChanSize <= 0 {
		cfg.ChanSize = DefaultChanSize
	}

	svr := &WebSocketServer{
		clientSIDAtomic:  10000,
		clientSessionMap: zMap.NewMap(),
		config:           cfg,
		upgrader: websocket.Upgrader{
			CheckOrigin: func(r *http.Request) bool {
				return true // 允许所有来源，生产环境应该根据实际情况设置
			},
		},
		// 初始化防DDoS攻击机制
		ddosProtection: NewDDoSProtection(),
	}

	for _, opt := range opts {
		opt(svr)
	}

	return svr
}

func (svr *WebSocketServer) Start() error {
	http.HandleFunc("/ws", svr.handleWebSocket)

	go func() {
		svr.wg.Add(1)
		defer svr.wg.Done()

		if svr.logger != nil {
			svr.logger.Info("WebSocket server listing on %s", svr.config.ListenAddress)
		}

		if err := http.ListenAndServe(svr.config.ListenAddress, nil); err != nil {
			if svr.logger != nil {
				svr.logger.Error("Failed to start WebSocket server: %v", err)
			}
		}
	}()

	return nil
}

func (svr *WebSocketServer) handleWebSocket(w http.ResponseWriter, r *http.Request) {
	// 获取客户端IP
	clientIP := r.RemoteAddr

	// 检查是否超过最大客户端数
	if svr.config.MaxClientCount > 0 && int(svr.clientSessionMap.Len()) >= svr.config.MaxClientCount {
		if svr.logger != nil {
			svr.logger.Warn("Maximum WebSocket connections exceeded, max:%d", svr.config.MaxClientCount)
		}
		http.Error(w, "Maximum connections exceeded", http.StatusServiceUnavailable)
		return
	}

	// 防DDoS攻击检查
	// 检查是否允许新连接
	if !svr.ddosProtection.AllowConnection(clientIP) {
		if svr.logger != nil {
			svr.logger.Warn("Connection denied by DDoS protection: %s", clientIP)
		}
		http.Error(w, "Too many requests", http.StatusTooManyRequests)
		return
	}

	// 检查流量是否超过限制
	requestSize := int64(0)
	if r.ContentLength > 0 {
		requestSize = r.ContentLength
	}
	if !svr.ddosProtection.AllowTraffic(clientIP, requestSize) {
		if svr.logger != nil {
			svr.logger.Warn("Traffic limit exceeded from IP: %s", clientIP)
		}
		http.Error(w, "Too much traffic", http.StatusTooManyRequests)
		return
	}

	// 检查数据包频率是否超过限制
	if !svr.ddosProtection.AllowPacket(clientIP) {
		if svr.logger != nil {
			svr.logger.Warn("Packet rate limit exceeded from IP: %s", clientIP)
		}
		http.Error(w, "Too many requests", http.StatusTooManyRequests)
		return
	}

	// 升级HTTP连接为WebSocket连接
	conn, err := svr.upgrader.Upgrade(w, r, nil)
	if err != nil {
		if svr.logger != nil {
			svr.logger.Error("Failed to upgrade to WebSocket: %v", err)
		}
		return
	}

	// 创建新会话
	svr.AddSession(conn)
}

func (svr *WebSocketServer) AddSession(conn *websocket.Conn) {
	sid := atomic.AddUint64(&svr.clientSIDAtomic, 1)
	newSession := NewWebSocketServerSession(svr, conn, sid, svr.RemoveSession)
	svr.clientSessionMap.Store(sid, newSession)

	if svr.onAddSession != nil {
		svr.onAddSession(newSession.sid)
	}

	newSession.Start()
}

func (svr *WebSocketServer) Close() {
	if svr.logger != nil {
		svr.logger.Info("Close WebSocket server, session count: %d", svr.clientSessionMap.Len())
	}

	svr.clientSessionMap.Range(func(key, value interface{}) bool {
		session := value.(*WebSocketServerSession)
		session.Close()
		svr.clientSessionMap.Delete(session.sid)
		return true
	})

	svr.wg.Wait()

	if svr.logger != nil {
		svr.logger.Info("WebSocket server closed")
	}
}

func (svr *WebSocketServer) RemoveSession(cli *WebSocketServerSession) {
	if svr.onRemoveSession != nil {
		svr.onRemoveSession(cli.sid)
	}
	svr.clientSessionMap.Delete(cli.sid)
}

func (svr *WebSocketServer) GetSession(sid int64) *WebSocketServerSession {
	if client, ok := svr.clientSessionMap.Get(sid); ok {
		return client.(*WebSocketServerSession)
	}
	return nil
}

func (svr *WebSocketServer) GetAllSession() []*WebSocketServerSession {
	var sessionList []*WebSocketServerSession
	svr.clientSessionMap.Range(func(key, value interface{}) bool {
		sessionList = append(sessionList, value.(*WebSocketServerSession))
		return true
	})

	return sessionList
}

func (svr *WebSocketServer) RegisterDispatcher(fun HandlerFun) {
	svr.dispatcher = fun
}
