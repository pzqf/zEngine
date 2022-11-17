package zNet

import (
	"context"
	"errors"
	"fmt"
	"net"
	"sync"
)

type UdpClientSession struct {
	conn      *net.UDPConn
	wg        sync.WaitGroup
	ctxCancel context.CancelFunc
	//aesKey            []byte
	heartbeatDuration int
	dispatcher        DispatcherFunc
}

func (s *UdpClientSession) Init(conn *net.UDPConn, aesKey []byte, dispatcher DispatcherFunc) {
	s.conn = conn
	//s.aesKey = aesKey
	s.dispatcher = dispatcher
}

func (s *UdpClientSession) Start() {
	if s.conn == nil {
		return
	}
	ctx, ctxCancel := context.WithCancel(context.Background())
	s.ctxCancel = ctxCancel

	go s.receive(ctx)

	return
}

func (s *UdpClientSession) Close() {
	s.ctxCancel()
	s.wg.Wait()
}

func (s *UdpClientSession) receive(ctx context.Context) {
	s.wg.Add(1)
	defer s.ctxCancel()
	defer s.wg.Done()
	defer Recover()
	for {
		if ctx.Err() != nil {
			break
		}

		data := make([]byte, DefaultPacketDataSize)
		_, _, err := s.conn.ReadFromUDP(data)
		if err != nil {
			//LogPrint(err)
			break
		}

		netPacket := NetPacket{}
		if err = netPacket.UnmarshalHead(data[:NetPacketHeadSize]); err != nil {
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

		err = s.dispatcher(s, &netPacket)
		if err != nil {
			LogPrint(fmt.Sprintf("Dispatcher NetPacket error,%v, ProtoId:%d", err, netPacket.ProtoId))
		}
	}
	s.ctxCancel()
}

func (s *UdpClientSession) Send(protoId int32, data []byte) error {
	netPacket := NetPacket{
		ProtoId: protoId,
		Version: 0,
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

	_, err := s.conn.Write(netPacket.Marshal())
	if err != nil {
		return err
	}
	return nil
}

func (s *UdpClientSession) GetSid() SessionIdType {
	return SessionIdType(1)
}
