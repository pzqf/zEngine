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

	"github.com/pzqf/zUtil/zAes"
)

type TcpClientSession struct {
	conn              *net.TCPConn
	wg                sync.WaitGroup
	lastHeartBeat     time.Time
	ctxCancel         context.CancelFunc
	aesKey            []byte
	heartbeatDuration int
}

func (s *TcpClientSession) Init(conn *net.TCPConn, aesKey []byte, heartbeatDuration int) {
	s.conn = conn
	s.lastHeartBeat = time.Now()
	s.aesKey = aesKey
	s.heartbeatDuration = heartbeatDuration
}

func (s *TcpClientSession) Start() {
	if s.conn == nil {
		return
	}
	ctx, ctxCancel := context.WithCancel(context.Background())
	s.ctxCancel = ctxCancel

	go s.receive(ctx)
	if s.heartbeatDuration > 0 {
		go s.heartbeatCheck(ctx)
	}

	return
}

func (s *TcpClientSession) Close() {
	s.ctxCancel()
	s.wg.Wait()
}

func (s *TcpClientSession) receive(ctx context.Context) {
	s.wg.Add(1)
	defer s.ctxCancel()
	defer s.wg.Done()
	defer Recover()
	for {
		if ctx.Err() != nil {
			break
		}

		_ = s.conn.SetReadDeadline(time.Now().Add(time.Second * 3))

		headBuf := make([]byte, NetPacketHeadSize)
		n, err := io.ReadFull(s.conn, headBuf)
		if err != nil {
			if netErr, ok := err.(net.Error); ok {
				if netErr.Timeout() {
					continue
				}
			}
			LogPrint(fmt.Sprintf("Client conn read error, error:%v, closed", err))
			break
		}

		if n != NetPacketHeadSize {
			LogPrint(fmt.Sprintf("Client conn read error, error:head size error %d, closed", n))
			break
		}

		netPacket := NetPacket{}
		if err = netPacket.UnmarshalHead(headBuf); err != nil {
			LogPrint("Receive NetPacket,Unmarshal head error", err, len(headBuf))
			break
		}

		if netPacket.DataSize > 0 {
			netPacket.Data = make([]byte, int(netPacket.DataSize))
			n, err = io.ReadFull(s.conn, netPacket.Data)
			if err != nil {
				LogPrint(fmt.Sprintf("Client conn read data error,%v,  closed", err))
				break
			}

			if netPacket.DataSize != int32(n) {
				LogPrint(fmt.Sprintf("Receive NetPacket, Data size error,protoid:%d, DataSize:%d, received:%d",
					netPacket.ProtoId, netPacket.DataSize, n))
				break
			}
		}

		if netPacket.ProtoId < 0 {
			log.Printf("receive NetPacket protoid empty")
			continue
		}

		if netPacket.DataSize > maxPacketDataSize {
			LogPrint(fmt.Sprintf("Receive NetPacket, Data size over max size, protoid:%d, data size:%d, max size: %d",
				netPacket.ProtoId, netPacket.DataSize, maxPacketDataSize))
			continue
		}

		if netPacket.DataSize > 0 && s.aesKey != nil {
			netPacket.Data = zAes.DecryptCBC(netPacket.Data, s.aesKey)
		}

		err = Dispatcher(s, &netPacket)
		if err != nil {
			LogPrint(fmt.Sprintf("Dispatcher NetPacket error,%v, ProtoId:%d", err, netPacket.ProtoId))
		}
	}
	s.ctxCancel()
}

func (s *TcpClientSession) Send(protoId int32, data []byte) error {
	netPacket := NetPacket{
		ProtoId: protoId,
	}

	if data != nil {
		if s.aesKey != nil {
			netPacket.Data = zAes.EncryptCBC(data, s.aesKey)
		} else {
			netPacket.Data = data
		}
	}

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
	s.heartbeatUpdate()
	return nil
}

func (s *TcpClientSession) heartbeatUpdate() {
	s.lastHeartBeat = time.Now()
}

func (s *TcpClientSession) heartbeatCheck(ctx context.Context) {
	s.wg.Add(1)
	defer s.wg.Done()
	hbd := float64(s.heartbeatDuration)
	for {
		select {
		case <-time.After(30 * time.Second):
			if time.Now().Sub(s.lastHeartBeat).Seconds() >= hbd {
				_ = s.Send(HeartbeatProtoId, nil)
			}
		case <-ctx.Done():
			return
		}
	}
}
func (s *TcpClientSession) GetSid() SessionIdType {
	return SessionIdType(1)
}
