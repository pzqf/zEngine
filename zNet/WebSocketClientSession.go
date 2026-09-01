package zNet

import (
	"context"
	"errors"
	"sync"
	"time"

	"github.com/gorilla/websocket"
	"github.com/pzqf/zUtil/zCrypto"
)

type WebSocketClientSession struct {
	conn              *websocket.Conn
	sid               SessionIdType
	wg                sync.WaitGroup
	lastHeartBeat     time.Time
	ctxCancel         context.CancelFunc
	aesKey            []byte
	packetCodec       PacketCodec
	invalidPacketLogs packetErrorLogLimiter
	writeGate         contextWriteGate
	writeGateOnce     sync.Once

	cli *WebSocketClient
}

func (s *WebSocketClientSession) Init(cli *WebSocketClient, wsURL string) error {
	s.cli = cli
	s.packetCodec = cli.packetCodec
	s.sid = 1 // 客户端会话ID，简单设置为1
	s.lastHeartBeat = time.Now()

	conn, _, err := websocket.DefaultDialer.Dial(wsURL, nil)
	if err != nil {
		return err
	}

	s.conn = conn
	s.ensureWriteGate()

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

// GetClientIP 获取客户端IP地址
func (s *WebSocketClientSession) GetClientIP() string {
	if s.conn != nil {
		return s.conn.RemoteAddr().String()
	}
	return ""
}

func (s *WebSocketClientSession) Start() {
	if s.conn == nil {
		return
	}
	if maxSize := s.cli.wirePacketSize(); maxSize > 0 {
		s.conn.SetReadLimit(int64(NetPacketHeadSize) + int64(maxSize))
	}
	ctx, ctxCancel := context.WithCancel(context.Background())
	s.ctxCancel = ctxCancel

	go s.receive(ctx)
	if s.cli.heartbeatDuration > 0 {
		go s.heartbeatCheck(ctx)
	}

	return
}

func (s *WebSocketClientSession) Send(protoId ProtoIdType, data []byte) error {
	ctx, cancel := context.WithTimeout(context.Background(), DefaultSendTimeout)
	defer cancel()
	return s.SendWithOptions(ctx, protoId, data, SendOptions{Class: ReliableCommand})
}

func (s *WebSocketClientSession) SendContext(ctx context.Context, protoId ProtoIdType, data []byte) error {
	return s.SendWithOptions(ctx, protoId, data, SendOptions{Class: ReliableCommand})
}

func (s *WebSocketClientSession) SendWithOptions(ctx context.Context, protoId ProtoIdType, data []byte, options SendOptions) error {
	ctx, cancel := normalizeSendContext(ctx, DefaultSendTimeout)
	defer cancel()
	if s.conn == nil {
		return errors.New("websocket connection is nil")
	}
	gate := s.ensureWriteGate()
	acquired, err := gate.acquire(ctx, options.Class)
	if err != nil || !acquired {
		return err
	}
	defer gate.release()

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
		netPacket.Data = append([]byte(nil), data...)
	}

	netPacket.DataSize = int32(len(netPacket.Data))
	if err := ValidatePacketHeader(&netPacket, s.cli.wirePacketSize()); err != nil {
		if s.cli.logger != nil {
			s.cli.logger.Error("Send WebSocket packet illegal: %v", err)
		}
		return err
	}

	packetData := s.packetCodec.Marshal(&netPacket)
	deadline, _ := ctx.Deadline()
	if err := s.conn.SetWriteDeadline(deadline); err != nil {
		return err
	}
	defer s.conn.SetWriteDeadline(time.Time{})
	err = s.conn.WriteMessage(websocket.BinaryMessage, packetData)
	if err != nil {
		if s.cli.logger != nil {
			s.cli.logger.Error("Failed to write to WebSocket: %v", err)
		}
		return err
	}

	return nil
}

func (s *WebSocketClientSession) ensureWriteGate() contextWriteGate {
	s.writeGateOnce.Do(func() { s.writeGate = newContextWriteGate() })
	return s.writeGate
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

			// WebSocket 消息保留帧边界：非法帧只丢当前消息。
			netPacket, decodeErr := s.packetCodec.DecodeFrame(data, s.cli.wirePacketSize())
			if decodeErr != nil {
				if s.cli.logger != nil && s.invalidPacketLogs.Allow(time.Now()) {
					s.cli.logger.Error("Invalid WebSocket packet: %v", decodeErr)
				}
				continue
			}

			encrypted := len(netPacket.Data) > 0 && s.aesKey != nil
			if err := decodePacketPayload(&netPacket, s.aesKey, encrypted, s.cli.decodedPacketSize()); err != nil {
				if s.cli.logger != nil && s.invalidPacketLogs.Allow(time.Now()) {
					s.cli.logger.Error("Decode WebSocket payload error: %v", err)
				}
				continue
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
