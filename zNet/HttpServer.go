package zNet

import (
	"crypto/rsa"
	"errors"
	"net/http"
	"sync"
	"sync/atomic"

	"github.com/pzqf/zUtil/zMap"
)

type HttpServer struct {
	clientSIDAtomic  SessionIdType
	server           *http.Server
	mux              *http.ServeMux
	clientSessionMap *zMap.TypedMap[SessionIdType, *HttpSession]
	wg               sync.WaitGroup
	onAddSession     SessionCallBackFunc
	onRemoveSession  SessionCallBackFunc
	privateKey       *rsa.PrivateKey
	config           *HttpConfig
	dispatcher       HandlerFun
	logger           Logger
	// 防DDoS相关
	ddosProtection *DDoSProtection
}

func NewHttpServer(cfg *HttpConfig, opts ...Options) *HttpServer {
	svr := &HttpServer{
		clientSIDAtomic:  10000,
		clientSessionMap: zMap.NewTypedMap[SessionIdType, *HttpSession](),
		config:           cfg,
		mux:              http.NewServeMux(),
		// 初始化防DDoS攻击机制
		ddosProtection: NewDDoSProtection(),
	}

	for _, opt := range opts {
		opt(svr)
	}

	// 注册默认的HTTP处理器
	svr.mux.HandleFunc("/", svr.handleHTTP)

	return svr
}

func (svr *HttpServer) Start() error {
	svr.server = &http.Server{
		Addr:    svr.config.ListenAddress,
		Handler: svr.mux,
	}

	svr.wg.Add(1)
	go func() {
		defer svr.wg.Done()

		if svr.logger != nil {
			svr.logger.Info("HTTP server listing on %s", svr.config.ListenAddress)
		}

		err := svr.server.ListenAndServe()
		if err != nil {
			if errors.Is(err, http.ErrServerClosed) {
				if svr.logger != nil {
					svr.logger.Info("HTTP server closed under request")
				}
			} else {
				if svr.logger != nil {
					svr.logger.Error("HTTP server closed unexpected: %v", err)
				}
			}
		}
	}()

	return nil
}

func (svr *HttpServer) Close() {
	if svr.logger != nil {
		svr.logger.Info("Close HTTP server")
	}

	_ = svr.server.Close()
	svr.wg.Wait()

	if svr.logger != nil {
		svr.logger.Info("HTTP server closed")
	}
}

func (svr *HttpServer) HandleFunc(pattern string, handler func(http.ResponseWriter, *http.Request)) {
	svr.mux.HandleFunc(pattern, handler)
}

func (svr *HttpServer) handleHTTP(w http.ResponseWriter, r *http.Request) {
	// 获取客户端IP
	clientIP := r.RemoteAddr

	// 检查是否超过最大客户端数
	if svr.config.MaxClientCount > 0 && int(svr.clientSessionMap.Len()) >= svr.config.MaxClientCount {
		if svr.logger != nil {
			svr.logger.Warn("Maximum HTTP connections exceeded, max:%d", svr.config.MaxClientCount)
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

	// 创建HTTP会话
	sid := atomic.AddUint64(&svr.clientSIDAtomic, 1)
	session := NewHttpSession(w, sid)
	svr.clientSessionMap.Store(sid, session)

	if svr.onAddSession != nil {
		svr.onAddSession(sid)
	}

	// 处理请求
	if svr.dispatcher != nil {
		// 这里需要根据实际情况解析HTTP请求，提取ProtoId和Data
		// 这里只是一个示例，实际实现需要根据你的协议格式来解析
		// 例如，从URL参数、请求体或头部中提取信息
		protoId := int32(0) // 示例值
		data := []byte{}

		netPacket := &NetPacket{
			ProtoId:  protoId,
			Data:     data,
			DataSize: int32(len(data)),
		}

		err := svr.dispatcher(session, netPacket)
		if err != nil {
			if svr.logger != nil {
				svr.logger.Error("Dispatcher error: %v, ProtoId: %d", err, protoId)
			}
			return
		}
	}

	// 移除会话
	defer func() {
		svr.clientSessionMap.Delete(sid)
		if svr.onRemoveSession != nil {
			svr.onRemoveSession(sid)
		}
	}()
}

func (svr *HttpServer) GetSession(sid SessionIdType) *HttpSession {
	if client, ok := svr.clientSessionMap.Load(sid); ok {
		return client
	}
	return nil
}

func (svr *HttpServer) GetAllSession() []*HttpSession {
	var sessionList []*HttpSession
	svr.clientSessionMap.Range(func(sid SessionIdType, value *HttpSession) bool {
		sessionList = append(sessionList, value)
		return true
	})

	return sessionList
}

func (svr *HttpServer) RegisterDispatcher(fun HandlerFun) {
	svr.dispatcher = fun
}
