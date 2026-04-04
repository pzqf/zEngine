package zNet

import (
	"errors"
	"net/url"
	"strconv"
)

type WebSocketClient struct {
	serverAddr        string
	serverPort        int
	session           *WebSocketClientSession
	dispatcher        HandlerFun
	heartbeatDuration int
	maxPacketDataSize int32
	logger            Logger
}

func (cli *WebSocketClient) ConnectToServer(serverAddr string, serverPort int, heartbeatDuration int, maxPacketDataSize int32) error {
	cli.serverAddr = serverAddr
	cli.serverPort = serverPort
	cli.heartbeatDuration = heartbeatDuration
	cli.maxPacketDataSize = maxPacketDataSize

	wsURL := url.URL{
		Scheme: "ws",
		Host:   serverAddr + ":" + strconv.Itoa(serverPort),
		Path:   "/",
	}

	cli.session = &WebSocketClientSession{}
	err := cli.session.Init(cli, wsURL.String())
	if err != nil {
		return err
	}

	cli.session.Start()
	return nil
}

func (cli *WebSocketClient) Send(protoId ProtoIdType, data []byte) error {
	if cli.session == nil {
		return errors.New("websocket client session is nil")
	}
	return cli.session.Send(protoId, data)
}

func (cli *WebSocketClient) Close() {
	if cli.session != nil {
		cli.session.Close()
	}
}

func (cli *WebSocketClient) SetDispatcher(dispatcher HandlerFun) {
	cli.dispatcher = dispatcher
}

func (cli *WebSocketClient) SetLogger(logger Logger) {
	cli.logger = logger
}
