package zNet

import (
	"context"
	"errors"
	"fmt"
	"net"
	"strconv"
	"sync"
	"sync/atomic"
	"time"
)

// ErrClientNotConnected 客户端未连接时发送返回的错误（OPT-11）。
// 此前返回 net.ErrWriteToConnected（语义相反：那是"写到已连接的连接"）。
var ErrClientNotConnected = errors.New("tcp client not connected")

// ClientState 客户端连接状态
type ClientState int

const (
	ClientStateDisconnected ClientState = iota // 未连接
	ClientStateConnecting                      // 连接中
	ClientStateConnected                       // 已连接
	ClientStateReconnecting                    // 重连中
)

// ClientStateCallback 客户端状态回调函数类型
type ClientStateCallback func(state ClientState)

// TcpClient TCP客户端
// 用于连接到TCP服务器并进行通信，支持自动重连、状态回调等功能
type TcpClient struct {
	config     *TcpClientConfig                 // 客户端配置
	session    atomic.Pointer[TcpClientSession] // 会话实例（原子读写：Connect 写，Send/GetSession/monitor 读，消除重连期数据竞争）
	dispatcher HandlerFun                       // 消息分发器
	logger     Logger                           // 日志记录器
	state      atomic.Value                     // 连接状态（原子操作）
	ctx        context.Context                  // 上下文

	ctxCancel context.CancelFunc // 上下文取消函数
	wg        sync.WaitGroup     // 等待组

	stateCallbacks []ClientStateCallback // 状态回调函数列表
	reconnectCount int                   // 当前重连次数
	monitorOnce    sync.Once             // 保证连接监控 goroutine 只启动一次（重连不再重复 spawn）
	packetCodec    PacketCodec
	packetCodecErr error
}

// NewTcpClient 创建新的TCP客户端实例
// 参数:
//   - cfg: TCP客户端配置
//   - opts: 可选配置项（函数式选项模式）
//
// 返回:
//   - *TcpClient: TCP客户端实例
func NewTcpClient(cfg *TcpClientConfig, opts ...ClientOption) *TcpClient {
	if cfg.ChanSize <= 0 {
		cfg.ChanSize = DefaultChanSize
	}
	if cfg.HeartbeatDuration <= 0 {
		cfg.HeartbeatDuration = 30
	}
	normalizePacketSizeLimits(&cfg.MaxWirePacketSize, &cfg.MaxPacketDataSize, &cfg.MaxDecodedPacketSize)
	if cfg.ReconnectDelay <= 0 {
		cfg.ReconnectDelay = 5
	}

	cli := &TcpClient{
		config:         cfg,
		stateCallbacks: make([]ClientStateCallback, 0),
	}
	cli.state.Store(ClientStateDisconnected)

	ctx, ctxCancel := context.WithCancel(context.Background())
	cli.ctx = ctx
	cli.ctxCancel = ctxCancel

	for _, opt := range opts {
		opt(cli)
	}
	cli.packetCodec, cli.packetCodecErr = newEndpointPacketCodec(cfg.ByteOrder)

	return cli
}

// Connect 连接到服务器
// 建立TCP连接，执行DH密钥交换，初始化会话
//
// 返回:
//   - error: 连接失败时返回错误
func (cli *TcpClient) Connect() error {
	return cli.connect(context.Background())
}

// ConnectContext establishes the initial TCP session within the caller's
// deadline. Context cancellation covers both dialing and the optional key
// exchange; a canceled handshake never publishes a partial session.
func (cli *TcpClient) ConnectContext(ctx context.Context) error {
	if ctx == nil {
		return errors.New("tcp client connect: nil context")
	}
	return cli.connect(ctx)
}

func (cli *TcpClient) connect(ctx context.Context) error {
	if cli.packetCodecErr != nil {
		return cli.packetCodecErr
	}
	if err := ctx.Err(); err != nil {
		return err
	}
	cli.setState(ClientStateConnecting)

	endpoint := net.JoinHostPort(cli.config.ServerAddr, strconv.Itoa(cli.config.ServerPort))
	rawConn, err := (&net.Dialer{}).DialContext(ctx, "tcp", endpoint)
	if err != nil {
		cli.setState(ClientStateDisconnected)
		if ctxErr := ctx.Err(); ctxErr != nil {
			return ctxErr
		}
		return err
	}
	conn, ok := rawConn.(*net.TCPConn)
	if !ok {
		_ = rawConn.Close()
		cli.setState(ClientStateDisconnected)
		return fmt.Errorf("tcp client connect: unexpected connection type %T", rawConn)
	}

	s := &TcpClientSession{}

	var aesKey []byte

	// 检查是否禁用加密
	if !cli.config.DisableEncryption {
		// Context cancellation closes conn to interrupt both reads and writes.
		// The existing handshake timeout remains the upper bound when the caller
		// supplies a longer deadline or no deadline.
		contextCloseDone := make(chan struct{})
		stopContextClose := context.AfterFunc(ctx, func() {
			_ = conn.Close()
			close(contextCloseDone)
		})
		handshakeTimeout := DefaultKeyExchangeTimeout
		if deadline, ok := ctx.Deadline(); ok {
			remaining := time.Until(deadline)
			if remaining < handshakeTimeout {
				handshakeTimeout = remaining
			}
		}
		aesKey, err = PerformKeyExchangeWithDeadline(conn, handshakeTimeout)
		if !stopContextClose() {
			<-contextCloseDone
		}
		if err != nil {
			_ = conn.Close()
			cli.setState(ClientStateDisconnected)
			if ctxErr := ctx.Err(); ctxErr != nil {
				return ctxErr
			}
			return err
		}

		if cli.logger != nil {
			cli.logger.Info("DH key exchange completed successfully, AES key length: %d", len(aesKey))
		}
	} else {
		if cli.logger != nil {
			cli.logger.Info("Encryption disabled, skipping key exchange")
		}
	}
	if err := ctx.Err(); err != nil {
		_ = conn.Close()
		cli.setState(ClientStateDisconnected)
		return err
	}

	s.Init(cli, conn, aesKey)
	s.Start()
	// NET-8: 原子换入新会话并关闭旧会话——重复调用 Connect（如手动重连）若只 Store 不 Close
	// 旧会话，旧会话的 goroutine 与底层 conn 会泄漏。
	if old := cli.session.Swap(s); old != nil {
		old.Close()
	}
	cli.setState(ClientStateConnected)
	cli.reconnectCount = 0

	// 仅在首次连接时启动唯一的连接监控 goroutine；重连也走 Connect()，
	// 若每次都 spawn 会造成 monitor goroutine 泄漏 + 多监控并发重复重连。
	if cli.config.AutoReconnect {
		cli.monitorOnce.Do(func() {
			cli.wg.Add(1)
			go cli.monitorConnection()
		})
	}

	return nil
}

// monitorConnection 监控连接状态，支持自动重连
func (cli *TcpClient) monitorConnection() {
	defer cli.wg.Done()

	for {
		select {
		case <-cli.ctx.Done():
			return
		case <-time.After(time.Second):
			state := cli.GetState()

			if s := cli.session.Load(); state == ClientStateConnected && s != nil {
				if s.IsClosed() {
					cli.handleDisconnect()
				}
			}
		}
	}
}

// handleDisconnect 处理连接断开：带退避的重连循环，直到重连成功、达到上限或客户端关闭。
// 由唯一的 monitorConnection 同步调用（重连期间暂停健康检查，避免并发重连）。
func (cli *TcpClient) handleDisconnect() {
	cli.setState(ClientStateDisconnected)

	if !cli.config.AutoReconnect {
		return
	}

	delay := time.Duration(cli.config.ReconnectDelay) * time.Second
	for {
		select {
		case <-cli.ctx.Done():
			return
		default:
		}

		if cli.config.MaxReconnectTimes > 0 && cli.reconnectCount >= cli.config.MaxReconnectTimes {
			if cli.logger != nil {
				cli.logger.Warn("Max reconnect times reached, stop reconnecting")
			}
			return
		}

		cli.setState(ClientStateReconnecting)
		if cli.logger != nil {
			cli.logger.Info("Connection lost, will reconnect in %d seconds", cli.config.ReconnectDelay)
		}
		time.Sleep(delay)

		cli.reconnectCount++
		// Connect 成功会把 reconnectCount 归零（下次断线重新计数）并置 Connected 状态。
		if err := cli.Connect(); err != nil {
			if cli.logger != nil {
				cli.logger.Error("Reconnect failed: %v", err)
			}
			continue // 重试直至成功或超过上限
		}
		if cli.logger != nil {
			cli.logger.Info("Reconnected successfully")
		}
		return
	}
}

// Send 发送数据
// 参数:
//   - protoId: 协议ID
//   - data: 要发送的数据
//
// 返回:
//   - error: 发送失败时返回错误
func (cli *TcpClient) Send(protoId ProtoIdType, data []byte) error {
	s := cli.session.Load()
	if s == nil {
		return ErrClientNotConnected // OPT-11: 语义正确的"未连接"错误
	}
	return s.Send(protoId, data)
}

// SendContext 使用当前重连 session 执行有 deadline 的可靠 admission/写入。
func (cli *TcpClient) SendContext(ctx context.Context, protoId ProtoIdType, data []byte) error {
	return cli.SendWithOptions(ctx, protoId, data, SendOptions{Class: ReliableCommand})
}

// SendWithOptions 每次发送都读取当前 session，避免自动重连后写旧连接。
func (cli *TcpClient) SendWithOptions(ctx context.Context, protoId ProtoIdType, data []byte, options SendOptions) error {
	s := cli.session.Load()
	if s == nil {
		return ErrClientNotConnected
	}
	return s.SendWithOptions(ctx, protoId, data, options)
}

// Close 关闭客户端
// 关闭会话并断开连接
func (cli *TcpClient) Close() {
	cli.ctxCancel()
	cli.wg.Wait()
	if s := cli.session.Load(); s != nil {
		s.Close()
	}
	cli.setState(ClientStateDisconnected)
}

// RegisterDispatcher 注册消息分发器
// 参数:
//   - fun: 消息处理函数
func (cli *TcpClient) RegisterDispatcher(fun HandlerFun) {
	cli.dispatcher = fun
}

// RegisterStateCallback 注册状态回调函数
// 参数:
//   - cb: 状态回调函数
func (cli *TcpClient) RegisterStateCallback(cb ClientStateCallback) {
	cli.stateCallbacks = append(cli.stateCallbacks, cb)
}

// setState 设置连接状态并触发回调
func (cli *TcpClient) setState(state ClientState) {
	cli.state.Store(state)

	for _, cb := range cli.stateCallbacks {
		cb(state)
	}
}

// GetState 获取连接状态
//
// 返回:
//   - ClientState: 当前连接状态
func (cli *TcpClient) GetState() ClientState {
	return cli.state.Load().(ClientState)
}

// IsConnected 检查是否已连接
//
// 返回:
//   - bool: 是否已连接
func (cli *TcpClient) IsConnected() bool {
	return cli.GetState() == ClientStateConnected
}

// GetSession 获取会话实例
//
// 返回:
//   - *TcpClientSession: 会话实例，未连接时返回nil
func (cli *TcpClient) GetSession() *TcpClientSession {
	return cli.session.Load()
}

// GetMaxPacketDataSize 获取最大数据包大小
//
// 返回:
//   - int32: 最大数据包大小
func (cli *TcpClient) GetMaxPacketDataSize() int32 {
	return cli.config.MaxWirePacketSize
}

// GetMaxWirePacketSize 获取线包 payload 上限。
func (cli *TcpClient) GetMaxWirePacketSize() int32 {
	return cli.config.MaxWirePacketSize
}

// GetMaxDecodedPacketSize 获取解密和解压后的 payload 上限。
func (cli *TcpClient) GetMaxDecodedPacketSize() int32 {
	return cli.config.MaxDecodedPacketSize
}

// ConnectToServer 连接到服务器（兼容旧API）
// 参数:
//   - serverAddr: 服务器地址
//   - serverPort: 服务器端口
//   - rsaPublicFile: RSA公钥文件（保留参数，当前使用DH密钥交换）
//   - heartbeatDuration: 心跳间隔（秒）
//   - maxPacketDataSize: 最大数据包大小
//
// 返回:
//   - error: 连接失败时返回错误
func (cli *TcpClient) ConnectToServer(serverAddr string, serverPort int, rsaPublicFile string, heartbeatDuration int, maxPacketDataSize int32) error {
	cli.config.ServerAddr = serverAddr
	cli.config.ServerPort = serverPort
	cli.config.HeartbeatDuration = heartbeatDuration
	cli.config.MaxPacketDataSize = maxPacketDataSize
	cli.config.MaxWirePacketSize = maxPacketDataSize
	normalizePacketSizeLimits(&cli.config.MaxWirePacketSize, &cli.config.MaxPacketDataSize, &cli.config.MaxDecodedPacketSize)
	return cli.Connect()
}
