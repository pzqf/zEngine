package zNet

import (
	"context"
	"errors"
	"sync"
	"time"

	"github.com/gorilla/websocket"
	"github.com/pzqf/zUtil/zCrypto"
)

type WebSocketServerSession struct {
	conn              *websocket.Conn
	sid               SessionIdType
	sendQueue         *outboundQueue
	receiveChan       chan *NetPacket
	wg                sync.WaitGroup
	lastHeartBeat     time.Time
	ctxCancel         context.CancelFunc
	onClose           WebSocketCloseCallBackFunc
	closeOnce         sync.Once
	aesKey            []byte
	svr               *WebSocketServer
	obj               interface{}
	packetCodec       PacketCodec
	invalidPacketLogs packetErrorLogLimiter
}

type WebSocketCloseCallBackFunc func(c *WebSocketServerSession)

func (s *WebSocketServerSession) triggerOnClose() {
	s.closeOnce.Do(func() {
		if s.onClose != nil {
			s.onClose(s)
		}
	})
}

func NewWebSocketServerSession(svr *WebSocketServer, conn *websocket.Conn, sid SessionIdType, closeCallBack WebSocketCloseCallBackFunc, aesKey []byte) *WebSocketServerSession {
	newSession := WebSocketServerSession{
		conn:          conn,
		sid:           sid,
		sendQueue:     newOutboundQueue(svr.config.ChanSize, backpressureMetrics(svr.metrics)),
		receiveChan:   make(chan *NetPacket, svr.config.ChanSize),
		lastHeartBeat: time.Now(),
		onClose:       closeCallBack,
		aesKey:        aesKey,
		svr:           svr,
		packetCodec:   svr.packetCodec,
	}
	return &newSession
}

func (s *WebSocketServerSession) Start() {
	if s.conn == nil {
		return
	}
	if maxSize := s.svr.config.MaxWirePacketSize; maxSize > 0 {
		s.conn.SetReadLimit(int64(NetPacketHeadSize) + int64(maxSize))
	}
	ctx, ctxCancel := context.WithCancel(context.Background())
	s.ctxCancel = ctxCancel

	// NET-4: wg.Add 必须在 go 之前，否则 Close 的 wg.Wait 可能在 goroutine 尚未 Add 时看到计数 0
	// 提前返回，既不等待又与 goroutine 竞争（与 TCP 会话同类修复）。
	s.wg.Add(1)
	go s.receive(ctx)
	s.wg.Add(1)
	go s.process(ctx)
	if s.svr.config.HeartbeatDuration > 0 {
		s.wg.Add(1)
		go s.heartbeatCheck(ctx)
	}
}

func (s *WebSocketServerSession) Close() {
	s.sendQueue.Close(ErrSessionClosed)
	if s.ctxCancel != nil {
		s.ctxCancel()
	}
	s.wg.Wait()
}

func (s *WebSocketServerSession) receive(ctx context.Context) {
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

	for {
		if ctx.Err() != nil {
			break
		}

		_, data, err := s.conn.ReadMessage()
		if err != nil {
			if s.svr.logger != nil {
				s.svr.logger.Error("WebSocket read error: %v, sid:%d, closed", err, s.sid)
			}
			break
		}

		// WebSocket 消息保留帧边界：非法帧只丢当前消息。
		netPacket, decodeErr := s.packetCodec.DecodeFrame(data, s.svr.config.MaxWirePacketSize)
		if decodeErr != nil {
			recordPacketDecodeError(s.svr.metrics, decodeErr)
			if s.svr.logger != nil && s.invalidPacketLogs.Allow(time.Now()) {
				s.svr.logger.Error("Invalid WebSocket packet: %v", decodeErr)
			}
			continue
		}

		if netPacket.ProtoId != HeartbeatProtoId {
			s.receiveChan <- &netPacket
		}

		s.heartbeatUpdate()
	}
	s.ctxCancel()
}

// dispatchSafely 调用 dispatcher 处理单个包，并隔离其 error 与 panic——绝不因单包问题拆掉会话（NET-4）。
func (s *WebSocketServerSession) dispatchSafely(receivePacket *NetPacket) {
	defer func() {
		if r := recover(); r != nil && s.svr.logger != nil {
			s.svr.logger.Error("Dispatcher panic: %v, ProtoId: %d", r, receivePacket.ProtoId)
		}
	}()
	if err := s.svr.dispatcher(s, receivePacket); err != nil && s.svr.logger != nil {
		s.svr.logger.Error("Dispatcher error: %v, ProtoId: %d", err, receivePacket.ProtoId)
	}
}

func (s *WebSocketServerSession) processReceivedPacket(receivePacket *NetPacket) {
	encrypted := len(receivePacket.Data) > 0 && s.aesKey != nil
	if err := decodePacketPayload(receivePacket, s.aesKey, encrypted, s.svr.config.MaxDecodedPacketSize); err != nil {
		recordPacketDecodeError(s.svr.metrics, err)
		if s.svr.logger != nil && s.invalidPacketLogs.Allow(time.Now()) {
			s.svr.logger.Error("Decode WebSocket payload error: %v, sid:%d", err, s.sid)
		}
		return
	}
	if s.svr.dispatcher != nil {
		s.dispatchSafely(receivePacket)
	}
}

func (s *WebSocketServerSession) process(ctx context.Context) {
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
			s.processReceivedPacket(receivePacket)

		case <-s.sendQueue.Ready():
			sendPacket, ok := s.sendQueue.TryDequeue()
			if !ok {
				continue
			}
			_, err := s.sendOutbound(sendPacket)
			if err != nil {
				if s.svr.logger != nil {
					s.svr.logger.Error("Send WebSocket packet error:%v, ProtoId:%d", err, sendPacket.packet.ProtoId)
				}
			}
		case <-ctx.Done():
			for {
				if len(s.receiveChan) > 0 {
					receivePacket := <-s.receiveChan
					s.processReceivedPacket(receivePacket)

					continue
				}
				break
			}
			s.sendQueue.Close(ErrSessionClosed)
			for {
				if sendPacket, ok := s.sendQueue.TryDequeue(); ok {
					_, err := s.sendOutbound(sendPacket)
					if err != nil {
						if s.svr.logger != nil {
							s.svr.logger.Error("Send WebSocket packet error:%v, ProtoId:%d", err, sendPacket.packet.ProtoId)
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

	if err := s.conn.Close(); err != nil {
		if s.svr.logger != nil {
			s.svr.logger.Error("Failed to close WebSocket connection: %v, sid: %d", err, s.sid)
		}
	}
	s.triggerOnClose()
}

func (s *WebSocketServerSession) Send(protoId ProtoIdType, data []byte) error {
	ctx, cancel := context.WithTimeout(context.Background(), DefaultSendTimeout)
	defer cancel()
	return s.SendWithOptions(ctx, protoId, data, SendOptions{Class: ReliableCommand})
}

func (s *WebSocketServerSession) SendContext(ctx context.Context, protoId ProtoIdType, data []byte) error {
	return s.SendWithOptions(ctx, protoId, data, SendOptions{Class: ReliableCommand})
}

func (s *WebSocketServerSession) SendWithOptions(ctx context.Context, protoId ProtoIdType, data []byte, options SendOptions) error {
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

func (s *WebSocketServerSession) buildOutboundPacket(protoId ProtoIdType, data []byte) (*NetPacket, error) {
	netPacket := NetPacket{
		ProtoId: protoId,
	}
	if s.aesKey != nil {
		encryptedData, err := zCrypto.AESEncrypt(data, s.aesKey, nil, zCrypto.AESModeGCM)
		if err != nil {
			if s.svr.logger != nil {
				s.svr.logger.Error("AESEncrypt error: %v", err)
			}
			return nil, err
		}
		netPacket.Data = encryptedData
	} else {
		netPacket.Data = append([]byte(nil), data...)
	}
	netPacket.DataSize = int32(len(netPacket.Data))
	if err := ValidatePacketHeader(&netPacket, s.svr.config.MaxWirePacketSize); err != nil {
		if s.svr.logger != nil {
			s.svr.logger.Error("Send WebSocket packet illegal: %v", err)
		}
		return nil, err
	}
	return &netPacket, nil
}

func (s *WebSocketServerSession) sendOutbound(outbound *outboundPacket) (int, error) {
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

func (s *WebSocketServerSession) send(netPacket *NetPacket) (int, error) {
	data := s.packetCodec.Marshal(netPacket)
	err := s.conn.WriteMessage(websocket.BinaryMessage, data)
	if err != nil {
		if s.svr.logger != nil {
			s.svr.logger.Error("Failed to write to WebSocket: %v, ProtoId: %d", err, netPacket.ProtoId)
		}
		return 0, err
	}
	return len(data), nil
}

func (s *WebSocketServerSession) heartbeatUpdate() {
	s.lastHeartBeat = time.Now()
}

func (s *WebSocketServerSession) heartbeatCheck(ctx context.Context) {
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

func (s *WebSocketServerSession) GetSid() SessionIdType {
	return s.sid
}

func (s *WebSocketServerSession) GetObj() interface{} {
	return s.obj
}

// SetObj 设置附加对象
func (s *WebSocketServerSession) SetObj(obj interface{}) {
	s.obj = obj
}

// GetClientIP 获取客户端IP地址
func (s *WebSocketServerSession) GetClientIP() string {
	if s.conn != nil {
		return s.conn.RemoteAddr().String()
	}
	return ""
}
