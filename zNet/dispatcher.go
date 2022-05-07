package zNet

import "errors"

type HandlerFun func(session *Session, packet *NetPacket)

var mapHandler = make(map[int32]HandlerFun)

func RegisterHandler(protoId int32, fun HandlerFun) error {
	if _, ok := mapHandler[protoId]; ok {
		return errors.New("had handlerFun")
	}
	mapHandler[protoId] = fun

	return nil
}

func Dispatcher(session *Session, netPacket *NetPacket) error {
	if netPacket == nil {
		return errors.New("packet is nil")
	}
	fun, ok := mapHandler[netPacket.ProtoId]
	if !ok {
		return errors.New("no handlerFun")
	}

	fun(session, netPacket)

	return nil
}
