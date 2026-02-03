package zNet

import (
	"context"
	"errors"
	"fmt"
	"sync"
	"time"

	"github.com/gorilla/websocket"
	"github.com/pzqf/zUtil/zCrypto"
)

type WebSocketClientSession struct {
	conn          *websocket.Conn
	sid           SessionIdType
	wg            sync.WaitGroup
	lastHeartBeat time.Time
	ctxCancel     context.CancelFunc
	aesKey        []byte

	cli *WebSocketClient
}

func (s *WebSocketClientSession) Init(cli *WebSocketClient, wsURL string) error {
	s.cli = cli
	s.sid = 1 // 客户端会话ID，简单设置为1
	s.lastHeartBeat = time.Now()

	conn, _, err := websocket.DefaultDialer.Dial(wsURL, nil)
	if err != nil {
		return err
	}

	s.conn = conn

	// 执行ECDH密钥交换
	aesKey, err := PerformKeyExchange(&websocketReaderWriter{conn})
	if err != nil {
		if s.cli.logger != nil {
			s.cli.logger.Error("Failed to perform ECDH key exchange: %v", err)
		}
		// 如果密钥交换失败，生成随机密钥作为备选方案
		aesKey, err = zCrypto.GenerateAESKey(zCrypto.AESKeySize16)
		if err != nil {
			if s.cli.logger != nil {
				s.cli.logger.Error("Failed to generate fallback AES key: %v", err)
			}
			s.aesKey = nil
		} else {
			if s.cli.logger != nil {
				s.cli.logger.Info("Generated fallback AES key, key length: %d", len(aesKey))
			}
			s.aesKey = aesKey
		}
	} else {
		if s.cli.logger != nil {
			s.cli.logger.Info("ECDH key exchange successful, AES key length: %d", len(aesKey))
		}
		s.aesKey = aesKey
	}

	return nil
}

func (s *WebSocketClientSession) GetSid() SessionIdType {
	return s.sid
}

func (s *WebSocketClientSession) Start() {
	if s.conn == nil {
		return
	}
	ctx, ctxCancel := context.WithCancel(context.Background())
	s.ctxCancel = ctxCancel

	go s.receive(ctx)
	if s.cli.heartbeatDuration > 0 {
		go s.heartbeatCheck(ctx)
	}

	return
}

func (s *WebSocketClientSession) Send(protoId int32, data []byte) error {
	if s.conn == nil {
		return errors.New("websocket connection is nil")
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
			s.cli.logger.Error("Send WebSocket packet illegal: protoId=%d, dataSize=%d", protoId, netPacket.DataSize)
		}
		return errors.New("send WebSocket packet illegal")
	}

	if s.cli.maxPacketDataSize > 0 && netPacket.DataSize > s.cli.maxPacketDataSize {
		if s.cli.logger != nil {
			s.cli.logger.Error("Send WebSocket packet, Data size over max size, data size :%d, max size: %d, protoId:%d",
				netPacket.DataSize, s.cli.maxPacketDataSize, protoId)
		}
		return fmt.Errorf("send WebSocket packet, Data size over max size, data size :%d, max size: %d, protoId:%d",
			netPacket.DataSize, s.cli.maxPacketDataSize, protoId)
	}

	packetData := netPacket.Marshal()
	err := s.conn.WriteMessage(websocket.BinaryMessage, packetData)
	if err != nil {
		if s.cli.logger != nil {
			s.cli.logger.Error("Failed to write to WebSocket: %v", err)
		}
		return err
	}

	return nil
}

func (s *WebSocketClientSession) receive(ctx context.Context) {
	s.wg.Add(1)
	defer s.wg.Done()

	for {
		select {
		case <-ctx.Done():
			return
		default:
			s.conn.SetReadDeadline(time.Now().Add(time.Second * 30))
			messageType, message, err := s.conn.ReadMessage()
			if err != nil {
				if s.cli.logger != nil {
					s.cli.logger.Error("WebSocket read error: %v", err)
				}
				return
			}

			if messageType != websocket.BinaryMessage {
				continue
			}

			data := message

			// 解析数据包
			if len(data) < NetPacketHeadSize {
				if s.cli.logger != nil {
					s.cli.logger.Error("Received WebSocket packet too small: %d bytes", len(data))
				}
				continue
			}

			netPacket := NetPacket{}
			if err := netPacket.UnmarshalHead(data[:NetPacketHeadSize]); err != nil {
				if s.cli.logger != nil {
					s.cli.logger.Error("Unmarshal WebSocket packet head error: %v", err)
				}
				continue
			}

			if netPacket.DataSize > 0 {
				dataSize := int(netPacket.DataSize)
				if len(data) < NetPacketHeadSize+dataSize {
					if s.cli.logger != nil {
						s.cli.logger.Error("WebSocket packet data size mismatch: expected %d, got %d", dataSize, len(data)-NetPacketHeadSize)
					}
					continue
				}
				netPacket.Data = data[NetPacketHeadSize : NetPacketHeadSize+dataSize]
			}

			if s.cli.maxPacketDataSize > 0 && netPacket.DataSize > s.cli.maxPacketDataSize {
				if s.cli.logger != nil {
					s.cli.logger.Warn("WebSocket packet data size over max size: %d, max: %d", netPacket.DataSize, s.cli.maxPacketDataSize)
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

func (s *WebSocketClientSession) heartbeatCheck(ctx context.Context) {
	ticker := time.NewTicker(time.Duration(s.cli.heartbeatDuration) * time.Second)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			if time.Since(s.lastHeartBeat) > time.Duration(s.cli.heartbeatDuration*3)*time.Second {
				if s.cli.logger != nil {
					s.cli.logger.Warn("WebSocket client heartbeat timeout")
				}
				s.Close()
				return
			}

			// 发送心跳包
			s.Send(HeartbeatProtoId, nil)
		}
	}
}

func (s *WebSocketClientSession) heartbeatUpdate() {
	s.lastHeartBeat = time.Now()
}

func (s *WebSocketClientSession) Close() {
	s.ctxCancel()
	s.wg.Wait()
	if s.conn != nil {
		s.conn.Close()
	}
}
