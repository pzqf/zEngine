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

func SendToClient(sid int64, protoId int32, data interface{}) {
	client := TcpServerInstance.GetSession(sid)
	if client != nil {
		_ = client.Send(protoId, data)
	}
}

func BroadcastToClient(protoId int32, data interface{}) {
	TcpServerInstance.BroadcastToClient(protoId, data)
}
