package zNet

import (
	"net"
	"strconv"
)

type TcpClient struct {
	serverAddr string
	serverPort int
	session    *Session
}

func (cli *TcpClient) ConnectToServer(serverAddr string, serverPort int) error {
	cli.serverAddr = serverAddr
	cli.serverPort = serverPort

	var tcpAddr *net.TCPAddr
	tcpAddr, _ = net.ResolveTCPAddr("tcp", cli.serverAddr+":"+strconv.Itoa(cli.serverPort))

	conn, err := net.DialTCP("tcp", nil, tcpAddr)
	if err != nil {
		return err
	}
	cli.session = &Session{}
	cli.session.Init(conn, 1, nil)
	cli.session.Start()

	return nil
}

func (cli *TcpClient) Send(protoId int32, data interface{}) error {
	return cli.session.Send(protoId, data)
}

func (cli *TcpClient) Close() {
	cli.session.Close()
}
