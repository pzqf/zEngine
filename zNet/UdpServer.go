package zNet

import (
	"crypto/rsa"
	"net"
	"sync"
	"sync/atomic"

	"github.com/pzqf/zUtil/zMap"
)

type UdpServer struct {
	clientSIDAtomic  SessionIdType
	listener         *net.UDPConn
	clientSessionMap *zMap.TypedMap[SessionIdType, *UdpServerSession]
	wg               sync.WaitGroup
	onAddSession     SessionCallBackFunc
	onRemoveSession  SessionCallBackFunc
	privateKey       *rsa.PrivateKey
	config           *UdpConfig
	dispatcher       HandlerFun
	logger           Logger
	// 防DDoS相关
	ddosProtection *DDoSProtection
	packetCodec    PacketCodec
	packetCodecErr error
	metrics        NetworkMetricsRecorder
	writeMu        sync.Mutex
}

func NewUdpServer(cfg *UdpConfig, opts ...Options) *UdpServer {
	if cfg.ChanSize <= 0 {
		cfg.ChanSize = DefaultChanSize
	}
	normalizePacketSizeLimits(&cfg.MaxWirePacketSize, &cfg.MaxPacketDataSize, &cfg.MaxDecodedPacketSize)

	svr := &UdpServer{
		clientSIDAtomic:  10000,
		clientSessionMap: zMap.NewTypedMap[SessionIdType, *UdpServerSession](),
		config:           cfg,
		// 初始化防DDoS攻击机制
		ddosProtection: NewDDoSProtection(),
	}

	for _, opt := range opts {
		opt(svr)
	}
	normalizePacketSizeLimits(&cfg.MaxWirePacketSize, &cfg.MaxPacketDataSize, &cfg.MaxDecodedPacketSize)
	svr.packetCodec, svr.packetCodecErr = newEndpointPacketCodec(cfg.ByteOrder)

	return svr
}

func (svr *UdpServer) Start() error {
	if svr.packetCodecErr != nil {
		return svr.packetCodecErr
	}
	udpAddr, err := net.ResolveUDPAddr("udp4", svr.config.ListenAddress)
	if err != nil {
		if svr.logger != nil {
			svr.logger.Error("Failed to resolve UDP address: %v", err)
		}
		return err
	}

	listener, err := net.ListenUDP("udp4", udpAddr)
	if err != nil {
		if svr.logger != nil {
			svr.logger.Error("Failed to listen UDP: %v", err)
		}
		return err
	}
	svr.listener = listener

	if svr.logger != nil {
		svr.logger.Info("Udp server listing on %s", svr.config.ListenAddress)
	}

	svr.wg.Add(1)
	go func() {
		defer svr.wg.Done()

		buffer := make([]byte, 65535) // UDP最大包大小
		for {
			n, addr, err := svr.listener.ReadFromUDP(buffer)
			if err != nil {
				if svr.logger != nil {
					svr.logger.Error("Failed to read from UDP: %v", err)
				}
				break
			}

			// ReadFromUDP 的 buffer 会在下一轮复用，DecodeFrame 的 Data 又引用输入切片，
			// 因此入 session 队列前必须复制。解析按到达顺序同步完成；业务仍由 session.process
			// goroutine 执行，避免 per-packet goroutine 并发修改握手/心跳状态和无界堆积。
			packetData := append([]byte(nil), buffer[:n]...)
			addrCopy := *addr
			svr.handleUdpPacket(&addrCopy, packetData)
		}
	}()

	return nil
}

func (svr *UdpServer) handleUdpPacket(addr *net.UDPAddr, data []byte) {
	// 获取客户端IP
	clientIP := addr.IP.String()

	// 防DDoS攻击检查
	// 检查是否允许新数据包
	if !svr.ddosProtection.AllowPacket(clientIP) {
		if svr.logger != nil {
			svr.logger.Warn("Packet rate limit exceeded from IP: %s", clientIP)
		}
		return
	}

	// 检查流量是否超过限制
	if !svr.ddosProtection.AllowTraffic(clientIP, int64(len(data))) {
		if svr.logger != nil {
			svr.logger.Warn("Traffic limit exceeded from IP: %s", clientIP)
		}
		return
	}

	// 查找或创建会话
	session, exists := svr.getSessionByAddr(addr)
	if !exists {
		// 检查是否允许新连接
		if !svr.ddosProtection.AllowConnection(clientIP) {
			if svr.logger != nil {
				svr.logger.Warn("Connection denied by DDoS protection: %s", clientIP)
			}
			return
		}

		// 检查是否超过最大客户端数
		if svr.config.MaxClientCount > 0 && int(svr.clientSessionMap.Len()) >= svr.config.MaxClientCount {
			if svr.logger != nil {
				svr.logger.Warn("Maximum connections exceeded, max:%d", svr.config.MaxClientCount)
			}
			return
		}

		// 创建新会话
		session = svr.createSession(addr)
	}

	// 处理数据包
	session.handlePacket(data)
}

func (svr *UdpServer) getSessionByAddr(addr *net.UDPAddr) (*UdpServerSession, bool) {
	// 遍历所有会话，查找匹配的地址
	var targetSession *UdpServerSession
	found := false

	svr.clientSessionMap.Range(func(sid SessionIdType, value *UdpServerSession) bool {
		if value.addr.String() == addr.String() {
			targetSession = value
			found = true
			return false
		}
		return true
	})

	return targetSession, found
}

func (svr *UdpServer) createSession(addr *net.UDPAddr) *UdpServerSession {
	// 初始化ECDH密钥交换
	dhExchange, err := NewDHKeyExchange()
	if err != nil {
		if svr.logger != nil {
			svr.logger.Error("Failed to initialize DH key exchange for UDP session: %v", err)
		}
		dhExchange = nil
	} else {
		if svr.logger != nil {
			svr.logger.Info("Initialized ECDH key exchange for new UDP session")
		}
	}

	sid := atomic.AddUint64(&svr.clientSIDAtomic, 1)
	newSession := NewUdpServerSession(svr, addr, sid, svr.RemoveSession, nil, dhExchange)
	svr.clientSessionMap.Store(sid, newSession)

	if svr.onAddSession != nil {
		svr.onAddSession(newSession.sid)
	}

	newSession.Start()
	return newSession
}

func (svr *UdpServer) Close() {
	if svr.logger != nil {
		svr.logger.Info("Close udp server, session count: %d", svr.clientSessionMap.Len())
	}

	if err := svr.listener.Close(); err != nil {
		if svr.logger != nil {
			svr.logger.Error("Failed to close listener: %v", err)
		}
	}

	svr.clientSessionMap.Range(func(sid SessionIdType, value *UdpServerSession) bool {
		session := value
		session.Close()
		svr.clientSessionMap.Delete(session.sid)
		return true
	})

	svr.wg.Wait()

	if svr.logger != nil {
		svr.logger.Info("Udp server closed")
	}
}

func (svr *UdpServer) RemoveSession(cli *UdpServerSession) {
	if svr.onRemoveSession != nil {
		svr.onRemoveSession(cli.sid)
	}
	svr.clientSessionMap.Delete(cli.sid)
}

func (svr *UdpServer) GetSession(sid SessionIdType) *UdpServerSession {
	if client, ok := svr.clientSessionMap.Load(sid); ok {
		return client
	}
	return nil
}

func (svr *UdpServer) GetAllSession() []*UdpServerSession {
	var sessionList []*UdpServerSession
	svr.clientSessionMap.Range(func(sid SessionIdType, value *UdpServerSession) bool {
		sessionList = append(sessionList, value)
		return true
	})

	return sessionList
}

func (svr *UdpServer) RegisterDispatcher(fun HandlerFun) {
	svr.dispatcher = fun
}
