package NetServer

import "errors"

type HandlerFun func(sid int64, packet *NetPacket)

var mapHandler = make(map[int32]HandlerFun)

func RegisterHandler(protoId int32, fun HandlerFun) error {
	if _, ok := mapHandler[protoId]; ok {
		return errors.New("had handlerFun")
	}
	mapHandler[protoId] = fun

	return nil
}

func Dispatcher(sid int64, netPacket *NetPacket) error {
	if netPacket == nil {
		return errors.New("packet is nil")
	}
	fun, ok := mapHandler[netPacket.ProtoId]
	if !ok {
		return errors.New("no handlerFun")
	}

	fun(sid, netPacket)

	return nil
}
