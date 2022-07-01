package zNet

import (
	"errors"
	"fmt"
	"reflect"
	"runtime"

	"github.com/panjf2000/ants"
)

type HandlerFun func(session Session, protoId int32, data []byte)

var mapHandler = make(map[int32]HandlerFun)
var defaultPoolSize = 10000
var workerPool *ants.Pool

func InitDispatcherWorkerPool(n int) {
	defaultPoolSize = n
	if defaultPoolSize <= 100 {
		defaultPoolSize = 10000
	}
}

func RegisterHandler(protoId int32, fun HandlerFun) error {
	if workerPool == nil {
		p, err := ants.NewPool(defaultPoolSize)
		if err != nil {
			panic(err)
		}
		workerPool = p
	}

	if _, ok := mapHandler[protoId]; ok {
		return errors.New(fmt.Sprintf("protoId %d had handlerFun", protoId))
	}
	mapHandler[protoId] = fun

	LogPrint("Register handler", protoId, runtime.FuncForPC(reflect.ValueOf(fun).Pointer()).Name())

	return nil
}

func Dispatcher(session Session, netPacket *NetPacket) error {
	if netPacket == nil {
		return errors.New("nil packet")
	}

	fun, ok := mapHandler[netPacket.ProtoId]
	if !ok {
		return errors.New(fmt.Sprintf("protoId %d no handlerFun", netPacket.ProtoId))
	}
	err := workerPool.Submit(func() {
		fun(session, netPacket.ProtoId, netPacket.Data)
	})
	if err != nil {
		return err
	}
	return nil
}

func GetHandler() map[int32]HandlerFun {
	return mapHandler
}
