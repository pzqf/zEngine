package zNet

import "github.com/pzqf/zEngine/zLog"

// Logger 日志接口
type Logger zLog.Logger

type Session interface {
	Start()
	Close()
	Send(protoId int32, data []byte) error
	GetSid() SessionIdType
}

type Server interface {
	Start() error
	Close()
}
