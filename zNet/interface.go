package zNet

func SendToClient(sid int64, netPacket *NetPacket) {
	client := TcpServerInstance.GetSession(sid)
	if client != nil {
		_ = client.Send(netPacket)
	}
}

func BroadcastToClient(netPacket *NetPacket) {
	var list []*Session
	TcpServerInstance.locker.Lock()
	for _, v := range TcpServerInstance.ClientSessionMap {
		list = append(list, v)
	}
	TcpServerInstance.locker.Unlock()

	for _, v := range list {
		_ = v.Send(netPacket)
	}
}

func GetSession(sid int64) *Session {
	return TcpServerInstance.GetSession(sid)
}
