package zNet

var TcpServerInstance *TcpServer

func InitTcpServerDefault(cfg *TcpConfig, opts ...Options) {
	TcpServerInstance = NewTcpServer(cfg, opts...)
	return
}

func GetTcpServerDefault() *TcpServer {
	return TcpServerInstance
}
