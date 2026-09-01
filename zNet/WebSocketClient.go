package zNet

import (
	"context"
	"encoding/binary"
	"errors"
	"net/url"
	"strconv"
)

type WebSocketClient struct {
	serverAddr           string
	serverPort           int
	session              *WebSocketClientSession
	dispatcher           HandlerFun
	heartbeatDuration    int
	maxPacketDataSize    int32 // Deprecated: 使用 maxWirePacketSize；保留内部兼容测试/调用
	maxWirePacketSize    int32
	maxDecodedPacketSize int32
	logger               Logger
	packetCodec          PacketCodec
}

func (cli *WebSocketClient) ConnectToServer(serverAddr string, serverPort int, heartbeatDuration int, maxPacketDataSize int32) error {
	if cli.packetCodec.order == nil {
		cli.packetCodec = NewPacketCodec(GetByteOrder())
	}
	cli.serverAddr = serverAddr
	cli.serverPort = serverPort
	cli.heartbeatDuration = heartbeatDuration
	cli.maxPacketDataSize = maxPacketDataSize
	cli.maxWirePacketSize = maxPacketDataSize
	normalizePacketSizeLimits(&cli.maxWirePacketSize, &cli.maxPacketDataSize, &cli.maxDecodedPacketSize)

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

func (cli *WebSocketClient) SendContext(ctx context.Context, protoId ProtoIdType, data []byte) error {
	return cli.SendWithOptions(ctx, protoId, data, SendOptions{Class: ReliableCommand})
}

func (cli *WebSocketClient) SendWithOptions(ctx context.Context, protoId ProtoIdType, data []byte, options SendOptions) error {
	if cli.session == nil {
		return errors.New("websocket client session is nil")
	}
	return cli.session.SendWithOptions(ctx, protoId, data, options)
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

// SetPacketSizeLimits 设置后续连接使用的 wire/decoded 两级上限。
func (cli *WebSocketClient) SetPacketSizeLimits(maxWirePacketSize, maxDecodedPacketSize int32) {
	cli.maxWirePacketSize = maxWirePacketSize
	cli.maxPacketDataSize = maxWirePacketSize
	cli.maxDecodedPacketSize = maxDecodedPacketSize
	normalizePacketSizeLimits(&cli.maxWirePacketSize, &cli.maxPacketDataSize, &cli.maxDecodedPacketSize)
}

func (cli *WebSocketClient) wirePacketSize() int32 {
	return resolveWirePacketSize(cli.maxWirePacketSize, cli.maxPacketDataSize)
}

func (cli *WebSocketClient) decodedPacketSize() int32 {
	return resolveDecodedPacketSize(cli.maxDecodedPacketSize)
}

// SetByteOrder 设置后续连接使用的端点字节序。应在 ConnectToServer 前调用。
func (cli *WebSocketClient) SetByteOrder(order binary.ByteOrder) {
	cli.packetCodec = NewPacketCodec(order)
}

// SetWireByteOrder 从可序列化配置值设置后续连接使用的端点字节序。
func (cli *WebSocketClient) SetWireByteOrder(order WireByteOrder) error {
	codec, err := newEndpointPacketCodec(order)
	if err != nil {
		return err
	}
	cli.packetCodec = codec
	return nil
}
