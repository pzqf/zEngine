package zNet

//type HandlerFun func(session Session, netPacket *NetPacket) error

//var dispatcherHandler HandlerFun
//var defaultPoolSize = 10000
//var workerPool *ants.Pool

/*
func RegisterHandler(fun HandlerFun, n int) error {
	defaultPoolSize = n
	if defaultPoolSize <= 100 {
		defaultPoolSize = 10000
	}

	if workerPool == nil {
		p, err := ants.NewPool(defaultPoolSize)
		if err != nil {
			panic(err)
		}
		workerPool = p
	}

	dispatcherHandler = fun

	return nil
}


func Dispatcher(session Session, netPacket *NetPacket) error {
	if netPacket == nil {
		return errors.New("nil packet")
	}

	if dispatcherHandler != nil {
		err := workerPool.Submit(func() {
			dispatcherHandler(session, netPacket)
		})
		if err != nil {
			return err
		}

	}
	return nil
}
*/
