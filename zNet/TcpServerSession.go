package zNet

import (
	"context"
	"errors"
	"fmt"
	"io"
	"net"
	"sync"
	"sync/atomic"
	"time"

	"github.com/golang/snappy"
	"github.com/pzqf/zUtil/zCrypto"
)

// TcpServerSession TCP服务器会话
// 管理与单个客户端的TCP连接，负责数据收发、心跳检测、加密解密
type TcpServerSession struct {
	conn              *net.TCPConn
	sid               SessionIdType
	sendQueue         *outboundQueue
	receiveChan       chan *NetPacket
	wg                sync.WaitGroup
	lastHeartBeat     time.Time
	ctxCancel         context.CancelFunc
	onClose           TcpCloseCallBackFunc
	closeOnce         sync.Once
	aesKey            []byte
	currentKey        atomic.Value
	currentKeyID      atomic.Uint32
	svr               *TcpServer    // 所属服务器
	obj               interface{}   // 附加对象
	sendSequence      atomic.Uint64 // 发送序列号
	packetCodec       PacketCodec
	invalidPacketLogs packetErrorLogLimiter
	protocol          *ProtocolVersionNegotiator
}

// TcpCloseCallBackFunc TCP连接关闭回调函数类型
// 参数:
//   - c: 被关闭的会话实例
type TcpCloseCallBackFunc func(c *TcpServerSession)

func (s *TcpServerSession) triggerOnClose() {
	s.closeOnce.Do(func() {
		if s.onClose != nil {
			s.onClose(s)
		}
	})
}

// NewTcpServerSession 创建TCP服务器会话
// 参数:
//   - svr: 所属的TCP服务器
//   - conn: TCP连接
//   - sid: 会话ID
//   - closeCallBack: 关闭回调函数
//   - aesKey: AES加密密钥
//
// 返回:
//   - *TcpServerSession: 会话实例
func NewTcpServerSession(svr *TcpServer, conn *net.TCPConn, sid SessionIdType, closeCallBack TcpCloseCallBackFunc, aesKey []byte) *TcpServerSession {
	protocol, _ := NewProtocolVersionNegotiator(svr.protocolPolicy)
	newSession := TcpServerSession{
		conn:          conn,
		sid:           sid,
		sendQueue:     newOutboundQueue(svr.config.ChanSize, backpressureMetrics(svr.metrics)),
		receiveChan:   make(chan *NetPacket, svr.config.ChanSize),
		lastHeartBeat: time.Now(),
		onClose:       closeCallBack,
		aesKey:        aesKey,
		svr:           svr,
		packetCodec:   svr.packetCodec,
		protocol:      protocol,
	}
	// 初始化当前密钥
	newSession.currentKey.Store(aesKey)
	newSession.currentKeyID.Store(0) // 0表示使用初始密钥
	return &newSession
}

// UpdateKey 更新密钥（密钥轮换时调用）
// 参数:
//   - newKey: 新密钥
//   - newKeyID: 新密钥ID
func (s *TcpServerSession) UpdateKey(newKey []byte, newKeyID uint32) {
	s.currentKey.Store(newKey)
	s.currentKeyID.Store(newKeyID)

	// 发送密钥轮换通知给客户端
	notify := &KeyRotationNotify{
		KeyID:     newKeyID,
		Timestamp: time.Now().Unix(),
		Nonce:     0,
	}
	data := notify.MarshalWithByteOrder(s.packetCodec.ByteOrder())
	_ = s.Send(KeyRotationNotifyProtoId, data)
}

// GetDecryptKey 获取解密密钥（根据密钥ID）
// 参数:
//   - keyID: 密钥ID
//
// 返回:
//   - []byte: 密钥
func (s *TcpServerSession) GetDecryptKey(keyID uint32) []byte {
	if keyID == 0 {
		return s.aesKey
	}

	if keyID == s.currentKeyID.Load() {
		return s.currentKey.Load().([]byte)
	}

	// 从服务器密钥管理器获取历史密钥
	if s.svr.keyRotationMgr != nil {
		if key, exists := s.svr.keyRotationMgr.GetKeyByID(keyID); exists {
			return key
		}
	}

	return nil
}

// Start 启动会话
// 启动接收、处理和心跳检测goroutine
func (s *TcpServerSession) Start() {
	if s.conn == nil {
		return
	}
	ctx, ctxCancel := context.WithCancel(context.Background())
	s.ctxCancel = ctxCancel

	// wg.Add 必须在启动 goroutine 之前完成，否则 Close 的 wg.Wait 可能在 goroutine 尚未
	// Add 时以计数 0 返回，既不等待其退出又与其内部 Add 竞争（-race 实测报此竞争）。
	s.wg.Add(1)
	go s.receive(ctx) // 启动接收协程

	// process() 是 sendQueue 的唯一消费者，必须无条件启动。worker-pool 模式下 receiveChan 不被喂，
	// process() 的 receiveChan 分支不触发、仅服务发送侧。
	s.wg.Add(1)
	go s.process(ctx)

	if s.svr.config.HeartbeatDuration > 0 {
		s.wg.Add(1)
		go s.heartbeatCheck(ctx) // 启动心跳检测协程
	}
}

// Close 关闭会话
// 取消上下文并等待所有goroutine退出
func (s *TcpServerSession) Close() {
	s.sendQueue.Close(ErrSessionClosed)
	if s.ctxCancel != nil {
		s.ctxCancel()
	}
	// 关闭底层 TCP 连接：让对端（客户端）检测到断开，并解除本端阻塞在 ReadFull 的
	// receive goroutine，避免下面的 wg.Wait 挂起（此前从不关闭 conn）。
	if s.conn != nil {
		_ = s.conn.Close()
	}
	s.wg.Wait()
}

// receive 接收数据
// 从TCP连接中读取数据包，解析并放入接收通道
//
// 参数:
//   - ctx: 上下文
func (s *TcpServerSession) receive(ctx context.Context) {
	// wg.Add 已由 Start 在启动本 goroutine 前完成。
	defer s.ctxCancel()
	defer s.wg.Done()
	defer func() {
		s.triggerOnClose()
		if err := recover(); err != nil {
			if s.svr.logger != nil {
				s.svr.logger.Error("process panic:%v, sid:%d, closed", err, s.sid)
			}
		}
	}()

	headBuf := make([]byte, NetPacketHeadSize)
	clientIP := s.conn.RemoteAddr().(*net.TCPAddr).IP.String()
	// ddosSubject: 包/流量限流的主体 = **本条连接**（IP+会话号）。
	// 不用裸 IP：NAT/CGNAT 下同 IP 后面是大量互不相干的玩家，按 IP 聚合会让一个人的突发
	// 把整段 IP 的人一起限掉（原实现还会顺手拉黑 IP 24 小时）。连接洪水另有 AllowConnection 按 IP 管。
	ddosSubject := fmt.Sprintf("%s#%d", clientIP, s.sid)
	for {
		if ctx.Err() != nil {
			break
		}

		// 读取数据包头
		n, err := io.ReadFull(s.conn, headBuf)
		if err != nil {
			if s.svr.logger != nil {
				s.svr.logger.Error("Client conn read error, error:%v, sid:%d, closed", err, s.sid)
			}
			break
		}

		if n != NetPacketHeadSize {
			if s.svr.logger != nil {
				s.svr.logger.Error("Client conn read error, head size %d, sid:%d, closed", n, s.sid)
			}
			break
		}

		// 检查流量是否超过限制（DDoS防护）。按**连接**限流：超限只断这一条连接，
		// 不牵连同 IP 的其他玩家（NAT/CGNAT 下同 IP 后面可能是成千上万无关玩家）。
		if !s.svr.ddosProtection.AllowTrafficFrom(ddosSubject, clientIP, int64(n)) {
			if s.svr.logger != nil {
				s.svr.logger.Warn("Traffic limit exceeded, ip: %s, sid: %d", clientIP, s.sid)
			}
			break
		}

		// 解析并校验数据包头。失败即关闭 TCP：流式连接无法可靠跳过一个不可信长度的 body。
		netPacket, decodeErr := s.packetCodec.DecodeHeader(headBuf, s.svr.config.MaxWirePacketSize)
		if decodeErr != nil {
			recordPacketDecodeError(s.svr.metrics, decodeErr)
			if s.svr.logger != nil && s.invalidPacketLogs.Allow(time.Now()) {
				s.svr.logger.Error("Receive NetPacket, invalid head: %v, len: %d", decodeErr, len(headBuf))
			}
			break
		}
		if err := s.protocol.ValidateInboundPacketVersion(netPacket.ProtoId, netPacket.Version); err != nil {
			if s.svr.logger != nil && s.invalidPacketLogs.Allow(time.Now()) {
				s.svr.logger.Error("Receive NetPacket, incompatible version: %v, sid:%d", err, s.sid)
			}
			break
		}

		// 读取数据包体
		if netPacket.DataSize > 0 {
			netPacket.Data = make([]byte, int(netPacket.DataSize))
			n, err = io.ReadFull(s.conn, netPacket.Data)
			if err != nil {
				if s.svr.logger != nil {
					s.svr.logger.Error("Client conn read data error:%v, sid:%d, closed", err, s.sid)
				}
				break
			}

			if netPacket.DataSize != int32(n) {
				if s.svr.logger != nil {
					s.svr.logger.Error("Receive NetPacket, Data size error,protoid:%d, DataSize:%d, received:%d",
						netPacket.ProtoId, netPacket.DataSize, n)
				}
				break
			}

			// 检查流量是否超过限制（按连接，同上）
			if !s.svr.ddosProtection.AllowTrafficFrom(ddosSubject, clientIP, int64(n)) {
				if s.svr.logger != nil {
					s.svr.logger.Warn("Traffic limit exceeded, ip: %s, sid: %d", clientIP, s.sid)
				}
				break
			}
		}

		// 检查数据包频率是否超过限制（按连接，同上）
		if !s.svr.ddosProtection.AllowPacketFrom(ddosSubject, clientIP) {
			if s.svr.logger != nil {
				s.svr.logger.Warn("Packet rate limit exceeded, ip: %s, sid: %d", clientIP, s.sid)
			}
			break
		}

		// 指标上报：整包（头+体）成功读入，计入接收字节与包数（Phase 3.5）。
		if s.svr.metrics != nil {
			s.svr.metrics.RecordBytesReceived(NetPacketHeadSize + int(netPacket.DataSize))
			s.svr.metrics.RecordPacketsReceived(1)
		}

		// Negotiation stays in the ordered read path even when application packets
		// use a worker pool. This guarantees the following packet observes the
		// compatibility established by the preceding control frame.
		if s.svr.protocolPolicy.Enabled && netPacket.ProtoId == ProtocolNegotiationProtoId {
			if err := s.handleProtocolNegotiationPacket(&netPacket); err != nil {
				if s.svr.logger != nil && s.invalidPacketLogs.Allow(time.Now()) {
					s.svr.logger.Error("Protocol negotiation failed: %v, sid:%d", err, s.sid)
				}
				break
			}
			s.heartbeatUpdate()
			continue
		}

		// 非心跳包处理
		if netPacket.ProtoId != HeartbeatProtoId {
			if s.svr.config.UseWorkerPool {
				// 工作池模式：队列饱和时停止读本连接形成 TCP 背压；关闭可经 ctx 取消等待。
				packet := netPacket // 复制数据包
				started := time.Now()
				err := s.svr.workerPool.SubmitWithContext(ctx, func() error {
					s.processPacket(&packet)
					return nil
				})
				if metrics := backpressureMetrics(s.svr.metrics); metrics != nil {
					metrics.RecordWorkerQueueWait(time.Since(started))
					if err != nil {
						metrics.IncWorkerQueueRejected()
					}
				}
				if err != nil {
					if ctx.Err() == nil && s.svr.logger != nil {
						s.svr.logger.Warn("Worker queue rejected packet: %v, sid:%d", err, s.sid)
					}
					break
				}
			} else {
				// 传统模式：放入接收通道
				s.receiveChan <- &netPacket
			}
		}

		s.heartbeatUpdate() // 更新心跳时间
	}
	s.ctxCancel()
}

// processPacket 处理数据包
// 解密数据并分发到消息处理器
//
// 参数:
//   - packet: 要处理的数据包
func (s *TcpServerSession) processPacket(packet *NetPacket) {
	// 检查序列号和时间戳（如果启用）
	if s.svr.config.EnableSequenceCheck && s.svr.sequenceManager != nil {
		if !s.svr.sequenceManager.ValidateSequence(s.sid, packet.Sequence, packet.Timestamp) {
			if s.svr.logger != nil {
				s.svr.logger.Warn("Invalid sequence or timestamp: seq=%d, ts=%d, sid:%d", packet.Sequence, packet.Timestamp, s.sid)
			}
			return
		}
	}

	// 处理密钥轮换通知
	if packet.ProtoId == KeyRotationNotifyProtoId {
		// 客户端不应该发送密钥轮换通知，这里可以记录日志或忽略
		if s.svr.logger != nil {
			s.svr.logger.Warn("Received unexpected key rotation notify from client, sid:%d", s.sid)
		}
		return
	}

	encrypted := len(packet.Data) > 0 && !s.svr.config.DisableEncryption
	var decryptKey []byte
	if encrypted {
		decryptKey = s.GetDecryptKey(packet.KeyID)
	}
	if err := decodePacketPayload(packet, decryptKey, encrypted, s.svr.config.MaxDecodedPacketSize); err != nil {
		recordPacketDecodeError(s.svr.metrics, err)
		if s.svr.logger != nil && s.invalidPacketLogs.Allow(time.Now()) {
			s.svr.logger.Error("Decode packet payload error: %v, sid:%d, keyID:%d", err, s.sid, packet.KeyID)
		}
		return
	}

	// 分发到消息处理器。就地 recover 隔离单条消息的 panic——避免一条坏包连累整个会话
	// （传统模式下会被 process() 的终端 recover 关闭会话）或 worker goroutine 乃至进程崩溃
	// （工作池模式）。单包 panic 只丢该包，会话/worker 继续。
	if s.svr.dispatcher != nil {
		func() {
			defer func() {
				if r := recover(); r != nil && s.svr.logger != nil {
					s.svr.logger.Error("Dispatcher panic recovered, ProtoId:%d, sid:%d, panic:%v", packet.ProtoId, s.sid, r)
				}
			}()
			if err := s.svr.dispatcher(s, packet); err != nil && s.svr.logger != nil {
				s.svr.logger.Error("Dispatcher error: %v, ProtoId: %d", err, packet.ProtoId)
			}
		}()
	}
}

func (s *TcpServerSession) handleProtocolNegotiationPacket(packet *NetPacket) error {
	encrypted := len(packet.Data) > 0 && !s.svr.config.DisableEncryption
	var decryptKey []byte
	if encrypted {
		decryptKey = s.GetDecryptKey(packet.KeyID)
	}
	if err := decodePacketPayload(packet, decryptKey, encrypted, s.svr.config.MaxDecodedPacketSize); err != nil {
		recordPacketDecodeError(s.svr.metrics, err)
		return err
	}
	_, err := s.protocol.AcceptProtocolNegotiation(packet.Version, packet.Data)
	return err
}

// process 处理数据
// 从接收通道读取数据包，解密后分发到消息处理器；从发送通道读取数据包并发送
//
// 参数:
//   - ctx: 上下文
func (s *TcpServerSession) process(ctx context.Context) {
	// wg.Add 已由 Start 在启动本 goroutine 前完成。
	defer s.wg.Done()
	defer func() {
		s.triggerOnClose()
		if err := recover(); err != nil {
			if s.svr.logger != nil {
				s.svr.logger.Error("process panic:%v, sid:%d, closed", err, s.sid)
			}
		}
	}()
	running := true
	for {
		select {
		case receivePacket := <-s.receiveChan:
			// 使用processPacket处理数据包
			s.processPacket(receivePacket)

		case <-s.sendQueue.Ready():
			// 发送数据包
			sendPacket, ok := s.sendQueue.TryDequeue()
			if !ok {
				continue
			}
			_, err := s.sendOutbound(sendPacket)
			if err != nil {
				if s.svr.logger != nil {
					s.svr.logger.Error("Send NetPacket error:%v, ProtoId:%d", err, sendPacket.packet.ProtoId)
				}
			}
		case <-ctx.Done():
			// 上下文取消，处理剩余消息
			// 处理接收通道中剩余的消息
			for {
				if len(s.receiveChan) > 0 {
					receivePacket := <-s.receiveChan
					s.processPacket(receivePacket)
					continue
				}
				break
			}
			s.sendQueue.Close(ErrSessionClosed)
			// 处理发送队列中剩余的消息
			for {
				if sendPacket, ok := s.sendQueue.TryDequeue(); ok {
					_, err := s.sendOutbound(sendPacket)
					if err != nil {
						if s.svr.logger != nil {
							s.svr.logger.Error("Send NetPacket error:%v, ProtoId:%d", err, sendPacket.packet.ProtoId)
						}
						break
					}
					continue
				}
				break
			}

			running = false
		}
		if !running {
			break
		}
	}

	// 关闭TCP连接
	if err := s.conn.Close(); err != nil {
		if s.svr.logger != nil {
			s.svr.logger.Error("Failed to close connection: %v, sid: %d", err, s.sid)
		}
	}
	s.triggerOnClose()
}

// Send 发送数据
// 将数据加密后放入发送通道
//
// 参数:
//   - protoId: 协议ID
//   - data: 要发送的数据
//
// 返回:
//   - error: 发送失败时返回错误
func (s *TcpServerSession) Send(protoId ProtoIdType, data []byte) error {
	ctx, cancel := context.WithTimeout(context.Background(), DefaultSendTimeout)
	defer cancel()
	return s.SendWithOptions(ctx, protoId, data, SendOptions{Class: ReliableCommand})
}

// SendContext 以 ReliableCommand admission 发送；成功只表示已进入发送队列。
func (s *TcpServerSession) SendContext(ctx context.Context, protoId ProtoIdType, data []byte) error {
	return s.SendWithOptions(ctx, protoId, data, SendOptions{Class: ReliableCommand})
}

// SendWithOptions 按投递等级执行有界 admission。
func (s *TcpServerSession) SendWithOptions(ctx context.Context, protoId ProtoIdType, data []byte, options SendOptions) error {
	ctx, cancel := normalizeSendContext(ctx, DefaultSendTimeout)
	defer cancel()
	if options.Class == LatestFrame && options.CoalesceKey == 0 {
		options.CoalesceKey = uint64(uint32(protoId))
	}
	netPacket, err := s.buildOutboundPacket(protoId, data)
	if err != nil {
		return err
	}
	return s.sendQueue.Enqueue(ctx, &outboundPacket{packet: netPacket, deadline: outboundDeadline(ctx)}, options)
}

func (s *TcpServerSession) buildOutboundPacket(protoId ProtoIdType, data []byte) (*NetPacket, error) {
	version, err := s.protocol.OutboundPacketVersion(protoId)
	if err != nil {
		return nil, err
	}
	netPacket := NetPacket{
		ProtoId:   protoId,
		Version:   version,
		Sequence:  s.sendSequence.Add(1),
		Timestamp: time.Now().Unix(),
	}

	// 获取加密密钥（如果启用密钥轮换）
	var key []byte
	var keyID uint32
	if s.svr.config.EnableKeyRotation {
		key = s.currentKey.Load().([]byte)
		keyID = s.currentKeyID.Load()
	} else if !s.svr.config.DisableEncryption {
		key = s.aesKey
	}
	netPacket.KeyID = keyID

	// 先压缩后加密（compress→encrypt）：压缩作用于明文（可压缩），并与接收端
	// decrypt→decompress 顺序互逆。旧实现 encrypt→compress 与接收端不互逆，
	// 仅因密文高熵压不小、IsCompressed 通常不置位而侥幸未暴露。
	payload := data

	// 压缩数据（基于明文大小判断阈值）
	if !s.svr.config.DisableCompression && s.svr.compressionConfig.Enabled &&
		len(payload) > s.svr.compressionConfig.CompressionThreshold &&
		len(payload) <= s.svr.compressionConfig.MaxCompressSize {
		compressed := snappy.Encode(nil, payload)
		if len(compressed) < len(payload) {
			payload = compressed
			netPacket.IsCompressed = CompressionSnappy
		}
	}

	// 加密（压缩后的）数据
	if key != nil && !s.svr.config.DisableEncryption {
		if encrypted, err := zCrypto.AESEncrypt(payload, key, nil, zCrypto.AESModeGCM); err == nil {
			netPacket.Data = encrypted
		} else {
			if s.svr.logger != nil {
				s.svr.logger.Error("AES-GCM encrypt error: %v, ProtoId:%d", err, protoId)
			}
			return nil, err
		}
	} else {
		netPacket.Data = append([]byte(nil), payload...)
	}

	netPacket.DataSize = int32(len(netPacket.Data))
	if err := ValidatePacketHeader(&netPacket, s.svr.config.MaxWirePacketSize); err != nil {
		if s.svr.logger != nil {
			s.svr.logger.Error("Send packet illegal: %v", err)
		}
		return nil, err
	}
	return &netPacket, nil
}

func (s *TcpServerSession) enqueueProtocolNegotiation() error {
	if !s.svr.protocolPolicy.Enabled {
		return nil
	}
	frame, err := MarshalProtocolNegotiation(s.svr.protocolPolicy)
	if err != nil {
		return err
	}
	packet, err := s.buildOutboundPacket(ProtocolNegotiationProtoId, frame)
	if err != nil {
		return err
	}
	return s.sendQueue.Enqueue(
		context.Background(),
		&outboundPacket{packet: packet},
		SendOptions{Class: BestEffortEvent},
	)
}

func (s *TcpServerSession) sendOutbound(outbound *outboundPacket) (int, error) {
	if outbound == nil || outbound.packet == nil {
		return 0, errors.New("nil outbound packet")
	}
	if !outbound.deadline.IsZero() {
		if err := s.conn.SetWriteDeadline(outbound.deadline); err != nil {
			return 0, err
		}
		defer s.conn.SetWriteDeadline(time.Time{})
	}
	return s.send(outbound.packet)
}

// send 发送数据包到TCP连接
// 参数:
//   - netPacket: 要发送的数据包
//
// 返回:
//   - int: 发送的字节数
//   - error: 发送失败时返回错误
func (s *TcpServerSession) send(netPacket *NetPacket) (int, error) {
	n, err := s.conn.Write(s.packetCodec.Marshal(netPacket))
	if err != nil {
		if s.svr.logger != nil {
			s.svr.logger.Error("Failed to write to connection: %v, ProtoId: %d", err, netPacket.ProtoId)
		}
		return 0, err
	}
	if s.svr.metrics != nil {
		s.svr.metrics.RecordBytesSent(n)
		s.svr.metrics.RecordPacketsSent(1)
	}
	return n, nil
}

// heartbeatUpdate 更新心跳时间
func (s *TcpServerSession) heartbeatUpdate() {
	s.lastHeartBeat = time.Now()
}

// heartbeatCheck 心跳检测
// 定期检查客户端是否超时，超时则关闭连接
//
// 参数:
//   - ctx: 上下文
func (s *TcpServerSession) heartbeatCheck(ctx context.Context) {
	// wg.Add 已由 Start 在启动本 goroutine 前完成。
	defer s.wg.Done()
	duration := time.Second * time.Duration(s.svr.config.HeartbeatDuration)
	breakDuration := time.Second * time.Duration(s.svr.config.HeartbeatDuration*2)
	for {
		select {
		case <-time.After(duration):
			if time.Since(s.lastHeartBeat) > breakDuration {
				s.ctxCancel()
				return
			}
		case <-ctx.Done():
			return
		}
	}
}

// GetSid 获取会话ID
// 返回:
//   - SessionIdType: 会话ID
func (s *TcpServerSession) GetSid() SessionIdType {
	return s.sid
}

// ProtocolCompatibility returns the immutable negotiated version/capability snapshot.
func (s *TcpServerSession) ProtocolCompatibility() (ProtocolCompatibility, bool) {
	if s == nil || s.protocol == nil {
		return ProtocolCompatibility{}, false
	}
	return s.protocol.Snapshot()
}

// GetObj 获取附加对象
// 返回:
//   - interface{}: 附加对象
func (s *TcpServerSession) GetObj() interface{} {
	return s.obj
}

// SetObj 设置附加对象
// 参数:
//   - obj: 要设置的对象
func (s *TcpServerSession) SetObj(obj interface{}) {
	s.obj = obj
}

// GetClientIP 获取客户端IP地址
// 返回:
//   - string: 客户端IP地址
func (s *TcpServerSession) GetClientIP() string {
	if s.conn != nil {
		return s.conn.RemoteAddr().(*net.TCPAddr).IP.String()
	}
	return ""
}
