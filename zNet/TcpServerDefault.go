package zNet

var TcpServerInstance *TcpServer

func InitTcpServerDefault(cfg *Config, opts ...Options) {
	TcpServerInstance = NewTcpServer(cfg, opts...)
	return
}

func GetTcpServerDefault() *TcpServer {
	return TcpServerInstance
}
