package zObject

import (
	"errors"
	"reflect"

	"github.com/pzqf/zUtil/zMap"
)

type ManagedObject interface {
	GetId() interface{}
	Init() error
	Close() error
	Serve()
}

type ObjectManager struct {
	objects zMap.Map
}

// NewObjectManager 创建一个新的对象管理器
func NewObjectManager() *ObjectManager {
	return &ObjectManager{
		objects: *zMap.NewMap(),
	}
}

func (om *ObjectManager) AddObject(key interface{}, obj ManagedObject) error {
	if reflect.ValueOf(obj).Kind() != reflect.Ptr {
		return errors.New("object must point")
	}
	if obj == nil {
		return errors.New("object can't be nil")
	}
	_, ok := om.objects.Get(key)
	if ok {
		return errors.New("object had exist")
	}

	om.objects.Store(key, obj)

	return nil
}

func (om *ObjectManager) GetObject(key interface{}) (interface{}, error) {
	v, ok := om.objects.Get(key)
	if !ok {
		return nil, errors.New("object not exist")
	}
	return v, nil
}

func (om *ObjectManager) RemoveObject(key interface{}) error {
	_, ok := om.objects.Get(key)
	if !ok {
		return errors.New("object not exist")
	}

	om.objects.Delete(key)

	return nil
}

func (om *ObjectManager) ClearAllObject() {
	om.objects.Range(func(key, value interface{}) bool {
		om.objects.Delete(key)
		return true
	})
}

func (om *ObjectManager) ObjectsRange(f func(key, value interface{}) bool) {
	om.objects.Range(f)
}

func (om *ObjectManager) GetAllObject() []Object {
	objs := make([]Object, 0)
	om.objects.Range(func(key, value interface{}) bool {
		objs = append(objs, value.(Object))
		return true
	})
	return objs
}

func (om *ObjectManager) GetObjectsCount() int64 {
	return om.objects.Len()
}
