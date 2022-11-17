package zNet

type Session interface {
	Start()
	Close()
	Send(protoId int32, data []byte) error
	GetSid() SessionIdType
}

type NetServer interface {
	Start() error
	Close()
}
