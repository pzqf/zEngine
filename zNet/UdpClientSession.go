package zNet

import (
	"context"
	"errors"
	"fmt"
	"net"
	"sync"
	"time"

	"github.com/pzqf/zUtil/zCrypto"
)

type UdpClientSession struct {
	conn          *net.UDPConn
	sid           SessionIdType
	wg            sync.WaitGroup
	lastHeartBeat time.Time
	ctxCancel     context.CancelFunc
	aesKey        []byte
	dhExchange    *DHKeyExchange
	keyExchanged  bool

	cli *UdpClient
}

func (s *UdpClientSession) Init(cli *UdpClient, conn *net.UDPConn) {
	s.conn = conn
	s.sid = 1 // 客户端会话ID，简单设置为1
	s.lastHeartBeat = time.Now()
	s.cli = cli

	// 初始化ECDH密钥交换
	dhExchange, err := NewDHKeyExchange()
	if err != nil {
		if cli.logger != nil {
			cli.logger.Error("Failed to initialize DH key exchange: %v", err)
		}
	} else {
		s.dhExchange = dhExchange
		if cli.logger != nil {
			cli.logger.Info("Initialized ECDH key exchange for UDP client")
		}
	}
}

func (s *UdpClientSession) GetSid() SessionIdType {
	return s.sid
}

// GetClientIP 获取客户端IP地址
func (s *UdpClientSession) GetClientIP() string {
	if s.conn != nil {
		return s.conn.RemoteAddr().(*net.UDPAddr).IP.String()
	}
	return ""
}

func (s *UdpClientSession) Start() {
	if s.conn == nil {
		return
	}
	ctx, ctxCancel := context.WithCancel(context.Background())
	s.ctxCancel = ctxCancel

	go s.receive(ctx)
	if s.cli.heartbeatDuration > 0 {
		go s.heartbeatCheck(ctx)
	}

	// 启动后发送第一个数据包触发密钥交换
	s.triggerKeyExchange()

	return
}

func (s *UdpClientSession) triggerKeyExchange() {
	if s.dhExchange != nil && !s.keyExchanged {
		// 发送一个空数据包触发服务器的密钥交换流程
		nullData := make([]byte, 1)
		if _, err := s.conn.Write(nullData); err != nil {
			if s.cli.logger != nil {
				s.cli.logger.Error("Failed to trigger key exchange: %v", err)
			}
		}
	}
}

func (s *UdpClientSession) handleKeyExchange(data []byte) bool {
	if s.dhExchange != nil && !s.keyExchanged {
		if len(data) == 64 {
			// 收到服务器的公钥，执行密钥交换
			aesKey, err := s.dhExchange.ComputeSharedSecret(data)
			if err != nil {
				if s.cli.logger != nil {
					s.cli.logger.Error("Failed to compute shared secret: %v", err)
				}
			} else {
				s.aesKey = aesKey
				s.keyExchanged = true
				if s.cli.logger != nil {
					s.cli.logger.Info("ECDH key exchange successful, AES key length: %d", len(aesKey))
				}
				// 发送客户端的公钥给服务器
				clientPublicKey := s.dhExchange.GetPublicKey()
				if _, err := s.conn.Write(clientPublicKey); err != nil {
					if s.cli.logger != nil {
						s.cli.logger.Error("Failed to send client public key: %v", err)
					}
				}
			}
			// 密钥交换完成，清理资源
			s.dhExchange = nil
			return true
		}
	}
	return false
}

func (s *UdpClientSession) Send(protoId int32, data []byte) error {
	if s.conn == nil {
		return errors.New("udp connection is nil")
	}

	netPacket := NetPacket{
		ProtoId: protoId,
	}

	if s.aesKey != nil {
		encryptedData, err := zCrypto.AESEncrypt(data, s.aesKey, nil, zCrypto.AESModeGCM)
		if err != nil {
			if s.cli.logger != nil {
				s.cli.logger.Error("AESEncrypt error: %v", err)
			}
			return err
		}
		netPacket.Data = encryptedData
	} else {
		netPacket.Data = data
	}

	netPacket.DataSize = int32(len(netPacket.Data))
	if netPacket.ProtoId <= 0 || netPacket.DataSize < 0 {
		if s.cli.logger != nil {
			s.cli.logger.Error("Send UDP packet illegal: protoId=%d, dataSize=%d", protoId, netPacket.DataSize)
		}
		return errors.New("send UDP packet illegal")
	}

	if s.cli.maxPacketDataSize > 0 && netPacket.DataSize > s.cli.maxPacketDataSize {
		if s.cli.logger != nil {
			s.cli.logger.Error("Send UDP packet, Data size over max size, data size :%d, max size: %d, protoId:%d",
				netPacket.DataSize, s.cli.maxPacketDataSize, protoId)
		}
		return fmt.Errorf("send UDP packet, Data size over max size, data size :%d, max size: %d, protoId:%d",
			netPacket.DataSize, s.cli.maxPacketDataSize, protoId)
	}

	packetData := netPacket.Marshal()
	_, err := s.conn.Write(packetData)
	if err != nil {
		if s.cli.logger != nil {
			s.cli.logger.Error("Failed to send UDP packet: %v", err)
		}
		return err
	}

	return nil
}

func (s *UdpClientSession) receive(ctx context.Context) {
	s.wg.Add(1)
	defer s.wg.Done()

	buffer := make([]byte, 65535)
	for {
		select {
		case <-ctx.Done():
			return
		default:
			s.conn.SetReadDeadline(time.Now().Add(time.Second * 5))
			n, err := s.conn.Read(buffer)
			if err != nil {
				if netErr, ok := err.(net.Error); ok && netErr.Timeout() {
					continue
				}
				if s.cli.logger != nil {
					s.cli.logger.Error("UDP read error: %v", err)
				}
				return
			}

			data := buffer[:n]

			// 处理密钥交换
			if s.handleKeyExchange(data) {
				continue
			}

			// 解析数据包
			if len(data) < NetPacketHeadSize {
				if s.cli.logger != nil {
					s.cli.logger.Error("Received UDP packet too small: %d bytes", len(data))
				}
				continue
			}

			netPacket := NetPacket{}
			if err := netPacket.UnmarshalHead(data[:NetPacketHeadSize]); err != nil {
				if s.cli.logger != nil {
					s.cli.logger.Error("Unmarshal UDP packet head error: %v", err)
				}
				continue
			}

			if netPacket.DataSize > 0 {
				dataSize := int(netPacket.DataSize)
				if len(data) < NetPacketHeadSize+dataSize {
					if s.cli.logger != nil {
						s.cli.logger.Error("UDP packet data size mismatch: expected %d, got %d", dataSize, len(data)-NetPacketHeadSize)
					}
					continue
				}
				netPacket.Data = data[NetPacketHeadSize : NetPacketHeadSize+dataSize]
			}

			if s.cli.maxPacketDataSize > 0 && netPacket.DataSize > s.cli.maxPacketDataSize {
				if s.cli.logger != nil {
					s.cli.logger.Warn("UDP packet data size over max size: %d, max: %d", netPacket.DataSize, s.cli.maxPacketDataSize)
				}
				continue
			}

			if netPacket.DataSize > 0 && s.aesKey != nil {
				// 使用GCM模式解密
				plaintext, err := zCrypto.AESDecrypt(netPacket.Data, s.aesKey, nil, zCrypto.AESModeGCM)
				if err != nil {
					if s.cli.logger != nil {
						s.cli.logger.Error("AESDecrypt error: %v", err)
					}
					continue
				}
				netPacket.Data = plaintext
			}

			if netPacket.ProtoId != HeartbeatProtoId {
				if s.cli.dispatcher != nil {
					err := s.cli.dispatcher(s, &netPacket)
					if err != nil {
						if s.cli.logger != nil {
							s.cli.logger.Error("Dispatcher error: %v, ProtoId: %d", err, netPacket.ProtoId)
						}
					}
				}
			}

			s.heartbeatUpdate()
		}
	}
}

func (s *UdpClientSession) heartbeatCheck(ctx context.Context) {
	ticker := time.NewTicker(time.Duration(s.cli.heartbeatDuration) * time.Second)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			if time.Since(s.lastHeartBeat) > time.Duration(s.cli.heartbeatDuration*3)*time.Second {
				if s.cli.logger != nil {
					s.cli.logger.Warn("UDP client heartbeat timeout")
				}
				s.Close()
				return
			}

			// 发送心跳包
			s.Send(HeartbeatProtoId, nil)
		}
	}
}

func (s *UdpClientSession) heartbeatUpdate() {
	s.lastHeartBeat = time.Now()
}

func (s *UdpClientSession) Close() {
	s.ctxCancel()
	s.wg.Wait()
	if s.conn != nil {
		s.conn.Close()
	}
}
