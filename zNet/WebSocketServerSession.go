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

type WebSocketServerSession struct {
	conn          *websocket.Conn
	sid           SessionIdType
	sendChan      chan *NetPacket
	receiveChan   chan *NetPacket
	wg            sync.WaitGroup
	lastHeartBeat time.Time
	ctxCancel     context.CancelFunc
	onClose       WebSocketCloseCallBackFunc
	aesKey        []byte
	svr           *WebSocketServer
	obj           interface{}
}

type WebSocketCloseCallBackFunc func(c *WebSocketServerSession)

func NewWebSocketServerSession(svr *WebSocketServer, conn *websocket.Conn, sid SessionIdType, closeCallBack WebSocketCloseCallBackFunc, aesKey []byte) *WebSocketServerSession {
	newSession := WebSocketServerSession{
		conn:          conn,
		sid:           sid,
		sendChan:      make(chan *NetPacket, svr.config.ChanSize),
		receiveChan:   make(chan *NetPacket, svr.config.ChanSize),
		lastHeartBeat: time.Now(),
		onClose:       closeCallBack,
		aesKey:        aesKey,
		svr:           svr,
	}
	return &newSession
}

func (s *WebSocketServerSession) Start() {
	if s.conn == nil {
		return
	}
	ctx, ctxCancel := context.WithCancel(context.Background())
	s.ctxCancel = ctxCancel

	go s.receive(ctx)
	go s.process(ctx)
	if s.svr.config.HeartbeatDuration > 0 {
		go s.heartbeatCheck(ctx)
	}
}

func (s *WebSocketServerSession) Close() {
	s.ctxCancel()
	s.wg.Wait()
}

func (s *WebSocketServerSession) receive(ctx context.Context) {
	s.wg.Add(1)
	defer s.ctxCancel()
	defer s.wg.Done()
	defer func() {
		if s.onClose != nil {
			s.onClose(s)
		}
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

		// 解析数据包
		if len(data) < NetPacketHeadSize {
			if s.svr.logger != nil {
				s.svr.logger.Error("WebSocket packet too small: %d bytes", len(data))
			}
			continue
		}

		netPacket := NetPacket{}
		if err := netPacket.UnmarshalHead(data[:NetPacketHeadSize]); err != nil {
			if s.svr.logger != nil {
				s.svr.logger.Error("WebSocket packet unmarshal head error: %v", err)
			}
			continue
		}

		if netPacket.DataSize > 0 {
			dataSize := int(netPacket.DataSize)
			if len(data) < NetPacketHeadSize+dataSize {
				if s.svr.logger != nil {
					s.svr.logger.Error("WebSocket packet data size mismatch: expected %d, got %d", dataSize, len(data)-NetPacketHeadSize)
				}
				continue
			}
			netPacket.Data = data[NetPacketHeadSize : NetPacketHeadSize+dataSize]
		}

		if s.svr.config.MaxPacketDataSize > 0 && netPacket.DataSize > s.svr.config.MaxPacketDataSize {
			if s.svr.logger != nil {
				s.svr.logger.Warn("WebSocket packet data size over max size: %d, max: %d", netPacket.DataSize, s.svr.config.MaxPacketDataSize)
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

func (s *WebSocketServerSession) process(ctx context.Context) {
	s.wg.Add(1)
	defer s.wg.Done()
	defer func() {
		if s.onClose != nil {
			s.onClose(s)
		}
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
			if receivePacket.DataSize > 0 && s.aesKey != nil {
				// 使用GCM模式解密
				plaintext, err := zCrypto.AESDecrypt(receivePacket.Data, s.aesKey, nil, zCrypto.AESModeGCM)
				if err != nil {
					if s.svr.logger != nil {
						s.svr.logger.Error("AESDecrypt error: %v", err)
					}
					continue
				}
				receivePacket.Data = plaintext
			}
			if s.svr.dispatcher != nil {
				err := s.svr.dispatcher(s, receivePacket)
				if err != nil {
					if s.svr.logger != nil {
						s.svr.logger.Error("Dispatcher error: %v, ProtoId: %d", err, receivePacket.ProtoId)
					}
					return
				}
			}

		case sendPacket := <-s.sendChan:
			_, err := s.send(sendPacket)
			if err != nil {
				if s.svr.logger != nil {
					s.svr.logger.Error("Send WebSocket packet error:%v, ProtoId:%d", err, sendPacket.ProtoId)
				}
			}
		case <-ctx.Done():
			for {
				if len(s.receiveChan) > 0 {
					receivePacket := <-s.receiveChan
					if s.svr.dispatcher != nil {
						err := s.svr.dispatcher(s, receivePacket)
						if err != nil {
							if s.svr.logger != nil {
								s.svr.logger.Error("Dispatcher error: %v, ProtoId: %d", err, receivePacket.ProtoId)
							}
							return
						}
					}

					continue
				}
				break
			}
			for {
				if len(s.sendChan) > 0 {
					sendPacket := <-s.sendChan
					_, err := s.send(sendPacket)
					if err != nil {
						if s.svr.logger != nil {
							s.svr.logger.Error("Send WebSocket packet error:%v, ProtoId:%d", err, sendPacket.ProtoId)
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
	if s.onClose != nil {
		s.onClose(s)
	}
}

func (s *WebSocketServerSession) Send(protoId int32, data []byte) error {
	netPacket := NetPacket{
		ProtoId: protoId,
	}
	if s.aesKey != nil {
		encryptedData, err := zCrypto.AESEncrypt(data, s.aesKey, nil, zCrypto.AESModeGCM)
		if err != nil {
			if s.svr.logger != nil {
				s.svr.logger.Error("AESEncrypt error: %v", err)
			}
			return err
		}
		netPacket.Data = encryptedData
	} else {
		netPacket.Data = data
	}
	netPacket.DataSize = int32(len(netPacket.Data))
	if netPacket.ProtoId <= 0 || netPacket.DataSize < 0 {
		if s.svr.logger != nil {
			s.svr.logger.Error("Send WebSocket packet illegal: protoId=%d, dataSize=%d", protoId, netPacket.DataSize)
		}
		return errors.New("send WebSocket packet illegal")
	}
	if s.svr.config.MaxPacketDataSize > 0 && netPacket.DataSize > s.svr.config.MaxPacketDataSize {
		if s.svr.logger != nil {
			s.svr.logger.Error("Send WebSocket packet, Data size over max size, data size :%d, max size: %d, protoId:%d",
				netPacket.DataSize, s.svr.config.MaxPacketDataSize, protoId)
		}
		return fmt.Errorf("send WebSocket packet, Data size over max size, data size :%d, max size: %d, protoId:%d",
			netPacket.DataSize, s.svr.config.MaxPacketDataSize, protoId)
	}

	s.sendChan <- &netPacket
	return nil
}

func (s *WebSocketServerSession) send(netPacket *NetPacket) (int, error) {
	data := netPacket.Marshal()
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
