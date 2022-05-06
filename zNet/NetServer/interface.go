package NetServer

//todo 此处有较大优化空间，

func SendToClientByUID(uid int64, protoId int32, data interface{}) {
	var sendToClient *Session = nil
	for _, client := range TcpServerInstance.ClientSessionMap {
		if client.uid == uid {
			sendToClient = client
			break
		}
	}
	if sendToClient != nil {
		_ = sendToClient.Send(protoId, data)
	}
}

func SendToClientBySID(SID int64, protoId int32, data interface{}) {
	if client, ok := TcpServerInstance.ClientSessionMap[SID]; ok {
		_ = client.Send(protoId, data)
	}
}

func BroadcastToClient(protoId int32, data interface{}) {
	for _, cli := range TcpServerInstance.ClientSessionMap {
		_ = cli.Send(protoId, data)
	}
}

func SendToClientList(uidList []int64, protoId int32, data interface{}) {
	for _, uid := range uidList {
		var cli *Session = nil
		for _, client := range TcpServerInstance.ClientSessionMap {
			if client.uid == uid {
				cli = client
				break
			}
		}
		if cli != nil {
			_ = cli.Send(protoId, data)
		}
	}
}

func GetSession(sid int64) *Session {
	if client, ok := TcpServerInstance.ClientSessionMap[sid]; ok {
		return client
	}

	return nil
}

func SetSessionUid(sid int64, uid int64) {
	session := GetSession(sid)
	if session != nil {
		session.SetUid(uid)
	}
}
