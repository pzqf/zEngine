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

//type SessionIdType int64

type UdpServerSession struct {
	conn          *net.UDPConn
	sid           SessionIdType // session ID
	addr          *net.UDPAddr
	sendChan      chan *NetPacket
	wg            sync.WaitGroup
	lastHeartBeat time.Time
	ctxCancel     context.CancelFunc
	aesKey        []byte
	config        *UdpConfig
	dispatcher    DispatcherFunc
}

func NewUdpServerSession(cfg *UdpConfig, conn *net.UDPConn, addr *net.UDPAddr, sid SessionIdType, dispatcher DispatcherFunc) *UdpServerSession {
	newSession := UdpServerSession{
		conn:          conn,
		addr:          addr,
		sid:           sid,
		sendChan:      make(chan *NetPacket, cfg.ChanSize),
		lastHeartBeat: time.Now(),
		dispatcher:    dispatcher,
	}
	return &newSession
}

func (s *UdpServerSession) Start() {
	if s.conn == nil {
		return
	}
	ctx, ctxCancel := context.WithCancel(context.Background())
	s.ctxCancel = ctxCancel

	go s.process(ctx)

	return
}

func (s *UdpServerSession) Close() {
	s.ctxCancel()
	s.wg.Wait()
}

func (s *UdpServerSession) Receive(data []byte) {
	netPacket := NetPacket{}
	if err := netPacket.UnmarshalHead(data[:NetPacketHeadSize]); err != nil {
		LogPrint("Receive NetPacket,Unmarshal head error", err, len(data))
		return
	}

	if netPacket.DataSize > 0 {
		netPacket.Data = data[NetPacketHeadSize : NetPacketHeadSize+int(netPacket.DataSize)]
	}

	if netPacket.ProtoId < 0 {
		LogPrint("receive NetPacket ProtoId less than 0")
		return
	}

	if netPacket.DataSize > maxPacketDataSize {
		LogPrint(fmt.Sprintf("Receive NetPacket, Data size over max size, protoid:%d, data size:%d, max size: %d",
			netPacket.ProtoId, netPacket.DataSize, maxPacketDataSize))
		return
	}

	if netPacket.ProtoId == HeartbeatProtoId {
		return
	}

	if netPacket.DataSize > 0 && s.aesKey != nil {
		netPacket.Data = zAes.DecryptCBC(netPacket.Data, s.aesKey)
	}
	err := s.dispatcher(s, &netPacket)
	if err != nil {
		LogPrint(fmt.Sprintf("Dispatcher NetPacket error,%v, ProtoId:%d", err, netPacket.ProtoId))
	}
}

func (s *UdpServerSession) process(ctx context.Context) {
	s.wg.Add(1)
	defer s.wg.Done()
	defer Recover()
	running := true
	for {
		select {
		case sendPacket := <-s.sendChan:
			_, err := s.conn.WriteToUDP(sendPacket.Marshal(), s.addr)
			if err != nil {
				LogPrint(fmt.Sprintf("Send NetPacket error,%v, ProtoId:%d", err, sendPacket.ProtoId))
			}
		case <-ctx.Done():
			for {
				if len(s.sendChan) > 0 {
					sendPacket := <-s.sendChan

					_, err := s.conn.WriteToUDP(sendPacket.Marshal(), s.addr)

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
	if netPacket.ProtoId <= 0 && netPacket.DataSize < 0 {
		return errors.New("send packet illegal")
	}
	if netPacket.DataSize > int32(maxPacketDataSize) {
		return errors.New(fmt.Sprintf("send NetPacket, Data size over max size, data size :%d, max size: %d, protoId:%d",
			netPacket.DataSize, maxPacketDataSize, protoId))
	}

	s.sendChan <- &netPacket
	return nil
}

func (s *UdpServerSession) GetSid() SessionIdType {
	return s.sid
}
