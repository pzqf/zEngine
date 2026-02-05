package zNet

import (
	"crypto/rsa"
	"net"
	"sync"
	"sync/atomic"
	"time"

	"github.com/pzqf/zUtil/zMap"
)

// TcpServer TCP服务器实现
// 支持多客户端连接、会话管理、DDoS防护、密钥交换等功能
type TcpServer struct {
	clientSIDAtomic  SessionIdType                              // 会话ID原子计数器，用于生成唯一会话ID
	listener         *net.TCPListener                           // TCP监听器
	clientSessionMap *zMap.TypedMap[SessionIdType, *TcpServerSession] // 客户端会话映射表
	wg               sync.WaitGroup                             // 等待组，用于优雅关闭
	onAddSession     SessionCallBackFunc                        // 会话添加回调
	onRemoveSession  SessionCallBackFunc                        // 会话移除回调
	privateKey       *rsa.PrivateKey                            // RSA私钥（用于密钥交换）
	config           *TcpConfig                                 // 服务器配置
	dispatcher       HandlerFun                                 // 消息处理器
	logger           Logger                                     // 日志记录器
	ddosProtection   *DDoSProtection                            // DDoS防护组件
}

// NewTcpServer 创建新的TCP服务器实例
// 参数:
//   - cfg: TCP服务器配置
//   - opts: 可选配置项（函数式选项模式）
//
// 返回:
//   - *TcpServer: TCP服务器实例
func NewTcpServer(cfg *TcpConfig, opts ...Options) *TcpServer {
	if cfg.ChanSize <= 0 {
		cfg.ChanSize = DefaultChanSize
	}

	svr := &TcpServer{
		clientSIDAtomic:  10000,
		clientSessionMap: zMap.NewTypedMap[SessionIdType, *TcpServerSession](),
		config:           cfg,
		ddosProtection: NewDDoSProtection(),
	}

	for _, opt := range opts {
		opt(svr)
	}

	return svr
}

// Start 启动TCP服务器
// 开始监听指定地址并接受客户端连接
//
// 返回:
//   - error: 启动失败时返回错误
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

	svr.wg.Add(1)
	go func() {
		defer svr.wg.Done()
		for {
			// 检查是否达到最大客户端数量
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

// Close 关闭TCP服务器
// 关闭监听器、断开所有客户端连接并等待所有goroutine退出
func (svr *TcpServer) Close() {
	if svr.logger != nil {
		svr.logger.Info("Close tcp server, session count: %d", svr.clientSessionMap.Len())
	}

	// 关闭监听器，停止接受新连接
	if err := svr.listener.Close(); err != nil {
		if svr.logger != nil {
			svr.logger.Error("Failed to close listener: %v", err)
		}
	}

	// 关闭所有客户端会话
	svr.clientSessionMap.Range(func(sid SessionIdType, value *TcpServerSession) bool {
		session := value
		session.Close()
		svr.clientSessionMap.Delete(session.sid)
		return true
	})

	// 等待所有goroutine退出
	svr.wg.Wait()

	if svr.logger != nil {
		svr.logger.Info("Tcp server closed")
	}
}

// AddSession 添加新的客户端会话
// 执行DH密钥交换并创建新的会话实例
//
// 参数:
//   - conn: TCP连接
func (svr *TcpServer) AddSession(conn *net.TCPConn) {
	// 执行DH密钥协商，生成AES密钥
	aesKey, err := PerformKeyExchange(conn)
	if err != nil {
		if svr.logger != nil {
			svr.logger.Error("DH key exchange failed: %v", err)
		}
		conn.Close()
		return
	}

	if svr.logger != nil {
		svr.logger.Info("DH key exchange completed successfully, AES key length: %d", len(aesKey))
	}

	// 生成唯一会话ID并创建会话
	sid := atomic.AddUint64(&svr.clientSIDAtomic, 1)
	newSession := NewTcpServerSession(svr, conn, sid, svr.RemoveSession, aesKey)
	svr.clientSessionMap.Store(sid, newSession)
	if svr.onAddSession != nil {
		svr.onAddSession(newSession.sid)
	}
	newSession.Start()
}

// RemoveSession 移除客户端会话
// 从会话映射中删除指定会话并触发回调
//
// 参数:
//   - cli: 要移除的客户端会话
func (svr *TcpServer) RemoveSession(cli *TcpServerSession) {
	if svr.onRemoveSession != nil {
		svr.onRemoveSession(cli.sid)
	}
	svr.clientSessionMap.Delete(cli.sid)
}

// GetSession 根据会话ID获取客户端会话
//
// 参数:
//   - sid: 会话ID
//
// 返回:
//   - *TcpServerSession: 客户端会话，不存在时返回nil
func (svr *TcpServer) GetSession(sid SessionIdType) *TcpServerSession {
	if client, ok := svr.clientSessionMap.Load(sid); ok {
		return client
	}
	return nil
}

// GetAllSession 获取所有客户端会话
//
// 返回:
//   - []*TcpServerSession: 所有客户端会话列表
func (svr *TcpServer) GetAllSession() []*TcpServerSession {
	var sessionList []*TcpServerSession
	svr.clientSessionMap.Range(func(sid SessionIdType, value *TcpServerSession) bool {
		sessionList = append(sessionList, value)
		return true
	})

	return sessionList
}

// RegisterDispatcher 注册消息分发处理器
//
// 参数:
//   - fun: 消息处理函数
func (svr *TcpServer) RegisterDispatcher(fun HandlerFun) {
	svr.dispatcher = fun
}
