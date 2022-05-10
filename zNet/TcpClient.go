package zNet

import (
	"bufio"
	"bytes"
	"encoding/binary"
	"errors"
	"fmt"
	"net"
	"strconv"

	"github.com/pzqf/zEngine/zLog"
)

type TcpClient struct {
	serverAddr string
	serverPort int
	conn       *net.TCPConn
}

func (cli *TcpClient) Connect(serverAddr string, serverPort int) error {
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

func (cli *TcpClient) Send(netPacket *NetPacket) error {
	sendBuf := new(bytes.Buffer)
	_ = binary.Write(sendBuf, binary.LittleEndian, netPacket.ProtoId)
	_ = binary.Write(sendBuf, binary.LittleEndian, netPacket.DataSize)
	_ = binary.Write(sendBuf, binary.LittleEndian, netPacket.Data)

	_, err := cli.conn.Write(sendBuf.Bytes())
	if err != nil {
		zLog.Error(err.Error())
		return err
	}
	return nil
}

func (cli *TcpClient) Receive() (*NetPacket, error) {
	reader := bufio.NewReader(cli.conn)
	headSize := 8

	//read head
	var buff = make([]byte, headSize)
	n, err := reader.Read(buff)
	if err != nil {
		return nil, err
	}

	if n != headSize {
		return nil, errors.New("receive NetPacket head error")
	}

	netPacket := &NetPacket{}
	if err = netPacket.UnmarshalHead(buff); err != nil {
		return nil, err
	}
	if netPacket.DataSize > MaxNetPacketDataSize {
		return nil, errors.New(fmt.Sprintf("Receive NetPacket length over max receive buf, data size :%d, max size: %d",
			netPacket.DataSize, MaxNetPacketDataSize))
	}

	//read data
	var dataBuf []byte
	readSize := 0
	readHappenError := false
	for {
		readBuf := make([]byte, netPacket.DataSize)
		n, err = reader.Read(readBuf)
		if err != nil {
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
		return nil, errors.New("read packet data error")
	}

	netPacket.Data = dataBuf
	return netPacket, nil
}
func (cli *TcpClient) Close() {
	_ = cli.conn.Close()
}
