package zNet

import (
	"context"
	"errors"
	"fmt"
	"net"
	"sync"
	"time"

	"github.com/pzqf/zUtil/zAes"
)

type UdpServerSession struct {
	addr          *net.UDPAddr
	sid           SessionIdType
	sendChan      chan *NetPacket
	receiveChan   chan *NetPacket
	wg            sync.WaitGroup
	lastHeartBeat time.Time
	ctxCancel     context.CancelFunc
	onClose       UdpCloseCallBackFunc
	aesKey        []byte
	svr           *UdpServer
	obj           interface{}
}

type UdpCloseCallBackFunc func(c *UdpServerSession)

func NewUdpServerSession(svr *UdpServer, addr *net.UDPAddr, sid SessionIdType, closeCallBack UdpCloseCallBackFunc) *UdpServerSession {
	newSession := UdpServerSession{
		addr:          addr,
		sid:           sid,
		sendChan:      make(chan *NetPacket, svr.config.ChanSize),
		receiveChan:   make(chan *NetPacket, svr.config.ChanSize),
		lastHeartBeat: time.Now(),
		onClose:       closeCallBack,
		svr:           svr,
	}
	return &newSession
}

func (s *UdpServerSession) Start() {
	if s.addr == nil {
		return
	}
	ctx, ctxCancel := context.WithCancel(context.Background())
	s.ctxCancel = ctxCancel

	go s.process(ctx)
	if s.svr.config.HeartbeatDuration > 0 {
		go s.heartbeatCheck(ctx)
	}
	return
}

func (s *UdpServerSession) Close() {
	s.ctxCancel()
	s.wg.Wait()
}

func (s *UdpServerSession) handlePacket(data []byte) {
	// 解析数据包
	if len(data) < NetPacketHeadSize {
		if s.svr.logger != nil {
			s.svr.logger.Error("Received UDP packet too small: %d bytes", len(data))
		}
		return
	}

	netPacket := NetPacket{}
	if err := netPacket.UnmarshalHead(data[:NetPacketHeadSize]); err != nil {
		if s.svr.logger != nil {
			s.svr.logger.Error("Unmarshal UDP packet head error: %v", err)
		}
		return
	}

	if netPacket.DataSize > 0 {
		dataSize := int(netPacket.DataSize)
		if len(data) < NetPacketHeadSize+dataSize {
			if s.svr.logger != nil {
				s.svr.logger.Error("UDP packet data size mismatch: expected %d, got %d", dataSize, len(data)-NetPacketHeadSize)
			}
			return
		}
		netPacket.Data = data[NetPacketHeadSize : NetPacketHeadSize+dataSize]
	}

	if s.svr.config.MaxPacketDataSize > 0 && netPacket.DataSize > s.svr.config.MaxPacketDataSize {
		if s.svr.logger != nil {
			s.svr.logger.Warn("UDP packet data size over max size: %d, max: %d", netPacket.DataSize, s.svr.config.MaxPacketDataSize)
		}
		return
	}

	if netPacket.ProtoId != HeartbeatProtoId {
		s.receiveChan <- &netPacket
	}

	s.heartbeatUpdate()
}

func (s *UdpServerSession) process(ctx context.Context) {
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
				receivePacket.Data = zAes.DecryptCBC(receivePacket.Data, s.aesKey)
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
					s.svr.logger.Error("Send UDP packet error:%v, ProtoId:%d", err, sendPacket.ProtoId)
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
							s.svr.logger.Error("Send UDP packet error:%v, ProtoId:%d", err, sendPacket.ProtoId)
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

	if s.onClose != nil {
		s.onClose(s)
	}
}

func (s *UdpServerSession) Send(protoId int32, data []byte) error {
	netPacket := NetPacket{
		ProtoId: protoId,
	}
	if s.aesKey != nil {
		netPacket.Data = zAes.EncryptCBC(data, s.aesKey)
	} else {
		netPacket.Data = data
	}
	netPacket.DataSize = int32(len(netPacket.Data))
	if netPacket.ProtoId <= 0 || netPacket.DataSize < 0 {
		if s.svr.logger != nil {
			s.svr.logger.Error("Send UDP packet illegal: protoId=%d, dataSize=%d", protoId, netPacket.DataSize)
		}
		return errors.New("send UDP packet illegal")
	}
	if s.svr.config.MaxPacketDataSize > 0 && netPacket.DataSize > s.svr.config.MaxPacketDataSize {
		if s.svr.logger != nil {
			s.svr.logger.Error("Send UDP packet, Data size over max size, data size :%d, max size: %d, protoId:%d",
				netPacket.DataSize, s.svr.config.MaxPacketDataSize, protoId)
		}
		return fmt.Errorf("send UDP packet, Data size over max size, data size :%d, max size: %d, protoId:%d",
			netPacket.DataSize, s.svr.config.MaxPacketDataSize, protoId)
	}

	s.sendChan <- &netPacket
	return nil
}

func (s *UdpServerSession) send(netPacket *NetPacket) (int, error) {
	data := netPacket.Marshal()
	n, err := s.svr.listener.WriteToUDP(data, s.addr)
	if err != nil {
		if s.svr.logger != nil {
			s.svr.logger.Error("Failed to write to UDP: %v, ProtoId: %d", err, netPacket.ProtoId)
		}
		return 0, err
	}
	return n, nil
}

func (s *UdpServerSession) heartbeatUpdate() {
	s.lastHeartBeat = time.Now()
}

func (s *UdpServerSession) heartbeatCheck(ctx context.Context) {
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

func (s *UdpServerSession) GetSid() SessionIdType {
	return s.sid
}

func (s *UdpServerSession) GetObj() interface{} {
	return s.obj
}

func (s *UdpServerSession) SetObj(obj interface{}) {
	s.obj = obj
}

func (s *UdpServerSession) GetAddr() *net.UDPAddr {
	return s.addr
}
