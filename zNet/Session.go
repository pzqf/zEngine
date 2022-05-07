package zNet

import (
	"bufio"
	"bytes"
	"encoding/binary"
	"fmt"
	"log"
	"net"
	"sync"
)

type Session struct {
	conn        *net.TCPConn
	sid         int64 // session ID
	exitChan    chan bool
	sendChan    chan *NetPacket
	receiveChan chan *NetPacket
	wg          sync.WaitGroup
}

func (s *Session) Init(conn *net.TCPConn, sid int64) {
	s.conn = conn
	s.sid = sid
	s.exitChan = make(chan bool, 1)
	s.sendChan = make(chan *NetPacket, 4096)
	s.receiveChan = make(chan *NetPacket, 4096)
}

func (s *Session) Start() {
	if s.conn == nil {
		return
	}
	s.wg.Add(2)
	go s.receive()
	go s.process()
	return
}

func (s *Session) receive() {
	defer func() {
		if err := recover(); err != nil {
			log.Println("panic:", err)
		}
	}()
	headSize := 8
	reader := bufio.NewReader(s.conn)
	for {
		//read head
		var buff = make([]byte, headSize)
		n, err := reader.Read(buff)
		if err != nil {
			//log.Printf("Client conn read error,%v, sid:%d, closed", err, s.sid)
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
		if netPacket.DataSize <= 0 {
			log.Printf("Receive NetPacket Data size 0, sid:%d, protoid:%d", s.sid, netPacket.ProtoId)
			continue
		}
		if netPacket.DataSize > MaxNetPacketDataSize {
			log.Printf("Receive NetPacket Data size over max size, sid:%d, protoid:%d, data size:%d, max size: %d",
				s.sid, netPacket.ProtoId, netPacket.DataSize, MaxNetPacketDataSize)
			continue
		}

		//read data
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
			break
		}

		if netPacket.DataSize != int32(len(dataBuf)) {
			log.Printf("receive NetPacket Data, sid:%d, protoid:%d, DataSize:%d:%d", s.sid, netPacket.ProtoId, netPacket.DataSize, len(dataBuf))
			continue
		}

		netPacket.Data = dataBuf

		s.receiveChan <- &netPacket
	}
	s.wg.Done()
	s.exitChan <- true
}

func (s *Session) process() {
	defer func() {
		if err := recover(); err != nil {
			log.Println("panic:", err)
		}
	}()
	running := true
	for {
		select {
		case receivePacket := <-s.receiveChan:
			if receivePacket.ProtoId == 0 {
				fmt.Println(receivePacket)
			}
			err := Dispatcher(s.sid, receivePacket)
			if err != nil {
				log.Printf("Dispatcher NetPacket error,%v, ProtoId:%d", err, receivePacket.ProtoId)
			}
		case sendPacket := <-s.sendChan:
			_, err := s.send(sendPacket)
			if err != nil {
				log.Printf("Send NetPacket error,%v, ProtoId:%d", err, sendPacket.ProtoId)
			}
		case <-s.exitChan:
			for {
				if len(s.receiveChan) > 0 {
					receivePacket := <-s.receiveChan
					err := Dispatcher(s.sid, receivePacket)
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
					_, _ = s.send(sendPacket)
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

	s.wg.Done()
	_ = s.conn.Close()
	s.close()
}

func (s *Session) Send(protoId int32, netPacket *NetPacket) error {
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
	s.exitChan <- true
}

func (s *Session) close() {
	s.wg.Wait()
	TcpServerInstance.DelClient(s)
}
