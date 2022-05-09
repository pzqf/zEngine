package zNet

func SendToClient(sid int64, netPacket *NetPacket) {
	client := TcpServerInstance.GetSession(sid)
	if client != nil {
		_ = client.Send(netPacket)
	}
}

func BroadcastToClient(netPacket *NetPacket) {
	TcpServerInstance.ClientSessionMap.Range(func(key, value interface{}) bool {
		session := value.(*Session)
		_ = session.Send(netPacket)
		return true
	})
}

func GetSession(sid int64) *Session {
	return TcpServerInstance.GetSession(sid)
}
