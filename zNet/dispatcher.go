package zNet

import (
	"errors"
	"fmt"
	"reflect"
	"runtime"

	"github.com/panjf2000/ants"
)

type HandlerFunc func(session Session, protoId int32, data []byte)

type DispatcherFunc func(session Session, netPacket *NetPacket) error

var defaultPoolSize = 2000

type Dispatcher struct {
	mapHandler map[int32]HandlerFunc
	workerPool *ants.Pool
}

func (d *Dispatcher) RegisterHandler(protoId int32, fun HandlerFunc) error {
	if d.workerPool == nil {
		p, err := ants.NewPool(defaultPoolSize)
		if err != nil {
			panic(err)
		}
		d.workerPool = p
	}
	if d.mapHandler == nil {
		d.mapHandler = make(map[int32]HandlerFunc)
	}

	if _, ok := d.mapHandler[protoId]; ok {
		return errors.New(fmt.Sprintf("protoId %d had handlerFun", protoId))
	}
	d.mapHandler[protoId] = fun

	LogPrint("Register handler", protoId, runtime.FuncForPC(reflect.ValueOf(fun).Pointer()).Name())

	return nil
}

func (d *Dispatcher) DispatcherFun(session Session, netPacket *NetPacket) error {
	if netPacket == nil {
		return errors.New("nil packet")
	}

	fun, ok := d.mapHandler[netPacket.ProtoId]
	if !ok {
		return errors.New(fmt.Sprintf("protoId %d no handlerFun", netPacket.ProtoId))
	}
	err := d.workerPool.Submit(func() {
		fun(session, netPacket.ProtoId, netPacket.Data)
	})
	if err != nil {
		return err
	}
	return nil
}
