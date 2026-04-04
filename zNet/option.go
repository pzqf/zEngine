package zNet

import (
	"crypto/x509"
	"encoding/pem"
	"io"
	"os"
	"reflect"
)

// Options 服务器配置选项函数类型
// 用于函数式配置模式，通过闭包设置服务器属性
type Options func(Server)

// WithMaxClientCount 设置最大客户端连接数
// 参数:
//   - count: 最大客户端连接数
//
// 返回:
//   - Options: 配置函数
func WithMaxClientCount(count int) Options {
	return func(svr Server) {
		switch reflect.TypeOf(svr).String() {
		case "*zNet.TcpServer":
			svr.(*TcpServer).config.MaxClientCount = count
		case "*zNet.UdpServer":
			svr.(*UdpServer).config.MaxClientCount = count
		case "*zNet.WebSocketServer":
			svr.(*WebSocketServer).config.MaxClientCount = count
		case "*zNet.HttpServer":
			svr.(*HttpServer).config.MaxClientCount = count
		}
	}
}

// WithMaxPacketDataSize 设置最大数据包数据大小
// 参数:
//   - size: 数据包最大字节数
//
// 返回:
//   - Options: 配置函数
func WithMaxPacketDataSize(size int32) Options {
	return func(svr Server) {
		switch reflect.TypeOf(svr).String() {
		case "*zNet.TcpServer":
			svr.(*TcpServer).config.MaxPacketDataSize = size
		case "*zNet.UdpServer":
			svr.(*UdpServer).config.MaxPacketDataSize = size
		case "*zNet.WebSocketServer":
			svr.(*WebSocketServer).config.MaxPacketDataSize = size
		case "*zNet.HttpServer":
			svr.(*HttpServer).config.MaxPacketDataSize = size
		}
	}
}

// WithRsaEncrypt 启用RSA加密
// 从指定文件加载RSA私钥用于加密通信
//
// 参数:
//   - rsaPrivateFile: RSA私钥文件路径
//
// 返回:
//   - Options: 配置函数
func WithRsaEncrypt(rsaPrivateFile string) Options {
	return func(svr Server) {
		if rsaPrivateFile != "" && reflect.TypeOf(svr).String() == "*zNet.TcpServer" {
			f, err := os.Open(rsaPrivateFile)
			if err != nil {
				return
			}
			all, err := io.ReadAll(f)
			if err != nil {
				return
			}

			block, _ := pem.Decode(all)
			if block == nil {
				return
			}

			prkI, err := x509.ParsePKCS1PrivateKey(block.Bytes)
			if err != nil {
				return
			}

			svr.(*TcpServer).privateKey = prkI
		}
	}
}

// WithChanSize 设置通道大小
// 参数:
//   - chanSize: 通道缓冲区大小
//
// 返回:
//   - Options: 配置函数
func WithChanSize(chanSize int) Options {
	return func(svr Server) {
		if chanSize <= 0 {
			return
		}
		switch reflect.TypeOf(svr).String() {
		case "*zNet.TcpServer":
			svr.(*TcpServer).config.ChanSize = chanSize
		case "*zNet.UdpServer":
			svr.(*UdpServer).config.ChanSize = chanSize
		case "*zNet.WebSocketServer":
			svr.(*WebSocketServer).config.ChanSize = chanSize
		}
	}
}

// WithHeartbeat 设置心跳间隔
// 参数:
//   - duration: 心跳间隔时间（秒）
//
// 返回:
//   - Options: 配置函数
func WithHeartbeat(duration int) Options {
	return func(svr Server) {
		switch reflect.TypeOf(svr).String() {
		case "*zNet.TcpServer":
			svr.(*TcpServer).config.HeartbeatDuration = duration
		case "*zNet.UdpServer":
			svr.(*UdpServer).config.HeartbeatDuration = duration
		case "*zNet.WebSocketServer":
			svr.(*WebSocketServer).config.HeartbeatDuration = duration
		}
	}
}

// WithAddSessionCallBack 设置Session添加回调
// 参数:
//   - cb: Session添加时的回调函数
//
// 返回:
//   - Options: 配置函数
func WithAddSessionCallBack(cb SessionCallBackFunc) Options {
	return func(svr Server) {
		switch reflect.TypeOf(svr).String() {
		case "*zNet.TcpServer":
			svr.(*TcpServer).onAddSession = cb
		case "*zNet.UdpServer":
			svr.(*UdpServer).onAddSession = cb
		case "*zNet.WebSocketServer":
			svr.(*WebSocketServer).onAddSession = cb
		case "*zNet.HttpServer":
			svr.(*HttpServer).onAddSession = cb
		}
	}
}

// WithRemoveSessionCallBack 设置Session移除回调
// 参数:
//   - cb: Session移除时的回调函数
//
// 返回:
//   - Options: 配置函数
func WithRemoveSessionCallBack(cb SessionCallBackFunc) Options {
	return func(svr Server) {
		switch reflect.TypeOf(svr).String() {
		case "*zNet.TcpServer":
			svr.(*TcpServer).onRemoveSession = cb
		case "*zNet.UdpServer":
			svr.(*UdpServer).onRemoveSession = cb
		case "*zNet.WebSocketServer":
			svr.(*WebSocketServer).onRemoveSession = cb
		case "*zNet.HttpServer":
			svr.(*HttpServer).onRemoveSession = cb
		}
	}
}

// WithWorkerPoolSize 设置工作池大小（已废弃）
// 为了确保FIFO数据包处理顺序，Worker Pool已被移除
// 此函数保留用于向后兼容
func WithWorkerPoolSize(size int) Options {
	return func(svr Server) {
		// Worker pool has been removed to ensure FIFO packet processing
		// This function is kept for backward compatibility
	}
}

// WithLogger 设置日志记录器
// 参数:
//   - logger: 日志记录器实例
//
// 返回:
//   - Options: 配置函数
func WithLogger(logger Logger) Options {
	return func(svr Server) {
		switch reflect.TypeOf(svr).String() {
		case "*zNet.TcpServer":
			svr.(*TcpServer).logger = logger
		case "*zNet.UdpServer":
			svr.(*UdpServer).logger = logger
		case "*zNet.WebSocketServer":
			svr.(*WebSocketServer).logger = logger
		case "*zNet.HttpServer":
			svr.(*HttpServer).logger = logger
		}
	}
}

// WithDDoSConfig 设置DDoS保护配置
// 参数:
//   - cfg: DDoS保护配置
//
// 返回:
//   - Options: 配置函数
func WithDDoSConfig(cfg *DDoSConfig) Options {
	return func(svr Server) {
		switch reflect.TypeOf(svr).String() {
		case "*zNet.TcpServer":
			svr.(*TcpServer).ddosProtection = NewDDoSProtection(cfg)
		case "*zNet.UdpServer":
			svr.(*UdpServer).ddosProtection = NewDDoSProtection(cfg)
		case "*zNet.WebSocketServer":
			svr.(*WebSocketServer).ddosProtection = NewDDoSProtection(cfg)
		case "*zNet.HttpServer":
			svr.(*HttpServer).ddosProtection = NewDDoSProtection(cfg)
		}
	}
}

// WithCompressionConfig 设置压缩配置
// 参数:
//   - cfg: 压缩配置
//
// 返回:
//   - Options: 配置函数
func WithCompressionConfig(cfg *CompressionConfig) Options {
	return func(svr Server) {
		switch reflect.TypeOf(svr).String() {
		case "*zNet.TcpServer":
			svr.(*TcpServer).compressionConfig = cfg
		}
	}
}

// ClientOption 客户端配置选项函数类型
// 用于函数式配置模式，通过闭包设置客户端属性
type ClientOption func(*TcpClient)

// WithClientLogger 设置客户端日志记录器
// 参数:
//   - logger: 日志记录器实例
//
// 返回:
//   - ClientOption: 配置函数
func WithClientLogger(logger Logger) ClientOption {
	return func(cli *TcpClient) {
		cli.logger = logger
	}
}

// WithClientStateCallback 设置客户端状态回调
// 参数:
//   - cb: 状态回调函数
//
// 返回:
//   - ClientOption: 配置函数
func WithClientStateCallback(cb ClientStateCallback) ClientOption {
	return func(cli *TcpClient) {
		cli.stateCallbacks = append(cli.stateCallbacks, cb)
	}
}
