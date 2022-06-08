package zNet

type Session interface {
	Send(protoId int32, data []byte) error
}
