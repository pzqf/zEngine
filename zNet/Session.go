package zNet

import (
	"bufio"
	"bytes"
	"encoding/binary"
	"errors"
	"fmt"
	"log"
	"net"
	"sync"
	"time"
)

type Session struct {
	conn *net.TCPConn
	sid  int64 // session ID
	//exitChan      chan bool
	sendChan      chan *NetPacket
	receiveChan   chan *NetPacket
	wg            sync.WaitGroup
	lastHeartBeat time.Time
	tcpServer     *TcpServer
	running       bool
}

func (s *Session) Init(conn *net.TCPConn, sid int64, server *TcpServer) {
	s.conn = conn
	s.sid = sid
	//s.exitChan = make(chan bool, 1)
	s.sendChan = make(chan *NetPacket, 4096)
	s.receiveChan = make(chan *NetPacket, 4096)
	s.lastHeartBeat = time.Now()
	s.tcpServer = server
	s.running = false
}

func (s *Session) Start() {
	if s.conn == nil {
		return
	}
	s.running = true
	go s.receive()
	go s.process()
	go s.heartbeatCheck()
	return
}

func (s *Session) receive() {
	s.wg.Add(1)
	defer func() {
		if err := recover(); err != nil {
			log.Println("panic:", err)
		}
	}()
	headSize := 8
	reader := bufio.NewReader(s.conn)
	for {
		if !s.running {
			break
		}
		//read head
		var buff = make([]byte, headSize)
		n, err := reader.Read(buff)
		if err != nil {
			//log.Printf("Client conn read error,%v, sid:%d, closed", err, s.sid)
			s.running = false
			break
		}

		if n != headSize {
			log.Printf("Receive NetPacket head error,sid:%d, addr:%s", s.sid, s.conn.RemoteAddr().String())
			continue
		}

		netPacket := NetPacket{}
		if err = netPacket.UnmarshalHead(buff); err != nil {
			log.Println(err)
			continue
		}
		if netPacket.ProtoId <= 0 {
			log.Printf("receive NetPacket protoid empty, sid:%d", s.sid)
			continue
		}

		if netPacket.DataSize > MaxNetPacketDataSize {
			log.Printf("Receive NetPacket Data size over max size, sid:%d, protoid:%d, data size:%d, max size: %d",
				s.sid, netPacket.ProtoId, netPacket.DataSize, MaxNetPacketDataSize)
			continue
		}

		//read data
		if netPacket.DataSize > 0 {
			var dataBuf []byte
			readSize := 0
			readHappenError := false
			for {
				readBuf := make([]byte, netPacket.DataSize)
				n, err = reader.Read(readBuf)
				if err != nil {
					log.Printf("Client conn read data error,%v, sid:%d, addr:%s", err, s.sid, s.conn.RemoteAddr().String())
					readHappenError = true
					break
				}

				dataBuf = append(dataBuf, readBuf[:n]...)
				readSize += n
				if readSize >= int(netPacket.DataSize) {
					break
				}
			}
			if readHappenError {
				s.running = false
				break
			}

			if netPacket.DataSize != int32(len(dataBuf)) {
				log.Printf("receive NetPacket Data size error, sid:%d, protoid:%d, DataSize:%d:%d", s.sid, netPacket.ProtoId, netPacket.DataSize, len(dataBuf))
				continue
			}

			netPacket.Data = dataBuf
		}

		s.receiveChan <- &netPacket
		//_ = Dispatcher(s, &netPacket)

		s.heartbeatUpdate()
	}
	s.wg.Done()
}

func (s *Session) process() {
	s.wg.Add(1)
	defer func() {
		if err := recover(); err != nil {
			log.Println("panic:", err)
		}
	}()
	for {
		select {
		case receivePacket := <-s.receiveChan:
			if receivePacket.ProtoId == 0 {
				fmt.Println(receivePacket)
			}
			err := Dispatcher(s, receivePacket)
			if err != nil {
				log.Printf("Dispatcher NetPacket error,%v, ProtoId:%d", err, receivePacket.ProtoId)
			}
		case sendPacket := <-s.sendChan:
			_, err := s.send(sendPacket)
			if err != nil {
				log.Printf("Send NetPacket error,%v, ProtoId:%d", err, sendPacket.ProtoId)
			}
		case <-time.After(1 * time.Second):
			if !s.running {
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
			}
		}

		if !s.running {
			break
		}
	}
	s.wg.Done()
	_ = s.conn.Close()
	s.close()
}

func (s *Session) Send(netPacket *NetPacket) error {
	if netPacket == nil {
		return errors.New("send packet is nil")
	}

	if netPacket.ProtoId <= 0 && netPacket.DataSize < 0 {
		return errors.New("send packet illegal")
	}
	if netPacket.DataSize > MaxNetPacketDataSize {
		return errors.New(fmt.Sprintf("NetPacket Data size over max size, data size :%d, max size: %d", netPacket.DataSize, MaxNetPacketDataSize))
	}

	s.sendChan <- netPacket

	return nil
}

func (s *Session) send(netPacket *NetPacket) (int, error) {
	sendBuf := new(bytes.Buffer)
	_ = binary.Write(sendBuf, binary.LittleEndian, netPacket.ProtoId)
	_ = binary.Write(sendBuf, binary.LittleEndian, netPacket.DataSize)
	_ = binary.Write(sendBuf, binary.LittleEndian, netPacket.Data)

	n, err := s.conn.Write(sendBuf.Bytes())
	if err != nil {
		return 0, err
	}
	return n, nil
}

func (s *Session) Close() {
	s.running = false
}

func (s *Session) close() {
	s.wg.Wait()
	s.tcpServer.DelClient(s)
}

func (s *Session) GetSid() int64 {
	return s.sid
}

func (s *Session) heartbeatUpdate() {
	s.lastHeartBeat = time.Now()
}

func (s *Session) heartbeatCheck() {
	s.wg.Add(1)
	for {
		time.Sleep(1 * time.Second)

		select {
		case <-time.After(60 * time.Second):
			if time.Now().Sub(s.lastHeartBeat).Seconds() > 120 {
				s.running = false
				break
			}
		case <-time.After(1 * time.Second):
		}

		if !s.running {
			break
		}
	}
	s.wg.Done()
}
