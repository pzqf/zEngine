package zNet

import (
	"context"
	"errors"
	"fmt"
	"io"
	"log"
	"net"
	"sync"
	"sync/atomic"
	"time"

	"github.com/golang/snappy"
	"github.com/pzqf/zUtil/zCrypto"
)

// TcpClientSession TCP客户端会话
// 管理与服务器的TCP连接，负责数据收发、心跳检测、加密解密
type TcpClientSession struct {
	conn          *net.TCPConn       // TCP连接
	wg            sync.WaitGroup     // 等待组
	lastHeartBeat time.Time          // 最后心跳时间
	ctxCancel     context.CancelFunc // 上下文取消函数
	aesKey        []byte             // AES加密密钥（初始密钥）
	currentKey    atomic.Value       // 当前密钥（支持密钥轮换）
	currentKeyID  atomic.Uint32      // 当前密钥ID
	closed        atomic.Bool        // 是否已关闭
	sendSequence  atomic.Uint64      // 发送序列号

	cli *TcpClient // 所属客户端
}

// Init 初始化会话
// 参数:
//   - cli: 所属客户端
//   - conn: TCP连接
//   - aesKey: AES加密密钥
func (s *TcpClientSession) Init(cli *TcpClient, conn *net.TCPConn, aesKey []byte) {
	s.conn = conn
	s.lastHeartBeat = time.Now()
	s.aesKey = aesKey
	s.cli = cli
	// 初始化当前密钥
	s.currentKey.Store(aesKey)
	s.currentKeyID.Store(0) // 0表示使用初始密钥
}

// UpdateKey 更新密钥（密钥轮换时调用）
// 参数:
//   - newKey: 新密钥
//   - newKeyID: 新密钥ID
func (s *TcpClientSession) UpdateKey(newKey []byte, newKeyID uint32) {
	s.currentKey.Store(newKey)
	s.currentKeyID.Store(newKeyID)
}

// GetDecryptKey 获取解密密钥（根据密钥ID）
// 参数:
//   - keyID: 密钥ID
//
// 返回:
//   - []byte: 密钥
func (s *TcpClientSession) GetDecryptKey(keyID uint32) []byte {
	if keyID == 0 {
		return s.aesKey
	}
	if keyID == s.currentKeyID.Load() {
		return s.currentKey.Load().([]byte)
	}
	return nil
}

// Start 启动会话
// 启动接收和心跳检测goroutine
func (s *TcpClientSession) Start() {
	if s.conn == nil {
		return
	}
	ctx, ctxCancel := context.WithCancel(context.Background())
	s.ctxCancel = ctxCancel

	go s.receive(ctx)
	if s.cli.config.HeartbeatDuration > 0 {
		go s.heartbeatCheck(ctx)
	}
}

// Close 关闭会话
// 取消上下文并等待所有goroutine退出
func (s *TcpClientSession) Close() {
	s.closed.Store(true)
	if s.ctxCancel != nil {
		s.ctxCancel()
	}
	s.wg.Wait()
}

// IsClosed 检查会话是否已关闭
//
// 返回:
//   - bool: 是否已关闭
func (s *TcpClientSession) IsClosed() bool {
	return s.closed.Load()
}

// receive 接收数据
// 从TCP连接中读取数据包，解密后分发到消息处理器
//
// 参数:
//   - ctx: 上下文
func (s *TcpClientSession) receive(ctx context.Context) {
	s.wg.Add(1)
	defer s.ctxCancel()
	defer s.wg.Done()
	defer func() {
		if err := recover(); err != nil {
			if s.cli.logger != nil {
				s.cli.logger.Error("process panic:%v, closed", err)
			}
		}
	}()
	for {
		if ctx.Err() != nil {
			break
		}

		// 设置读取超时时间
		_ = s.conn.SetReadDeadline(time.Now().Add(time.Second * 3))

		// 读取数据包头
		headBuf := make([]byte, NetPacketHeadSize)
		n, err := io.ReadFull(s.conn, headBuf)
		if err != nil {
			var netErr net.Error
			if errors.As(err, &netErr) {
				if netErr.Timeout() {
					continue // 超时继续等待
				}
			}
			if s.cli.logger != nil {
				s.cli.logger.Error("Client conn read error, error:%v, closed", err)
			}
			break
		}

		if n != NetPacketHeadSize {
			if s.cli.logger != nil {
				s.cli.logger.Error("Client conn read error, error:head size error %d, closed", n)
			}
			break
		}

		// 解析数据包头
		netPacket := NetPacket{}
		if err = netPacket.UnmarshalHead(headBuf); err != nil {
			if s.cli.logger != nil {
				s.cli.logger.Error("Receive NetPacket,Unmarshal head error: %v, len: %d", err, len(headBuf))
			}
			break
		}

		// 读取数据包体
		if netPacket.DataSize > 0 {
			netPacket.Data = make([]byte, int(netPacket.DataSize))
			n, err = io.ReadFull(s.conn, netPacket.Data)
			if err != nil {
				if s.cli.logger != nil {
					s.cli.logger.Error("Client conn read data error:%v, closed", err)
				}
				break
			}

			if netPacket.DataSize != int32(n) {
				if s.cli.logger != nil {
					s.cli.logger.Error("Receive NetPacket, Data size error,protoid:%d, DataSize:%d, received:%d",
						netPacket.ProtoId, netPacket.DataSize, n)
				}
				break
			}
		}

		// 校验协议ID
		if netPacket.ProtoId < 0 {
			log.Printf("receive NetPacket protoid empty")
			continue
		}

		// 检查数据包大小是否超过限制
		if netPacket.DataSize > s.cli.config.MaxPacketDataSize {
			if s.cli.logger != nil {
				s.cli.logger.Warn("Receive NetPacket, Data size over max size, protoid:%d, data size:%d, max size: %d",
					netPacket.ProtoId, netPacket.DataSize, s.cli.config.MaxPacketDataSize)
			}
			continue
		}

		// 处理密钥轮换通知
		if netPacket.ProtoId == KeyRotationNotifyProtoId {
			s.handleKeyRotationNotify(&netPacket)
			continue
		}

		// 解密数据
		if netPacket.DataSize > 0 && !s.cli.config.DisableEncryption {
			key := s.GetDecryptKey(netPacket.KeyID)
			if key != nil {
				if decrypted, err := zCrypto.AESDecrypt(netPacket.Data, key, nil, zCrypto.AESModeGCM); err == nil {
					netPacket.Data = decrypted
				} else {
					if s.cli.logger != nil {
						s.cli.logger.Error("AES-GCM decrypt error: %v, keyID:%d", err, netPacket.KeyID)
					}
				}
			}
		}

		// 解压缩数据
		if netPacket.IsCompressed == CompressionSnappy && netPacket.DataSize > 0 {
			if decompressed, err := snappy.Decode(nil, netPacket.Data); err == nil {
				netPacket.Data = decompressed
				netPacket.DataSize = int32(len(decompressed))
			} else {
				if s.cli.logger != nil {
					s.cli.logger.Error("Decompress error: %v", err)
				}
			}
		}

		// 分发到消息处理器（异步）
		go func() {
			err = s.cli.dispatcher(s, &netPacket)
			if err != nil {
				if s.cli.logger != nil {
					s.cli.logger.Error("Dispatcher NetPacket error,%v, ProtoId:%d", err, netPacket.ProtoId)
				}
			}
		}()

	}
	s.ctxCancel()
}

// Send 发送数据
// 将数据加密后发送到服务器
//
// 参数:
//   - protoId: 协议ID
//   - data: 要发送的数据
//
// 返回:
//   - error: 发送失败时返回错误
func (s *TcpClientSession) Send(protoId ProtoIdType, data []byte) error {
	netPacket := NetPacket{
		ProtoId:   protoId,
		Sequence:  s.sendSequence.Add(1),
		Timestamp: time.Now().Unix(),
	}

	// 获取加密密钥
	var key []byte
	var keyID uint32
	if s.currentKeyID.Load() > 0 {
		key = s.currentKey.Load().([]byte)
		keyID = s.currentKeyID.Load()
	} else if !s.cli.config.DisableEncryption {
		key = s.aesKey
	}
	netPacket.KeyID = keyID

	if data != nil {
		// 加密数据
		if key != nil && !s.cli.config.DisableEncryption {
			if encrypted, err := zCrypto.AESEncrypt(data, key, nil, zCrypto.AESModeGCM); err == nil {
				netPacket.Data = encrypted
			} else {
				if s.cli.logger != nil {
					s.cli.logger.Error("AES-GCM encrypt error: %v", err)
				}
				return err
			}
		} else {
			netPacket.Data = data
		}

		// 压缩数据
		if s.cli.config.Compression.Enabled && len(netPacket.Data) > s.cli.config.Compression.CompressionThreshold && len(netPacket.Data) <= s.cli.config.Compression.MaxCompressSize {
			compressed := snappy.Encode(nil, netPacket.Data)
			// 只有当压缩后的数据小于原始数据时才使用压缩数据
			if len(compressed) < len(netPacket.Data) {
				netPacket.Data = compressed
				netPacket.IsCompressed = CompressionSnappy
			}
		}
	}

	netPacket.DataSize = int32(len(netPacket.Data))
	// 校验数据包合法性
	if netPacket.ProtoId <= 0 || netPacket.DataSize < 0 {
		return errors.New("send packet illegal")
	}
	// 检查数据包大小是否超过限制
	if netPacket.DataSize > s.cli.config.MaxPacketDataSize {
		return fmt.Errorf("send NetPacket, Data size over max size, data size :%d, max size: %d, protoId:%d",
			netPacket.DataSize, s.cli.config.MaxPacketDataSize, protoId)
	}

	// 发送数据包
	_, err := s.conn.Write(netPacket.Marshal())
	if err != nil {
		return err
	}
	s.heartbeatUpdate() // 更新心跳时间
	return nil
}

// handleKeyRotationNotify 处理密钥轮换通知
// 参数:
//   - packet: 密钥轮换通知数据包
func (s *TcpClientSession) handleKeyRotationNotify(packet *NetPacket) {
	if packet.DataSize == 0 {
		return
	}

	// 使用当前密钥解密通知数据
	key := s.GetDecryptKey(packet.KeyID)
	if key == nil {
		if s.cli.logger != nil {
			s.cli.logger.Error("Failed to get decrypt key for key rotation notify, keyID:%d", packet.KeyID)
		}
		return
	}

	decryptedData, err := zCrypto.AESDecrypt(packet.Data, key, nil, zCrypto.AESModeGCM)
	if err != nil {
		if s.cli.logger != nil {
			s.cli.logger.Error("Failed to decrypt key rotation notify: %v", err)
		}
		return
	}

	var notify KeyRotationNotify
	if !notify.Unmarshal(decryptedData) {
		if s.cli.logger != nil {
			s.cli.logger.Error("Failed to unmarshal key rotation notify")
		}
		return
	}

	// 验证时间戳
	now := time.Now().Unix()
	if now-notify.Timestamp > 30 || now-notify.Timestamp < -30 {
		if s.cli.logger != nil {
			s.cli.logger.Warn("Key rotation notify timestamp out of tolerance: %d", notify.Timestamp)
		}
		return
	}

	// 这里需要从安全通道获取新密钥
	// 简化处理：使用通知中的KeyID作为新密钥的标识
	// 实际应用中应该通过安全通道协商新密钥
	s.currentKeyID.Store(notify.KeyID)

	if s.cli.logger != nil {
		s.cli.logger.Info("Key rotation notify received, new key ID: %d", notify.KeyID)
	}
}

// heartbeatUpdate 更新心跳时间
func (s *TcpClientSession) heartbeatUpdate() {
	s.lastHeartBeat = time.Now()
}

// heartbeatCheck 心跳检测
// 定期发送心跳包保持连接
//
// 参数:
//   - ctx: 上下文
func (s *TcpClientSession) heartbeatCheck(ctx context.Context) {
	s.wg.Add(1)
	defer s.wg.Done()
	hbd := float64(s.cli.config.HeartbeatDuration)
	for {
		select {
		case <-time.After(time.Duration(s.cli.config.HeartbeatDuration) * time.Second):
			if time.Since(s.lastHeartBeat).Seconds() >= hbd {
				_ = s.Send(HeartbeatProtoId, nil)
			}
		case <-ctx.Done():
			return
		}
	}
}

// GetSid 获取会话ID
// 客户端会话固定返回1
//
// 返回:
//   - SessionIdType: 会话ID
func (s *TcpClientSession) GetSid() SessionIdType {
	return SessionIdType(1)
}

// GetClientIP 获取客户端IP地址
//
// 返回:
//   - string: 客户端IP地址
func (s *TcpClientSession) GetClientIP() string {
	if s.conn != nil {
		return s.conn.RemoteAddr().(*net.TCPAddr).IP.String()
	}
	return ""
}
