package zNet

import (
	"net"
	"strconv"
)

type UdpClient struct {
	serverAddr string
	serverPort int
	session    *UdpClientSession
}

func (cli *UdpClient) ConnectToServer(serverAddr string, serverPort int) error {
	cli.serverAddr = serverAddr
	cli.serverPort = serverPort

	udpAddr, _ := net.ResolveUDPAddr("udp", cli.serverAddr+":"+strconv.Itoa(cli.serverPort))

	conn, err := net.DialUDP("udp", nil, udpAddr)
	if err != nil {
		return err
	}
	cli.session = &UdpClientSession{}
	cli.session.Init(conn, nil)
	cli.session.Start()

	return nil
}

func (cli *UdpClient) Send(protoId int32, data []byte) error {
	return cli.session.Send(protoId, data)
}

func (cli *UdpClient) Close() {
	_ = cli.session.conn.Close()
}
