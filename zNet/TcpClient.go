package zNet

import (
	"net"
	"strconv"
)

// TcpClient TCP客户端
// 用于连接到TCP服务器并进行通信
type TcpClient struct {
	serverAddr        string            // 服务器地址
	serverPort        int               // 服务器端口
	session           *TcpClientSession // 会话实例
	dispatcher        HandlerFun        // 消息分发器
	heartbeatDuration int               // 心跳间隔（秒）
	maxPacketDataSize int32             // 最大数据包大小
	logger            Logger            // 日志记录器
}

// ConnectToServer 连接到服务器
// 建立TCP连接，执行DH密钥交换，初始化会话
//
// 参数:
//   - serverAddr: 服务器地址
//   - serverPort: 服务器端口
//   - rsaPublicFile: RSA公钥文件（保留参数，当前使用DH密钥交换）
//   - heartbeatDuration: 心跳间隔（秒）
//   - maxPacketDataSize: 最大数据包大小
//
// 返回:
//   - error: 连接失败时返回错误
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

	// 初始化会话，传递协商得到的 AES 密钥
	cli.session.Init(cli, conn, aesKey)
	cli.session.Start()

	return nil
}

// Send 发送数据
// 参数:
//   - protoId: 协议ID
//   - data: 要发送的数据
//
// 返回:
//   - error: 发送失败时返回错误
func (cli *TcpClient) Send(protoId int32, data []byte) error {
	return cli.session.Send(protoId, data)
}

// Close 关闭客户端
// 关闭会话并断开连接
func (cli *TcpClient) Close() {
	cli.session.Close()
}

// RegisterHandler 注册消息处理器
// 参数:
//   - fun: 消息处理函数
//   - n: 保留参数
//
// 返回:
//   - error: 注册失败时返回错误
func (cli *TcpClient) RegisterHandler(fun HandlerFun, n int) error {
	cli.dispatcher = fun
	return nil
}
