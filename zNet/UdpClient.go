package zNet

import (
	"errors"
	"net"
	"strconv"
)

type UdpClient struct {
	serverAddr        string
	serverPort        int
	session           *UdpClientSession
	dispatcher        HandlerFun
	heartbeatDuration int
	maxPacketDataSize int32
	logger            Logger
}

func (cli *UdpClient) ConnectToServer(serverAddr string, serverPort int, heartbeatDuration int, maxPacketDataSize int32) error {
	cli.serverAddr = serverAddr
	cli.serverPort = serverPort
	cli.heartbeatDuration = heartbeatDuration
	cli.maxPacketDataSize = maxPacketDataSize

	udpAddr, err := net.ResolveUDPAddr("udp", cli.serverAddr+":"+strconv.Itoa(cli.serverPort))
	if err != nil {
		return err
	}

	conn, err := net.DialUDP("udp", nil, udpAddr)
	if err != nil {
		return err
	}

	cli.session = &UdpClientSession{}
	cli.session.Init(cli, conn)
	cli.session.Start()

	return nil
}

func (cli *UdpClient) Send(protoId int32, data []byte) error {
	if cli.session == nil {
		return errors.New("udp client session is nil")
	}
	return cli.session.Send(protoId, data)
}

func (cli *UdpClient) Close() {
	if cli.session != nil {
		cli.session.Close()
	}
}

func (cli *UdpClient) SetDispatcher(dispatcher HandlerFun) {
	cli.dispatcher = dispatcher
}

func (cli *UdpClient) SetLogger(logger Logger) {
	cli.logger = logger
}
