package zNet

var TcpServerInstance *TcpServer

func InitDefaultTcpServer(address string, maxClientCount int32) {
	TcpServerInstance = NewTcpServer(address, maxClientCount)
	return
}

func StartDefaultTcpServer() error {
	return TcpServerInstance.Start()
}

func CloseDefaultTcpServer() {
	TcpServerInstance.Close()
}

func SendToClient(sid int64, netPacket *NetPacket) {
	client := TcpServerInstance.GetSession(sid)
	if client != nil {
		_ = client.Send(netPacket)
	}
}

func BroadcastToClient(netPacket *NetPacket) {
	TcpServerInstance.clientSessionMap.Range(func(key, value interface{}) bool {
		session := value.(*Session)
		_ = session.Send(netPacket)
		return true
	})
}
