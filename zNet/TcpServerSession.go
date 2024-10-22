package zNet

import (
	"context"
	"errors"
	"fmt"
	"io"
	"net"
	"sync"
	"time"

	"github.com/pzqf/zUtil/zAes"
)

type TcpServerSession struct {
	conn          *net.TCPConn
	sid           SessionIdType
	sendChan      chan *NetPacket
	receiveChan   chan *NetPacket
	wg            sync.WaitGroup
	lastHeartBeat time.Time
	ctxCancel     context.CancelFunc
	onClose       TcpCloseCallBackFunc
	aesKey        []byte
	svr           *TcpServer
	obj           interface{}
}

type TcpCloseCallBackFunc func(c *TcpServerSession)

func NewTcpServerSession(svr *TcpServer, conn *net.TCPConn, sid SessionIdType, closeCallBack TcpCloseCallBackFunc, aesKey []byte) *TcpServerSession {
	newSession := TcpServerSession{
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

func (s *TcpServerSession) Start() {
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

		headBuf := make([]byte, NetPacketHeadSize)
		n, err := io.ReadFull(s.conn, headBuf)
		if err != nil {
			//LogPrint(fmt.Sprintf("Client conn read error, error:%v, sid:%d, closed", err, s.sid))
			break
		}

		if n != NetPacketHeadSize {
			//LogPrint(fmt.Sprintf("Client conn read error, head size %d, sid:%d, closed", n, s.sid))
			break
		}

		netPacket := NetPacket{}
		if err = netPacket.UnmarshalHead(headBuf); err != nil {
			//LogPrint("Receive NetPacket,Unmarshal head error", err, len(headBuf))
			break
		}

		if netPacket.DataSize > 0 {
			netPacket.Data = make([]byte, int(netPacket.DataSize))
			n, err = io.ReadFull(s.conn, netPacket.Data)
			if err != nil {
				//LogPrint(fmt.Sprintf("Client conn read data error,%v,  sid:%d, closed", err, s.sid))
				break
			}

			if netPacket.DataSize != int32(n) {
				//LogPrint(fmt.Sprintf("Receive NetPacket, Data size error,protoid:%d, DataSize:%d, received:%d",
				//	netPacket.ProtoId, netPacket.DataSize, n))
				break
			}
		}

		if s.svr.config.MaxPacketDataSize > 0 && netPacket.DataSize > s.svr.config.MaxPacketDataSize {
			//LogPrint(fmt.Sprintf("Receive NetPacket, Data size over max size, protoid:%d, data size:%d, max size: %d",
			//	netPacket.ProtoId, netPacket.DataSize, maxPacketDataSize))
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
			if s.svr.dispatcher != nil {
				//err := s.svr.workerPool.Submit(func() {
				//	err := s.svr.dispatcher(s, receivePacket)
				//	if err != nil {
				//		return
				//	}
				//})
				go func() {
					err := s.svr.dispatcher(s, receivePacket)
					if err != nil {
					}
				}()
			}

		case sendPacket := <-s.sendChan:
			_, err := s.send(sendPacket)
			if err != nil {
				//LogPrint(fmt.Sprintf("Send NetPacket error,%v, ProtoId:%d", err, sendPacket.ProtoId))
			}
		case <-ctx.Done():
			for {
				if len(s.receiveChan) > 0 {
					receivePacket := <-s.receiveChan
					if s.svr.dispatcher != nil && s.svr.workerPool != nil {
						err := s.svr.workerPool.Submit(func() {
							err := s.svr.dispatcher(s, receivePacket)
							if err != nil {
								return
							}
						})
						if err != nil {
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
						//LogPrint(err)
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
	if s.svr.config.MaxPacketDataSize > 0 && netPacket.DataSize > s.svr.config.MaxPacketDataSize {
		return errors.New(fmt.Sprintf("send NetPacket, Data size over max size, data size :%d, max size: %d, protoId:%d",
			netPacket.DataSize, s.svr.config.MaxPacketDataSize, protoId))
	}

	s.sendChan <- &netPacket
	return nil
}

func (s *TcpServerSession) send(netPacket *NetPacket) (int, error) {
	n, err := s.conn.Write(netPacket.Marshal())
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
	duration := time.Second * time.Duration(s.svr.config.HeartbeatDuration)
	breakDuration := time.Second * time.Duration(s.svr.config.HeartbeatDuration*2)
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

func (s *TcpServerSession) GetObj() interface{} {
	return s.obj
}

func (s *TcpServerSession) SetObj(obj interface{}) {
	s.obj = obj
}
