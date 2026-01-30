package zObject

import (
	"sync"
)

// ObjectPool 对象池接口
type ObjectPool interface {
	// Get 从池中获取一个对象
	Get() interface{}
	// Put 将对象放回池中
	Put(obj interface{})
	// Size 获取池的大小
	Size() int
}

// GenericPool 通用对象池实现
type GenericPool struct {
	mu      sync.Mutex
	objects []interface{}
	newFunc func() interface{}
	maxSize int
}

// NewGenericPool 创建一个新的通用对象池
func NewGenericPool(newFunc func() interface{}, maxSize int) *GenericPool {
	return &GenericPool{
		objects: make([]interface{}, 0, 10),
		newFunc: newFunc,
		maxSize: maxSize,
	}
}

// Get 从池中获取一个对象
func (p *GenericPool) Get() interface{} {
	p.mu.Lock()
	defer p.mu.Unlock()

	if len(p.objects) > 0 {
		obj := p.objects[len(p.objects)-1]
		p.objects = p.objects[:len(p.objects)-1]
		return obj
	}

	return p.newFunc()
}

// Put 将对象放回池中
func (p *GenericPool) Put(obj interface{}) {
	p.mu.Lock()
	defer p.mu.Unlock()

	if len(p.objects) < p.maxSize {
		p.objects = append(p.objects, obj)
	}
}

// Size 获取池的大小
func (p *GenericPool) Size() int {
	p.mu.Lock()
	defer p.mu.Unlock()

	return len(p.objects)
}
