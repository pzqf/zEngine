package zNet

import (
	"net"
	"strconv"
)

type TcpClient struct {
	serverAddr        string
	serverPort        int
	session           *TcpClientSession
	dispatcher        HandlerFun
	heartbeatDuration int
	maxPacketDataSize int32
	logger            Logger
}

func (cli *TcpClient) ConnectToServer(serverAddr string, serverPort int, rsaPublicFile string, heartbeatDuration int, maxPacketDataSize int32) error {
	cli.serverAddr = serverAddr
	cli.serverPort = serverPort
	cli.heartbeatDuration = heartbeatDuration
	cli.maxPacketDataSize = maxPacketDataSize

	tcpAddr, _ := net.ResolveTCPAddr("tcp", cli.serverAddr+":"+strconv.Itoa(cli.serverPort))

	conn, err := net.DialTCP("tcp", nil, tcpAddr)
	if err != nil {
		return err
	}
	cli.session = &TcpClientSession{}

	// 执行DH密钥协商
	aesKey, err := PerformKeyExchange(conn)
	if err != nil {
		conn.Close()
		return err
	}

	if cli.logger != nil {
		cli.logger.Info("DH key exchange completed successfully, AES key length: %d", len(aesKey))
	}

	// 步骤3：初始化会话，传递协商得到的 AES 密钥
	cli.session.Init(cli, conn, aesKey)
	cli.session.Start()

	return nil
}

func (cli *TcpClient) Send(protoId int32, data []byte) error {
	return cli.session.Send(protoId, data)
}

func (cli *TcpClient) Close() {
	cli.session.Close()
}

func (cli *TcpClient) RegisterHandler(fun HandlerFun, n int) error {
	cli.dispatcher = fun
	return nil
}
