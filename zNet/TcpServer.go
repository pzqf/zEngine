package zNet

import (
	"crypto/rsa"
	"net"
	"sync"
	"sync/atomic"
	"time"

	"github.com/pzqf/zUtil/zConcurrency"
	"github.com/pzqf/zUtil/zMap"
)

// TcpServer TCP服务器实现
// 支持多客户端连接、会话管理、DDoS防护、密钥交换等功能
type TcpServer struct {
	clientSIDAtomic   SessionIdType                                    // 会话ID原子计数器，用于生成唯一会话ID
	listener          atomic.Pointer[net.TCPListener]                  // TCP监听器（原子读写：Start 写，Accept/Close/GetListenAddress 读，消除数据竞争）
	closing           atomic.Bool                                      // 关服信号：置位后 accept 循环与 AddSession 停止注册新会话
	clientSessionMap  *zMap.TypedMap[SessionIdType, *TcpServerSession] // 客户端会话映射表
	wg                sync.WaitGroup                                   // 等待组，用于优雅关闭
	onAddSession      SessionCallBackFunc                              // 会话添加回调
	onRemoveSession   SessionCallBackFunc                              // 会话移除回调
	privateKey        *rsa.PrivateKey                                  // RSA私钥（用于密钥交换）
	config            *TcpConfig                                       // 服务器配置
	dispatcher        HandlerFun                                       // 消息处理器
	logger            Logger                                           // 日志记录器
	ddosProtection    *DDoSProtection                                  // DDoS防护组件
	compressionConfig *CompressionConfig                               // 压缩配置
	workerPool        *zConcurrency.WorkerPool                         // 工作池
	keyRotationMgr    *KeyRotationManager                              // 密钥轮换管理器
	sequenceManager   *SequenceManager                                 // 序列号管理器
	metrics           NetworkMetricsRecorder                           // 可选网络指标上报器（nil 则不上报）
	packetCodec       PacketCodec
	packetCodecErr    error
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
	normalizePacketSizeLimits(&cfg.MaxWirePacketSize, &cfg.MaxPacketDataSize, &cfg.MaxDecodedPacketSize)

	// 设置默认工作池参数
	if cfg.WorkerPoolSize <= 0 {
		cfg.WorkerPoolSize = 100
	}
	if cfg.WorkerQueueSize <= 0 {
		cfg.WorkerQueueSize = 10000
	}

	svr := &TcpServer{
		clientSIDAtomic:   10000,
		clientSessionMap:  zMap.NewTypedMap[SessionIdType, *TcpServerSession](),
		config:            cfg,
		ddosProtection:    NewDDoSProtection(),
		compressionConfig: DefaultCompressionConfig(),
	}

	// 根据配置创建工作池
	if cfg.UseWorkerPool {
		svr.workerPool = zConcurrency.NewWorkerPool(cfg.WorkerPoolSize, cfg.WorkerQueueSize)
		svr.workerPool.Start()
	}

	// 初始化密钥轮换管理器
	if cfg.EnableKeyRotation && cfg.KeyRotationInterval > 0 {
		if cfg.MaxHistoryKeys <= 0 {
			cfg.MaxHistoryKeys = 3
		}
		svr.keyRotationMgr = NewKeyRotationManager(cfg.KeyRotationInterval, cfg.MaxHistoryKeys)
	}

	// 初始化序列号管理器
	if cfg.EnableSequenceCheck {
		if cfg.SequenceWindowSize <= 0 {
			cfg.SequenceWindowSize = 1000
		}
		if cfg.TimestampTolerance <= 0 {
			cfg.TimestampTolerance = 30
		}
		svr.sequenceManager = NewSequenceManager(cfg.SequenceWindowSize, cfg.TimestampTolerance)
	}

	for _, opt := range opts {
		opt(svr)
	}
	normalizePacketSizeLimits(&cfg.MaxWirePacketSize, &cfg.MaxPacketDataSize, &cfg.MaxDecodedPacketSize)
	svr.packetCodec, svr.packetCodecErr = newEndpointPacketCodec(cfg.ByteOrder)

	return svr
}

// Start 启动TCP服务器
// 开始监听指定地址并接受客户端连接
//
// 返回:
//   - error: 启动失败时返回错误
func (svr *TcpServer) Start() error {
	if svr.packetCodecErr != nil {
		return svr.packetCodecErr
	}
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
	svr.listener.Store(listener)
	svr.closing.Store(false)

	if svr.logger != nil {
		svr.logger.Info("Tcp server listing on %s", svr.config.ListenAddress)
	}

	// NET-6: 自动密钥轮换未完整实现——轮换只向客户端发新 KeyID、不下发新密钥材料（缺安全密钥
	// 下发信道），一旦轮换服务端换新 key 而客户端仍持旧 key → 双方失配、加解密全乱。故暂不启用：
	// 即便配置开启也只告警不启动，会话保持握手协商的稳定密钥。待实现安全再密钥（如 DH 再交换）恢复。
	if svr.config.EnableKeyRotation && svr.config.KeyRotationInterval > 0 {
		if svr.logger != nil {
			svr.logger.Warn("EnableKeyRotation is set but auto key rotation is NOT started: feature incomplete (no secure key delivery to clients would desync keys). Sessions keep the handshake key. See NET-6")
		}
	}

	svr.wg.Add(1)
	go func() {
		defer svr.wg.Done()
		for {
			// 关服信号：满员空转分支必须检查 closing 并退出，否则 accept goroutine 永不
			// 退出、Close 的 wg.Wait 永久挂起（满员时关服死锁）。
			if svr.closing.Load() {
				break
			}
			// 检查是否达到最大客户端数量
			if svr.config.MaxClientCount > 0 && int(svr.clientSessionMap.Len()) >= svr.config.MaxClientCount {
				if svr.logger != nil {
					svr.logger.Warn("Maximum connections exceeded, max:%d", svr.config.MaxClientCount)
				}
				time.Sleep(5 * time.Millisecond)
				continue
			}
			conn, err := svr.listener.Load().AcceptTCP()
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

			// 纳入 svr.wg 跟踪：Close 的 wg.Wait() 会等待所有在飞 AddSession 完成注册，
			// 之后再统一关闭会话，避免"关闭后又有会话被 Store"的时序竞争与 goroutine 泄漏。
			svr.wg.Add(1)
			go func(c *net.TCPConn) {
				defer svr.wg.Done()
				svr.AddSession(c)
			}(conn)
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

	// 1) 置关服信号：accept 循环与 AddSession 不再注册新会话。
	svr.closing.Store(true)

	// 2) 关闭监听器，令 accept 循环的 AcceptTCP 立即返回错误而退出。
	if l := svr.listener.Load(); l != nil {
		if err := l.Close(); err != nil {
			if svr.logger != nil {
				svr.logger.Error("Failed to close listener: %v", err)
			}
		}
	}

	// 3) 等待 accept 循环 + 所有在飞 AddSession goroutine 退出。此后 clientSessionMap
	//    不再有新会话写入，可安全遍历关闭——消除"关闭后又被 Store"的时序竞争。
	svr.wg.Wait()

	// 4) 关闭所有客户端会话（session.Close 内部同步等待该会话的收发 goroutine 退出）。
	svr.clientSessionMap.Range(func(sid SessionIdType, value *TcpServerSession) bool {
		value.Close()
		svr.clientSessionMap.Delete(sid)
		return true
	})

	// 5) 会话已全部停止、不再向工作池投递，最后停工作池。
	if svr.workerPool != nil {
		svr.workerPool.Stop()
	}

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
	// 关服中：不再接纳新会话，直接关连接返回（避免在关闭序列中被 Store 后遗漏）。
	if svr.closing.Load() {
		conn.Close()
		return
	}

	var aesKey []byte
	var err error

	if svr.config.TcpNoDelay {
		_ = conn.SetNoDelay(true)
	}

	if svr.config.WriteBufferSize > 0 {
		_ = conn.SetWriteBuffer(svr.config.WriteBufferSize)
	}

	if svr.config.ReadBufferSize > 0 {
		_ = conn.SetReadBuffer(svr.config.ReadBufferSize)
	}

	if !svr.config.DisableEncryption {
		// 执行DH密钥协商，生成AES密钥（带握手超时，防对端不配合导致永久阻塞）
		aesKey, err = PerformKeyExchangeWithDeadline(conn, DefaultKeyExchangeTimeout)
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
	} else {
		if svr.logger != nil {
			svr.logger.Info("Encryption disabled, skipping key exchange")
		}
	}

	// 握手可能耗时（DH+超时），期间若已进入关服则放弃注册，加速关闭并避免遗漏会话。
	if svr.closing.Load() {
		conn.Close()
		return
	}

	// 生成唯一会话ID并创建会话
	sid := atomic.AddUint64(&svr.clientSIDAtomic, 1)
	newSession := NewTcpServerSession(svr, conn, sid, svr.RemoveSession, aesKey)
	svr.clientSessionMap.Store(sid, newSession)

	if svr.logger != nil {
		clientAddr := conn.RemoteAddr().String()
		svr.logger.Info("New client connected, sid=%d, client=%s, total_clients=%d", sid, clientAddr, svr.clientSessionMap.Len())
	}

	if svr.metrics != nil {
		svr.metrics.IncActiveConnections()
	}

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
	if svr.metrics != nil {
		svr.metrics.DecActiveConnections()
	}

	if svr.onRemoveSession != nil {
		svr.onRemoveSession(cli.sid)
	}

	// 从序列号管理器中移除会话
	if svr.sequenceManager != nil {
		svr.sequenceManager.RemoveSession(cli.sid)
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

// GetListenAddress 获取服务器实际监听地址
//
// 返回:
//   - string: 服务器监听地址
func (svr *TcpServer) GetListenAddress() string {
	l := svr.listener.Load()
	if l == nil {
		return ""
	}
	return l.Addr().String()
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

// SetOnAddSession 设置会话添加回调
//
// 参数:
//   - fun: 会话添加回调函数
func (svr *TcpServer) SetOnAddSession(fun SessionCallBackFunc) {
	svr.onAddSession = fun
}

// SetOnRemoveSession 设置会话移除回调
//
// 参数:
//   - fun: 会话移除回调函数
func (svr *TcpServer) SetOnRemoveSession(fun SessionCallBackFunc) {
	svr.onRemoveSession = fun
}

// StartKeyRotation 启动密钥轮换（手动触发）
func (svr *TcpServer) StartKeyRotation() {
	if svr.keyRotationMgr == nil {
		return
	}

	newKey, newKeyID := svr.keyRotationMgr.Rotate()

	// 通知所有会话更新密钥
	svr.clientSessionMap.Range(func(sid SessionIdType, session *TcpServerSession) bool {
		session.UpdateKey(newKey, newKeyID)
		return true
	})

	if svr.logger != nil {
		svr.logger.Info("Key rotation started, new key ID: %d", newKeyID)
	}
}

// StartAutoKeyRotation 启动自动密钥轮换
func (svr *TcpServer) StartAutoKeyRotation() {
	if svr.keyRotationMgr == nil {
		return
	}

	// 启动自动轮换
	svr.keyRotationMgr.StartAutoRotation(func() []*TcpServerSession {
		return svr.GetAllSession()
	})
}
