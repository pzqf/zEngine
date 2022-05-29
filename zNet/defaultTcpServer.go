package zNet

var TcpServerInstance *TcpServer

func InitDefaultTcpServer(address string, opts ...Options) {
	TcpServerInstance = NewTcpServer(address, opts...)
	return
}

func GetDefaultTcpServer() *TcpServer {
	return TcpServerInstance
}
