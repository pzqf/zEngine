package NetServer

import (
	"bufio"
	"bytes"
	"encoding/binary"
	"net"
	"zEngine/zLog"
	//"time"
)

type Session struct {
	conn        *net.TCPConn
	uid         int64
	sid         int64 // sessionID
	exitChan    chan bool
	sendChan    chan *NetPacket
	receiveChan chan *NetPacket
}

func (s *Session) Init(conn *net.TCPConn, sid int64) {
	s.conn = conn
	s.sid = sid
	s.exitChan = make(chan bool, 1)
	s.sendChan = make(chan *NetPacket, 512)
	s.receiveChan = make(chan *NetPacket, 512)
}

func (s *Session) SetUid(uid int64) {
	s.uid = uid
}

func (s *Session) Start() {
	if s.conn == nil {
		return
	}
	zLog.InfoF("Session [%d] Started, begin read", s.sid)
	go func() {
		headSize := 8
		reader := bufio.NewReader(s.conn)
		for {
			//read head
			var buff = make([]byte, headSize)
			n, err := reader.Read(buff)
			if err != nil {
				zLog.ErrorF("Client conn read error,%v, sid:%d", err, s.sid)
				break
			}

			if n != headSize {
				zLog.ErrorF("Receive NetPacket head error,sid:%d, addr:%s", s.sid, s.conn.RemoteAddr().String())
				continue
			}

			netPacket := &NetPacket{}
			if err = netPacket.UnmarshalHead(buff); err != nil {
				zLog.Error(err.Error())
				break
			}
			if netPacket.DataSize > MaxNetPacketDataSize {
				zLog.ErrorF("Receive NetPacket Data size over max size, data size :%d, max size: %d",
					netPacket.DataSize, MaxNetPacketDataSize)
				break
			}

			//read data
			var dataBuf []byte
			readSize := 0
			readHappenError := false
			for {
				readBuf := make([]byte, netPacket.DataSize)
				n, err = reader.Read(readBuf)
				if err != nil {
					zLog.ErrorF("Client conn read error,%v, sid:%d, addr:%s", err, s.sid, s.conn.RemoteAddr().String())
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

			netPacket.Data = dataBuf
			zLog.InfoF("Receive NetPacket, sid:%d, uid:%d, ProtoId:%d, DataSize: %d",
				s.sid, s.uid, netPacket.ProtoId, netPacket.DataSize)
			s.receiveChan <- netPacket
		}
		s.Close()
	}()

	go s.processMsg()

	return
}

func (s *Session) processMsg() {
	running := true
	for {
		select {
		case receivePacket := <-s.receiveChan:
			err := Dispatcher(s.sid, receivePacket)
			if err != nil {
				zLog.ErrorF("Dispatcher NetPacket error,%v, ProtoId:%d", err, receivePacket.ProtoId)
			}
		case sendPacket := <-s.sendChan:
			_, _ = s.send(sendPacket)
		case <-s.exitChan:
			for {
				if len(s.receiveChan) > 0 {
					sendPacket := <-s.sendChan
					err := Dispatcher(s.sid, sendPacket)
					if err != nil {
						zLog.ErrorF("Dispatcher NetPacket error,%v, ProtoId:%d", err, sendPacket.ProtoId)
					}
					continue
				}
				break
			}
			close(s.receiveChan)

			for {
				if len(s.sendChan) > 0 {
					msg := <-s.sendChan
					_, _ = s.send(msg)
					continue
				}
				break
			}

			close(s.sendChan)

			_ = s.conn.Close()
			running = false
			break
		}

		if !running {
			break
		}
	}
}

func (s *Session) Send(protoId int32, data interface{}) error {
	netPacket := &NetPacket{}
	netPacket.ProtoId = protoId

	err := netPacket.EncodeData(data)
	if err != nil {
		return err
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

	zLog.InfoF("Send NetPacket, sid:%d, uid:%d, ProtoId:%d, DataSize: %d",
		s.sid, s.uid, netPacket.ProtoId, netPacket.DataSize)
	return n, nil
}

func (s *Session) Close() {
	zLog.InfoF("Close session %d ", s.sid)
	TcpServerInstance.DelClient(s)
	s.exitChan <- true
}
