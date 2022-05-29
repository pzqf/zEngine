package zNet

import (
	"errors"

	"github.com/panjf2000/ants"
)

type HandlerFun func(session *Session, packet *NetPacket)

var mapHandler = make(map[int32]HandlerFun)
var defaultPoolSize = 10000
var workerPool *ants.Pool

func RegisterHandler(protoId int32, fun HandlerFun) error {
	p, err := ants.NewPool(defaultPoolSize)
	if err != nil {
		panic(err)
	}
	workerPool = p

	if _, ok := mapHandler[protoId]; ok {
		return errors.New("had handlerFun")
	}
	mapHandler[protoId] = fun

	return nil
}

func Dispatcher(session *Session, netPacket *NetPacket) error {
	if netPacket == nil {
		return errors.New("nil packet")
	}

	fun, ok := mapHandler[netPacket.ProtoId]
	if !ok {
		return errors.New("no handlerFun")
	}
	workerPool.Submit(func() {
		fun(session, netPacket)
	})
	return nil
}

func init() {
	_ = RegisterHandler(0, heartbeat)
}

func heartbeat(session *Session, packet *NetPacket) {
	session.heartbeatUpdate()
	sendPacket := NetPacket{
		ProtoId:  0,
		DataSize: packet.DataSize,
		Data:     packet.Data,
	}
	_, _ = session.send(&sendPacket)
}
