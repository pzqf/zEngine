package zNet

import (
	"bufio"
	"bytes"
	"context"
	"encoding/binary"
	"errors"
	"fmt"
	"log"
	"net"
	"sync"
	"time"
)

type SessionIdType int64

type Session struct {
	conn          *net.TCPConn
	sid           SessionIdType // session ID
	sendChan      chan *NetPacket
	receiveChan   chan *NetPacket
	wg            sync.WaitGroup
	lastHeartBeat time.Time
	ctxCancel     context.CancelFunc
	onClose       CloseCallBackFunc
}

type CloseCallBackFunc func(c *Session)

func (s *Session) Init(conn *net.TCPConn, sid SessionIdType, closeCallBack CloseCallBackFunc) {
	s.conn = conn
	s.sid = sid
	s.sendChan = make(chan *NetPacket, 4096)
	s.receiveChan = make(chan *NetPacket, 4096)
	s.lastHeartBeat = time.Now()
	s.onClose = closeCallBack
}

func (s *Session) Start() {
	if s.conn == nil {
		return
	}
	ctx, ctxCancel := context.WithCancel(context.Background())
	s.ctxCancel = ctxCancel
	s.wg.Add(2)
	go s.receive(ctx)
	go s.process(ctx)
	//go s.heartbeatCheck(s.ctx)
	return
}

func (s *Session) Close() {
	s.ctxCancel()
	s.wg.Wait()
}

func (s *Session) receive(ctx context.Context) {
	defer s.ctxCancel()
	defer s.wg.Done()
	defer func() {
		if err := recover(); err != nil {
			log.Println("panic:", err)
		}
	}()

	reader := bufio.NewReader(s.conn)
endOfReceiveErr:
	for {
		if ctx.Err() != nil {
			break
		}

		//read head
		var headBuf []byte
		needReadSize := 8
		for {
			readBuf := make([]byte, needReadSize)
			n, err := reader.Read(readBuf)
			if err != nil {
				log.Printf("Client conn read error,%v, sid:%d, closed", err, s.sid)
				break endOfReceiveErr
			}
			headBuf = append(headBuf, readBuf[:n]...)
			needReadSize -= n
			if needReadSize <= 0 {
				break
			}
		}

		netPacket := NetPacket{}
		if err := netPacket.UnmarshalHead(headBuf); err != nil {
			log.Println("Receive NetPacket,Unmarshal head error", err, len(headBuf))
			break
		}

		if netPacket.DataSize > maxPacketDataSize {
			log.Printf("Receive NetPacket, head DataSize over max size, protoid:%d, DataSize:%d", netPacket.ProtoId, netPacket.DataSize)
			break
		}

		//read data
		if netPacket.DataSize > 0 {
			var dataBuf []byte
			needReadSize = int(netPacket.DataSize)
			for {
				readBuf := make([]byte, needReadSize)
				n, err := reader.Read(readBuf)
				if err != nil {
					log.Printf("Client conn read data error,%v, addr:%s", err, s.conn.RemoteAddr().String())
					break endOfReceiveErr
				}

				dataBuf = append(dataBuf, readBuf[:n]...)
				needReadSize -= n
				if needReadSize <= 0 {
					break
				}
			}

			if netPacket.DataSize != int32(len(dataBuf)) {
				log.Printf("Receive NetPacket, Data size error,protoid:%d, DataSize:%d, received:%d",
					netPacket.ProtoId, netPacket.DataSize, len(dataBuf))
				continue
			}

			netPacket.Data = dataBuf
		}

		if netPacket.ProtoId < 0 {
			log.Printf("receive NetPacket protoid empty")
			continue
		}

		if netPacket.DataSize > maxPacketDataSize {
			log.Printf("Receive NetPacket, Data size over max size, protoid:%d, data size:%d, max size: %d",
				netPacket.ProtoId, netPacket.DataSize, maxPacketDataSize)
			continue
		}

		s.receiveChan <- &netPacket

		s.heartbeatUpdate()
	}
	s.ctxCancel()
}

func (s *Session) process(ctx context.Context) {
	//s.wg.Add(1)
	defer s.wg.Done()
	defer func() {
		if err := recover(); err != nil {
			log.Println("panic:", err)
		}
	}()
	running := true
	for {
		select {
		case receivePacket := <-s.receiveChan:
			err := Dispatcher(s, receivePacket)
			if err != nil {
				log.Printf("Dispatcher NetPacket error,%v, ProtoId:%d", err, receivePacket.ProtoId)
			}
		case sendPacket := <-s.sendChan:
			_, err := s.send(sendPacket)
			if err != nil {
				log.Printf("Send NetPacket error,%v, ProtoId:%d", err, sendPacket.ProtoId)
			}
		case <-ctx.Done():
			for {
				if len(s.receiveChan) > 0 {
					receivePacket := <-s.receiveChan
					err := Dispatcher(s, receivePacket)
					if err != nil {
						log.Printf("Dispatcher NetPacket error,%v, ProtoId:%d", err, receivePacket.ProtoId)
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

func (s *Session) Send(protoId int32, data interface{}) error {
	netPacket := NetPacket{
		ProtoId: protoId,
	}

	err := netPacket.EncodeData(data)
	if err != nil {
		return err
	}
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

func (s *Session) send(netPacket *NetPacket) (int, error) {
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

func (s *Session) heartbeatUpdate() {
	s.lastHeartBeat = time.Now()
}

func (s *Session) heartbeatCheck(ctx context.Context) {
	s.wg.Add(1)
	defer s.wg.Done()
	for {
		select {
		case <-time.After(60 * time.Second):
			if time.Now().Sub(s.lastHeartBeat).Seconds() > 120 {
				s.ctxCancel()
				break
			}
		case <-ctx.Done():
			return
		}
	}
}

func (s *Session) GetSid() SessionIdType {
	return s.sid
}
