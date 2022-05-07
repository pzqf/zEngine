package zNet

func SendToClient(sid int64, protoId int32, data interface{}) {
	client := TcpServerInstance.GetSession(sid)
	if client != nil {
		_ = client.Send(protoId, data)
	}
}

func BroadcastToClient(protoId int32, data interface{}) {
	var list []*Session
	TcpServerInstance.locker.Lock()
	for _, v := range TcpServerInstance.ClientSessionMap {
		list = append(list, v)
	}
	TcpServerInstance.locker.Unlock()

	for _, v := range list {
		_ = v.Send(protoId, data)
	}
}

func GetSession(sid int64) *Session {
	return TcpServerInstance.GetSession(sid)
}
