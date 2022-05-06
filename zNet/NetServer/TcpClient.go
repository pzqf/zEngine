package NetServer

import (
	"bufio"
	"bytes"
	"encoding/binary"
	"net"
	"strconv"
	"zEngine/zLog"
)

type NetClient struct {
	serverAddr string
	serverPort int
	conn       *net.TCPConn
}

func (cli *NetClient) Connect(serverAddr string, serverPort int) error {
	cli.serverAddr = serverAddr
	cli.serverPort = serverPort

	var tcpAddr *net.TCPAddr
	tcpAddr, _ = net.ResolveTCPAddr("tcp", cli.serverAddr+":"+strconv.Itoa(cli.serverPort))

	conn, err := net.DialTCP("tcp", nil, tcpAddr)
	if err != nil {
		//zLog.Error("Client connect error ! " + err.Error())
		return err
	}
	cli.conn = conn
	return nil
}

func (cli *NetClient) Send(protoId int32, data interface{}) error {
	netPacket := NetPacket{
		ProtoId: protoId,
	}
	err := netPacket.EncodeData(data)
	if err != nil {
		return err
	}
	sendBuf := new(bytes.Buffer)
	_ = binary.Write(sendBuf, binary.LittleEndian, netPacket.ProtoId)
	_ = binary.Write(sendBuf, binary.LittleEndian, netPacket.DataSize)
	_ = binary.Write(sendBuf, binary.LittleEndian, netPacket.Data)

	_, err = cli.conn.Write(sendBuf.Bytes())
	if err != nil {
		zLog.Error(err.Error())
		return err
	}
	//zLog.InfoF("Send NetPacket, ProtoId:%d, DataSize: %d", netPacket.ProtoId, netPacket.DataSize)

	return nil
}

func (cli *NetClient) Receive() *NetPacket {
	reader := bufio.NewReader(cli.conn)
	headSize := 8

	//read head
	var buff = make([]byte, headSize)
	n, err := reader.Read(buff)
	if err != nil {
		zLog.Error(err.Error())
		return nil
	}

	if n != headSize {
		zLog.ErrorF("Receive NetPacket head error, addr:%s", cli.conn.RemoteAddr().String())
		return nil
	}

	netPacket := &NetPacket{}
	if err = netPacket.UnmarshalHead(buff); err != nil {
		zLog.Error(err.Error())
		return nil
	}
	if netPacket.DataSize > MaxNetPacketDataSize {
		zLog.ErrorF("Receive NetPacket length over max receive buf, data size :%d, max size: %d",
			netPacket.DataSize, MaxNetPacketDataSize)
		return nil
	}

	//read data
	var dataBuf []byte
	readSize := 0
	readHappenError := false
	for {
		readBuf := make([]byte, netPacket.DataSize)
		n, err = reader.Read(readBuf)
		if err != nil {
			zLog.ErrorF("net read error,%v, addr:%s", err, cli.conn.RemoteAddr().String())
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
		return nil
	}

	netPacket.Data = dataBuf
	//zLog.InfoF("Receive NetPacket, ProtoId:%d, DataSize:%d", netPacket.ProtoId, netPacket.DataSize)
	return netPacket
}
func (cli *NetClient) Close() {
	cli.conn.Close()
}
