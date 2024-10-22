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
	var aesKey []byte

	/*
		helloBuf := make([]byte, 5)
		_, err = io.ReadFull(conn, helloBuf)
		if err != nil {
			return err
		}

		if string(helloBuf) == "hello" {
			h := md5.New()
			h.Write([]byte(fmt.Sprintf("aes%d", time.Now().UnixNano())))
			aesKey = []byte(hex.EncodeToString(h.Sum(nil)))

			f, err := os.Open(rsaPublicFile)
			if err != nil {
				return err
			}
			all, err := io.ReadAll(f)
			if err != nil {
				return err
			}

			block, _ := pem.Decode(all)
			if block == nil {
				return errors.New("public key error")
			}
			prkI, err := x509.ParsePKIXPublicKey(block.Bytes)
			if err != nil {
				return err
			}
			priKey := prkI.(*rsa.PublicKey)
			v15, err := rsa.EncryptPKCS1v15(rand.Reader, priKey, aesKey)
			if err != nil {
				return err
			}

			_, _ = conn.Write(v15)
		}
	*/
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
