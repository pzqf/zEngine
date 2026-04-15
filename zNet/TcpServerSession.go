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
	conn          *net.TCPConn
	sid           SessionIdType
	sendChan      chan *NetPacket
	receiveChan   chan *NetPacket
	wg            sync.WaitGroup
	lastHeartBeat time.Time
	ctxCancel     context.CancelFunc
	onClose       TcpCloseCallBackFunc
	closeOnce     sync.Once
	aesKey        []byte
	currentKey    atomic.Value
	currentKeyID  atomic.Uint32
	svr           *TcpServer    // 所属服务器
	obj           interface{}   // 附加对象
	sendSequence  atomic.Uint64 // 发送序列号
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
	newSession := TcpServerSession{
		conn:          conn,
		sid:           sid,
		sendChan:      make(chan *NetPacket, svr.config.ChanSize),
		receiveChan:   make(chan *NetPacket, svr.config.ChanSize),
		lastHeartBeat: time.Now(),
		onClose:       closeCallBack,
		aesKey:        aesKey,
		svr:           svr,
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
	data := notify.Marshal()
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

	go s.receive(ctx) // 启动接收协程

	// 根据配置决定是否启动处理协程
	if !s.svr.config.UseWorkerPool {
		// 传统模式：启动处理协程
		go s.process(ctx)
	}

	if s.svr.config.HeartbeatDuration > 0 {
		go s.heartbeatCheck(ctx) // 启动心跳检测协程
	}
}

// Close 关闭会话
// 取消上下文并等待所有goroutine退出
func (s *TcpServerSession) Close() {
	if s.ctxCancel != nil {
		s.ctxCancel()
	}
	s.wg.Wait()
}

// receive 接收数据
// 从TCP连接中读取数据包，解析并放入接收通道
//
// 参数:
//   - ctx: 上下文
func (s *TcpServerSession) receive(ctx context.Context) {
	s.wg.Add(1)
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

		// 检查流量是否超过限制（DDoS防护）
		if !s.svr.ddosProtection.AllowTraffic(clientIP, int64(n)) {
			if s.svr.logger != nil {
				s.svr.logger.Warn("Traffic limit exceeded from IP: %s, sid: %d", clientIP, s.sid)
			}
			break
		}

		// 解析数据包头
		netPacket := NetPacket{}
		if err = netPacket.UnmarshalHead(headBuf); err != nil {
			if s.svr.logger != nil {
				s.svr.logger.Error("Receive NetPacket,Unmarshal head error: %v, len: %d", err, len(headBuf))
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

			// 检查流量是否超过限制
			if !s.svr.ddosProtection.AllowTraffic(clientIP, int64(n)) {
				if s.svr.logger != nil {
					s.svr.logger.Warn("Traffic limit exceeded from IP: %s, sid: %d", clientIP, s.sid)
				}
				break
			}
		}

		// 检查数据包频率是否超过限制
		if !s.svr.ddosProtection.AllowPacket(clientIP) {
			if s.svr.logger != nil {
				s.svr.logger.Warn("Packet rate limit exceeded from IP: %s, sid: %d", clientIP, s.sid)
			}
			break
		}

		// 检查数据包大小是否超过限制
		if s.svr.config.MaxPacketDataSize > 0 && netPacket.DataSize > s.svr.config.MaxPacketDataSize {
			if s.svr.logger != nil {
				s.svr.logger.Warn("Receive NetPacket, Data size over max size, protoid:%d, data size:%d, max size: %d",
					netPacket.ProtoId, netPacket.DataSize, s.svr.config.MaxPacketDataSize)
			}
			continue
		}

		// 非心跳包处理
		if netPacket.ProtoId != HeartbeatProtoId {
			if s.svr.config.UseWorkerPool {
				// 工作池模式：提交任务到工作池
				packet := netPacket // 复制数据包
				s.svr.workerPool.Submit(func() error {
					s.processPacket(&packet)
					return nil
				})
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

	// 解密数据
	if packet.DataSize > 0 && !s.svr.config.DisableEncryption {
		key := s.GetDecryptKey(packet.KeyID)
		if key != nil {
			if decrypted, err := zCrypto.AESDecrypt(packet.Data, key, nil, zCrypto.AESModeGCM); err == nil {
				packet.Data = decrypted
			} else {
				if s.svr.logger != nil {
					s.svr.logger.Error("AES-GCM decrypt error: %v, sid:%d, keyID:%d", err, s.sid, packet.KeyID)
				}
				return
			}
		}
	}

	// 解压缩数据
	if packet.IsCompressed == CompressionSnappy && packet.DataSize > 0 {
		if decompressed, err := snappy.Decode(nil, packet.Data); err == nil {
			packet.Data = decompressed
			packet.DataSize = int32(len(decompressed))
		} else {
			if s.svr.logger != nil {
				s.svr.logger.Error("Decompress error: %v, sid:%d", err, s.sid)
			}
			return
		}
	}

	// 分发到消息处理器
	if s.svr.dispatcher != nil {
		err := s.svr.dispatcher(s, packet)
		if err != nil {
			if s.svr.logger != nil {
				s.svr.logger.Error("Dispatcher error: %v, ProtoId: %d", err, packet.ProtoId)
			}
		}
	}
}

// process 处理数据
// 从接收通道读取数据包，解密后分发到消息处理器；从发送通道读取数据包并发送
//
// 参数:
//   - ctx: 上下文
func (s *TcpServerSession) process(ctx context.Context) {
	s.wg.Add(1)
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

		case sendPacket := <-s.sendChan:
			// 发送数据包
			_, err := s.send(sendPacket)
			if err != nil {
				if s.svr.logger != nil {
					s.svr.logger.Error("Send NetPacket error:%v, ProtoId:%d", err, sendPacket.ProtoId)
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
			// 处理发送通道中剩余的消息
			for {
				if len(s.sendChan) > 0 {
					sendPacket := <-s.sendChan
					_, err := s.send(sendPacket)
					if err != nil {
						if s.svr.logger != nil {
							s.svr.logger.Error("Send NetPacket error:%v, ProtoId:%d", err, sendPacket.ProtoId)
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
	netPacket := NetPacket{
		ProtoId:   protoId,
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

	// 加密数据
	if key != nil && !s.svr.config.DisableEncryption {
		if encrypted, err := zCrypto.AESEncrypt(data, key, nil, zCrypto.AESModeGCM); err == nil {
			netPacket.Data = encrypted
		} else {
			if s.svr.logger != nil {
				s.svr.logger.Error("AES-GCM encrypt error: %v, ProtoId:%d", err, protoId)
			}
			return err
		}
	} else {
		netPacket.Data = data
	}

	// 压缩数据
	if !s.svr.config.DisableCompression && s.svr.compressionConfig.Enabled && len(netPacket.Data) > s.svr.compressionConfig.CompressionThreshold && len(netPacket.Data) <= s.svr.compressionConfig.MaxCompressSize {
		compressed := snappy.Encode(nil, netPacket.Data)
		// 只有当压缩后的数据小于原始数据时才使用压缩数据
		if len(compressed) < len(netPacket.Data) {
			netPacket.Data = compressed
			netPacket.IsCompressed = CompressionSnappy
		}
	}

	netPacket.DataSize = int32(len(netPacket.Data))
	// 校验数据包合法性
	if netPacket.ProtoId <= 0 || netPacket.DataSize < 0 {
		if s.svr.logger != nil {
			s.svr.logger.Error("Send packet illegal: protoId=%d, dataSize=%d", protoId, netPacket.DataSize)
		}
		return errors.New("send packet illegal")
	}
	// 检查数据包大小是否超过限制
	if s.svr.config.MaxPacketDataSize > 0 && netPacket.DataSize > s.svr.config.MaxPacketDataSize {
		if s.svr.logger != nil {
			s.svr.logger.Error("Send NetPacket, Data size over max size, data size :%d, max size: %d, protoId:%d",
				netPacket.DataSize, s.svr.config.MaxPacketDataSize, protoId)
		}
		return fmt.Errorf("send NetPacket, Data size over max size, data size :%d, max size: %d, protoId:%d",
			netPacket.DataSize, s.svr.config.MaxPacketDataSize, protoId)
	}

	s.sendChan <- &netPacket
	return nil
}

// send 发送数据包到TCP连接
// 参数:
//   - netPacket: 要发送的数据包
//
// 返回:
//   - int: 发送的字节数
//   - error: 发送失败时返回错误
func (s *TcpServerSession) send(netPacket *NetPacket) (int, error) {
	n, err := s.conn.Write(netPacket.Marshal())
	if err != nil {
		if s.svr.logger != nil {
			s.svr.logger.Error("Failed to write to connection: %v, ProtoId: %d", err, netPacket.ProtoId)
		}
		return 0, err
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
	s.wg.Add(1)
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
