package zNet

import (
	"crypto/x509"
	"encoding/pem"
	"io"
	"os"
)

// Options 服务器配置选项函数类型
// 用于函数式配置模式，通过闭包设置服务器属性
type Options func(Server)

// 说明（成熟化改造 Phase 1.1.3）：以下选项统一使用 Go 类型断言（type switch）
// 分派到具体 Server 实现，取代原先的 reflect.TypeOf(svr).String() 字符串匹配。
// 字符串匹配脆弱（改包名/类型名即静默失效）、有反射开销且无法编译期校验；
// 类型 switch 由编译器检查、零反射、语义清晰。

// WithMaxClientCount 设置最大客户端连接数
func WithMaxClientCount(count int) Options {
	return func(svr Server) {
		switch s := svr.(type) {
		case *TcpServer:
			s.config.MaxClientCount = count
		case *UdpServer:
			s.config.MaxClientCount = count
		case *WebSocketServer:
			s.config.MaxClientCount = count
		case *HttpServer:
			s.config.MaxClientCount = count
		}
	}
}

// WithMaxPacketDataSize 设置最大数据包数据大小
func WithMaxPacketDataSize(size int32) Options {
	return func(svr Server) {
		size = resolveWirePacketSize(size, size)
		switch s := svr.(type) {
		case *TcpServer:
			s.config.MaxPacketDataSize = size
			s.config.MaxWirePacketSize = size
		case *UdpServer:
			s.config.MaxPacketDataSize = size
			s.config.MaxWirePacketSize = size
		case *WebSocketServer:
			s.config.MaxPacketDataSize = size
			s.config.MaxWirePacketSize = size
		case *HttpServer:
			s.config.MaxPacketDataSize = size
		}
	}
}

// WithMaxWirePacketSize 设置线包 payload 上限，并同步旧字段供兼容调用者读取。
func WithMaxWirePacketSize(size int32) Options {
	return WithMaxPacketDataSize(size)
}

// WithMaxDecodedPacketSize 设置解密和解压后的 payload 上限。
func WithMaxDecodedPacketSize(size int32) Options {
	return func(svr Server) {
		size = resolveDecodedPacketSize(size)
		switch s := svr.(type) {
		case *TcpServer:
			s.config.MaxDecodedPacketSize = size
		case *UdpServer:
			s.config.MaxDecodedPacketSize = size
		case *WebSocketServer:
			s.config.MaxDecodedPacketSize = size
		}
	}
}

// WithRsaEncrypt 启用RSA加密
// 从指定文件加载RSA私钥用于加密通信（仅 TcpServer 支持）
func WithRsaEncrypt(rsaPrivateFile string) Options {
	return func(svr Server) {
		s, ok := svr.(*TcpServer)
		if !ok || rsaPrivateFile == "" {
			return
		}
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

		s.privateKey = prkI
	}
}

// WithChanSize 设置通道大小
func WithChanSize(chanSize int) Options {
	return func(svr Server) {
		if chanSize <= 0 {
			return
		}
		switch s := svr.(type) {
		case *TcpServer:
			s.config.ChanSize = chanSize
		case *UdpServer:
			s.config.ChanSize = chanSize
		case *WebSocketServer:
			s.config.ChanSize = chanSize
		}
	}
}

// WithServerMetrics 注入网络指标上报器。
// 传入的对象需满足 NetworkMetricsRecorder（zMetrics.NetworkMetrics 即可）；
// 注入后 zNet 会上报连接生命周期、收发字节/包数与解码错误。未注入则不上报。
func WithServerMetrics(recorder NetworkMetricsRecorder) Options {
	return func(svr Server) {
		switch s := svr.(type) {
		case *TcpServer:
			s.metrics = recorder
		case *UdpServer:
			s.metrics = recorder
		case *WebSocketServer:
			s.metrics = recorder
		}
	}
}

// WithHeartbeat 设置心跳间隔（秒）
func WithHeartbeat(duration int) Options {
	return func(svr Server) {
		switch s := svr.(type) {
		case *TcpServer:
			s.config.HeartbeatDuration = duration
		case *UdpServer:
			s.config.HeartbeatDuration = duration
		case *WebSocketServer:
			s.config.HeartbeatDuration = duration
		}
	}
}

// WithAddSessionCallBack 设置Session添加回调
func WithAddSessionCallBack(cb SessionCallBackFunc) Options {
	return func(svr Server) {
		switch s := svr.(type) {
		case *TcpServer:
			s.onAddSession = cb
		case *UdpServer:
			s.onAddSession = cb
		case *WebSocketServer:
			s.onAddSession = cb
		case *HttpServer:
			s.onAddSession = cb
		}
	}
}

// WithRemoveSessionCallBack 设置Session移除回调
func WithRemoveSessionCallBack(cb SessionCallBackFunc) Options {
	return func(svr Server) {
		switch s := svr.(type) {
		case *TcpServer:
			s.onRemoveSession = cb
		case *UdpServer:
			s.onRemoveSession = cb
		case *WebSocketServer:
			s.onRemoveSession = cb
		case *HttpServer:
			s.onRemoveSession = cb
		}
	}
}

// WithWorkerPoolSize 已废弃、无副作用：工作池大小请通过 TcpConfig.WorkerPoolSize 配置。
//
// 关于 FIFO（成熟化改造 Phase 1.1.4 澄清）：TcpConfig.UseWorkerPool=true 时，同一会话的
// 数据包会并发 Submit 到共享工作池，**不再保证会话内 FIFO 顺序**；对需要按连接有序处理的
// 场景（如游戏逻辑，玩家消息必须有序）应保持默认的 UseWorkerPool=false（逐会话单 goroutine
// 顺序处理）。此前本函数注释误称"worker pool 已移除保 FIFO"，与 UseWorkerPool 仍可用不符，故更正。
func WithWorkerPoolSize(size int) Options {
	return func(svr Server) {
		// no-op：工作池大小由 TcpConfig.WorkerPoolSize 决定，此处保留仅为向后兼容
	}
}

// WithLogger 设置日志记录器
func WithLogger(logger Logger) Options {
	return func(svr Server) {
		switch s := svr.(type) {
		case *TcpServer:
			s.logger = logger
		case *UdpServer:
			s.logger = logger
		case *WebSocketServer:
			s.logger = logger
		case *HttpServer:
			s.logger = logger
		}
	}
}

// WithDDoSConfig 设置DDoS保护配置
func WithDDoSConfig(cfg *DDoSConfig) Options {
	return func(svr Server) {
		switch s := svr.(type) {
		case *TcpServer:
			s.ddosProtection = NewDDoSProtection(cfg)
		case *UdpServer:
			s.ddosProtection = NewDDoSProtection(cfg)
		case *WebSocketServer:
			s.ddosProtection = NewDDoSProtection(cfg)
		case *HttpServer:
			s.ddosProtection = NewDDoSProtection(cfg)
		}
	}
}

// WithCompressionConfig 设置压缩配置（仅 TcpServer 支持）
func WithCompressionConfig(cfg *CompressionConfig) Options {
	return func(svr Server) {
		if s, ok := svr.(*TcpServer); ok {
			s.compressionConfig = cfg
		}
	}
}

// ClientOption 客户端配置选项函数类型
// 用于函数式配置模式，通过闭包设置客户端属性
type ClientOption func(*TcpClient)

// WithClientLogger 设置客户端日志记录器
func WithClientLogger(logger Logger) ClientOption {
	return func(cli *TcpClient) {
		cli.logger = logger
	}
}

// WithClientStateCallback 设置客户端状态回调
func WithClientStateCallback(cb ClientStateCallback) ClientOption {
	return func(cli *TcpClient) {
		cli.stateCallbacks = append(cli.stateCallbacks, cb)
	}
}
