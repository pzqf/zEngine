package zNet

import (
	"bytes"
	"context"
	"encoding/binary"
	"errors"
	"fmt"
	"io"
	"net"
	"sync"
	"time"

	"github.com/pzqf/zUtil/zAes"
)

type SessionIdType int64

type TcpServerSession struct {
	conn          *net.TCPConn
	sid           SessionIdType // session ID
	sendChan      chan *NetPacket
	receiveChan   chan *NetPacket
	wg            sync.WaitGroup
	lastHeartBeat time.Time
	ctxCancel     context.CancelFunc
	onClose       CloseCallBackFunc
	aesKey        []byte
}

type CloseCallBackFunc func(c *TcpServerSession)

func (s *TcpServerSession) Init(conn *net.TCPConn, sid SessionIdType, closeCallBack CloseCallBackFunc, aesKey []byte) {
	s.conn = conn
	s.sid = sid
	s.sendChan = make(chan *NetPacket, GConfig.ChanSize)
	s.receiveChan = make(chan *NetPacket, GConfig.ChanSize)
	s.lastHeartBeat = time.Now()
	s.onClose = closeCallBack
	s.aesKey = aesKey
}

func (s *TcpServerSession) Start() {
	if s.conn == nil {
		return
	}
	ctx, ctxCancel := context.WithCancel(context.Background())
	s.ctxCancel = ctxCancel

	go s.receive(ctx)
	go s.process(ctx)
	if GConfig.HeartbeatDuration > 0 {
		go s.heartbeatCheck(ctx)
	}
	return
}

func (s *TcpServerSession) Close() {
	s.ctxCancel()
	s.wg.Wait()
}

func (s *TcpServerSession) receive(ctx context.Context) {
	s.wg.Add(1)
	defer s.ctxCancel()
	defer s.wg.Done()
	defer Recover()

	for {
		if ctx.Err() != nil {
			break
		}

		headSize := 8
		headBuf := make([]byte, headSize)
		n, err := io.ReadFull(s.conn, headBuf)
		if err != nil {
			if err != io.EOF {
				//log.Printf("Client conn read error, error:%v, sid:%d, closed", err, s.sid)
			} else {
				//log.Printf("Socket closed, error:%v, sid:%d, closed", err, s.sid)
			}
			LogPrint(fmt.Sprintf("Client conn read error, error:%v, sid:%d, closed", err, s.sid))
			break
		}

		if n != headSize {
			LogPrint(fmt.Sprintf("Client conn read error, head size %d, sid:%d, closed", n, s.sid))
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
				LogPrint(fmt.Sprintf("Client conn read data error,%v,  sid:%d, closed", err, s.sid))
				break
			}

			if netPacket.DataSize != int32(n) {
				LogPrint(fmt.Sprintf("Receive NetPacket, Data size error,protoid:%d, DataSize:%d, received:%d",
					netPacket.ProtoId, netPacket.DataSize, n))
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

		s.heartbeatUpdate()
	}
	s.ctxCancel()
}

func (s *TcpServerSession) process(ctx context.Context) {
	s.wg.Add(1)
	defer s.wg.Done()
	defer Recover()
	running := true
	for {
		select {
		case receivePacket := <-s.receiveChan:
			if receivePacket.DataSize > 0 && s.aesKey != nil {
				receivePacket.Data = zAes.DecryptCBC(receivePacket.Data, s.aesKey)
			}
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

func (s *TcpServerSession) Send(protoId int32, data []byte) error {
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
	if netPacket.DataSize > maxPacketDataSize {
		return errors.New(fmt.Sprintf("send NetPacket, Data size over max size, data size :%d, max size: %d, protoId:%d",
			netPacket.DataSize, maxPacketDataSize, protoId))
	}

	s.sendChan <- &netPacket
	return nil
}

func (s *TcpServerSession) send(netPacket *NetPacket) (int, error) {
	sendBuf := new(bytes.Buffer)
	_ = binary.Write(sendBuf, binary.LittleEndian, netPacket.ProtoId)
	_ = binary.Write(sendBuf, binary.LittleEndian, netPacket.DataSize)
	if netPacket.Data != nil {
		_ = binary.Write(sendBuf, binary.LittleEndian, netPacket.Data)
	}

	n, err := s.conn.Write(sendBuf.Bytes())
	if err != nil {
		return 0, err
	}
	return n, nil
}

func (s *TcpServerSession) heartbeatUpdate() {
	s.lastHeartBeat = time.Now()
}

func (s *TcpServerSession) heartbeatCheck(ctx context.Context) {
	s.wg.Add(1)
	defer s.wg.Done()
	duration := time.Second * time.Duration(GConfig.HeartbeatDuration)
	breakDuration := time.Second * time.Duration(GConfig.HeartbeatDuration*2)
	for {
		select {
		case <-time.After(duration):
			if time.Now().Sub(s.lastHeartBeat) > breakDuration {
				s.ctxCancel()
				break
			}
		case <-ctx.Done():
			return
		}
	}
}

func (s *TcpServerSession) GetSid() SessionIdType {
	return s.sid
}
