package zObject

type ObjectPool interface {
	Get() interface{}
	Put(obj interface{})
	Size() int
}

type GenericPool struct {
	ch      chan interface{}
	newFunc func() interface{}
}

func NewGenericPool(newFunc func() interface{}, maxSize int) *GenericPool {
	p := &GenericPool{
		ch:      make(chan interface{}, maxSize),
		newFunc: newFunc,
	}
	return p
}

func (p *GenericPool) Get() interface{} {
	select {
	case obj := <-p.ch:
		return obj
	default:
		return p.newFunc()
	}
}

func (p *GenericPool) Put(obj interface{}) {
	select {
	case p.ch <- obj:
	default:
	}
}

func (p *GenericPool) Size() int {
	return len(p.ch)
}

type TypedObjectPool[T any] struct {
	ch      chan T
	newFunc func() T
}

func NewTypedObjectPool[T any](newFunc func() T, maxSize int) *TypedObjectPool[T] {
	return &TypedObjectPool[T]{
		ch:      make(chan T, maxSize),
		newFunc: newFunc,
	}
}

func (p *TypedObjectPool[T]) Get() T {
	select {
	case obj := <-p.ch:
		return obj
	default:
		return p.newFunc()
	}
}

func (p *TypedObjectPool[T]) Put(obj T) {
	select {
	case p.ch <- obj:
	default:
	}
}

func (p *TypedObjectPool[T]) Size() int {
	return len(p.ch)
}
