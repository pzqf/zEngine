package zNet

import (
	"context"
	"errors"
	"fmt"
	"net"
	"sync"
	"time"

	"github.com/gorilla/websocket"
)

type WebSocketServerSession struct {
	conn          *websocket.Conn
	sid           SessionIdType // session ID
	sendChan      chan *NetPacket
	receiveChan   chan *NetPacket
	wg            sync.WaitGroup
	lastHeartBeat time.Time
	ctxCancel     context.CancelFunc
	onClose       WebsocketCloseCallBackFunc
	config        *WebSocketConfig
}

type WebsocketCloseCallBackFunc func(c *WebSocketServerSession)

func NewWebSocketServerSession(cfg *WebSocketConfig, conn *websocket.Conn, sid SessionIdType, onClose WebsocketCloseCallBackFunc) *WebSocketServerSession {
	newSession := &WebSocketServerSession{
		conn:          conn,
		sid:           sid,
		sendChan:      make(chan *NetPacket, cfg.ChanSize),
		receiveChan:   make(chan *NetPacket, cfg.ChanSize),
		lastHeartBeat: time.Now(),
		onClose:       onClose,
		config:        cfg,
	}

	return newSession
}

func (s *WebSocketServerSession) Send(protoId int32, data []byte) error {
	netPacket := NetPacket{
		ProtoId: protoId,
	}

	netPacket.Data = data

	netPacket.DataSize = int32(len(netPacket.Data))
	if netPacket.ProtoId <= 0 && netPacket.DataSize < 0 {
		return errors.New("send packet illegal")
	}
	if netPacket.DataSize > maxPacketDataSize {
		return errors.New(fmt.Sprintf("send NetPacket, Data size over max size, data size :%d, max size: %d, protoId:%d",
			netPacket.DataSize, maxPacketDataSize, protoId))
	}

	s.sendChan <- &netPacket
	return nil
}

func (s *WebSocketServerSession) GetSid() SessionIdType {
	return s.sid
}

func (s *WebSocketServerSession) Start() {
	if s.conn == nil {
		return
	}
	ctx, ctxCancel := context.WithCancel(context.Background())
	s.ctxCancel = ctxCancel

	go s.receive(ctx)
	go s.process(ctx)

	return
}

func (s *WebSocketServerSession) Close() {
	s.ctxCancel()
}

func (s *WebSocketServerSession) receive(ctx context.Context) {
	s.wg.Add(1)
	defer s.ctxCancel()
	defer s.wg.Done()
	defer Recover()

	for {
		if ctx.Err() != nil {
			break
		}

		//_ = s.conn.SetReadDeadline(time.Now().Add(time.Millisecond * time.Duration(5000)))
		_, msg, err := s.conn.ReadMessage()
		if err != nil {
			if netErr, ok := err.(net.Error); ok {
				if netErr.Timeout() {
					continue
				}
			}

			LogPrint(fmt.Sprintf("ReadMessage from remote:%v error: %v \n", s.conn.RemoteAddr(), err))

			break
		}
		//fmt.Println("收到消息：", string(msg))

		if len(msg) < NetPacketHeadSize {
			LogPrint(fmt.Sprintf("Client conn read error, head size %d, sid:%d, closed", len(msg), s.sid))
			continue
		}

		headBuf := msg[:NetPacketHeadSize]

		netPacket := NetPacket{}
		if err = netPacket.UnmarshalHead(headBuf); err != nil {
			LogPrint("Receive NetPacket,Unmarshal head error", err, len(headBuf))
			break
		}

		if netPacket.DataSize > 0 {
			netPacket.Data = msg[NetPacketHeadSize:]

			if netPacket.DataSize != int32(len(netPacket.Data)) {
				LogPrint(fmt.Sprintf("Receive NetPacket, Data size error,protoid:%d, DataSize:%d, received:%d",
					netPacket.ProtoId, netPacket.DataSize, len(netPacket.Data)))
				break
			}
		}

		if netPacket.ProtoId < 0 {
			LogPrint("receive NetPacket ProtoId less than 0")
			continue
		}

		if netPacket.DataSize > maxPacketDataSize {
			LogPrint(fmt.Sprintf("Receive NetPacket, Data size over max size, protoid:%d, data size:%d, max size: %d",
				netPacket.ProtoId, netPacket.DataSize, maxPacketDataSize))
			continue
		}

		if netPacket.ProtoId != HeartbeatProtoId {
			s.receiveChan <- &netPacket
		}

	}
	s.ctxCancel()
}

func (s *WebSocketServerSession) process(ctx context.Context) {
	s.wg.Add(1)
	defer s.wg.Done()
	defer Recover()
	running := true
	for {
		select {
		case receivePacket := <-s.receiveChan:
			err := Dispatcher(s, receivePacket)
			if err != nil {
				LogPrint(fmt.Sprintf("Dispatcher NetPacket error,%v, ProtoId:%d", err, receivePacket.ProtoId))
			}
		case sendPacket := <-s.sendChan:
			_, err := s.send(sendPacket)
			if err != nil {
				LogPrint(fmt.Sprintf("Send NetPacket error,%v, ProtoId:%d", err, sendPacket.ProtoId))
			}
		case <-ctx.Done():
			for {
				if len(s.receiveChan) > 0 {
					receivePacket := <-s.receiveChan
					err := Dispatcher(s, receivePacket)
					if err != nil {
						LogPrint(fmt.Sprintf("Dispatcher NetPacket error,%v, ProtoId:%d", err, receivePacket.ProtoId))
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
						LogPrint(err)
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

	_ = s.conn.Close()
	if s.onClose != nil {
		s.onClose(s)
	}
}

func (s *WebSocketServerSession) send(netPacket *NetPacket) (int, error) {
	err := s.conn.WriteMessage(1, netPacket.Marshal())
	if err != nil {
		return 0, err
	}
	return 0, nil
}
