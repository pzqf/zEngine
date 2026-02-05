package zNet

import (
	"context"
	"errors"
	"fmt"
	"io"
	"log"
	"net"
	"sync"
	"time"

	"github.com/pzqf/zUtil/zCrypto"
)

// TcpClientSession TCP客户端会话
// 管理与服务器的TCP连接，负责数据收发、心跳检测、加密解密
type TcpClientSession struct {
	conn          *net.TCPConn       // TCP连接
	wg            sync.WaitGroup     // 等待组
	lastHeartBeat time.Time          // 最后心跳时间
	ctxCancel     context.CancelFunc // 上下文取消函数
	aesKey        []byte             // AES加密密钥

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
	if s.cli.heartbeatDuration > 0 {
		go s.heartbeatCheck(ctx)
	}
}

// Close 关闭会话
// 取消上下文并等待所有goroutine退出
func (s *TcpClientSession) Close() {
	s.ctxCancel()
	s.wg.Wait()
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
		if netPacket.DataSize > s.cli.maxPacketDataSize {
			if s.cli.logger != nil {
				s.cli.logger.Warn("Receive NetPacket, Data size over max size, protoid:%d, data size:%d, max size: %d",
					netPacket.ProtoId, netPacket.DataSize, s.cli.maxPacketDataSize)
			}
			continue
		}

		// 解密数据
		if netPacket.DataSize > 0 && s.aesKey != nil {
			if decrypted, err := zCrypto.AESDecrypt(netPacket.Data, s.aesKey, nil, zCrypto.AESModeGCM); err == nil {
				netPacket.Data = decrypted
			} else {
				if s.cli.logger != nil {
					s.cli.logger.Error("AES-GCM decrypt error: %v", err)
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
func (s *TcpClientSession) Send(protoId int32, data []byte) error {
	netPacket := NetPacket{
		ProtoId: protoId,
	}

	if data != nil {
		// 加密数据
		if s.aesKey != nil {
			if encrypted, err := zCrypto.AESEncrypt(data, s.aesKey, nil, zCrypto.AESModeGCM); err == nil {
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
	}

	netPacket.DataSize = int32(len(netPacket.Data))
	// 校验数据包合法性
	if netPacket.ProtoId <= 0 || netPacket.DataSize < 0 {
		return errors.New("send packet illegal")
	}
	// 检查数据包大小是否超过限制
	if netPacket.DataSize > s.cli.maxPacketDataSize {
		return fmt.Errorf("send NetPacket, Data size over max size, data size :%d, max size: %d, protoId:%d",
			netPacket.DataSize, s.cli.maxPacketDataSize, protoId)
	}

	// 发送数据包
	_, err := s.conn.Write(netPacket.Marshal())
	if err != nil {
		return err
	}
	s.heartbeatUpdate() // 更新心跳时间
	return nil
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
	hbd := float64(s.cli.heartbeatDuration)
	for {
		select {
		case <-time.After(30 * time.Second):
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
