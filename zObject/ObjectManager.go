package zObject

import (
	"errors"
	"fmt"

	"github.com/pzqf/zUtil/zMap"
)

type ManagedObject interface {
	GetId() interface{}
	Init() error
	Close() error
	Serve()
}

type ObjectManager struct {
	objects *zMap.Map
}

func NewObjectManager() *ObjectManager {
	return &ObjectManager{
		objects: zMap.NewMap(),
	}
}

func (om *ObjectManager) AddObject(key interface{}, obj ManagedObject) error {
	if obj == nil {
		return errors.New("object can't be nil")
	}
	_, ok := om.objects.Load(key)
	if ok {
		return errors.New("object had exist")
	}

	om.objects.Store(key, obj)
	return nil
}

func (om *ObjectManager) GetObject(key interface{}) (interface{}, error) {
	v, ok := om.objects.Load(key)
	if !ok {
		return nil, errors.New("object not exist")
	}
	return v, nil
}

func (om *ObjectManager) RemoveObject(key interface{}) error {
	_, ok := om.objects.Load(key)
	if !ok {
		return errors.New("object not exist")
	}

	om.objects.Delete(key)
	return nil
}

func (om *ObjectManager) ClearAllObject() {
	om.objects.Clear()
}

func (om *ObjectManager) ObjectsRange(f func(key, value interface{}) bool) {
	om.objects.Range(f)
}

func (om *ObjectManager) GetAllObject() []ManagedObject {
	objs := make([]ManagedObject, 0)
	om.objects.Range(func(key, value interface{}) bool {
		objs = append(objs, value.(ManagedObject))
		return true
	})
	return objs
}

func (om *ObjectManager) GetObjectsCount() int64 {
	return om.objects.Len()
}

type TypedManagedObject[ID comparable, T any] interface {
	GetTypedId() ID
	Init() error
	Close() error
	Serve()
}

type TypedObjectManager[ID comparable, T any] struct {
	objects *zMap.TypedShardedMap[ID, T]
}

func NewTypedObjectManager[ID comparable, T any](shardCount int) *TypedObjectManager[ID, T] {
	if shardCount <= 0 {
		shardCount = 32
	}
	return &TypedObjectManager[ID, T]{
		objects: zMap.NewTypedShardedMap[ID, T](shardCount),
	}
}

func (m *TypedObjectManager[ID, T]) Add(id ID, obj T) error {
	_, ok := m.objects.Load(id)
	if ok {
		return fmt.Errorf("object with id %v already exists", id)
	}
	m.objects.Store(id, obj)
	return nil
}

func (m *TypedObjectManager[ID, T]) Get(id ID) (T, bool) {
	return m.objects.Load(id)
}

func (m *TypedObjectManager[ID, T]) Remove(id ID) error {
	_, ok := m.objects.Load(id)
	if !ok {
		return fmt.Errorf("object with id %v not found", id)
	}
	m.objects.Delete(id)
	return nil
}

func (m *TypedObjectManager[ID, T]) Range(f func(id ID, obj T) bool) {
	m.objects.Range(f)
}

func (m *TypedObjectManager[ID, T]) Count() int64 {
	return m.objects.Len()
}

func (m *TypedObjectManager[ID, T]) Clear() {
	m.objects.Clear()
}
