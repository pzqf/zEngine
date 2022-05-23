package zNet

var TcpServerInstance *TcpServer

func InitDefaultTcpServer(address string, maxClientCount int32) {
	TcpServerInstance = NewTcpServer(address, maxClientCount)
	return
}

func GetDefaultTcpServer() *TcpServer {
	return TcpServerInstance
}
