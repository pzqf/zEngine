package zObject

import (
	"errors"
	"reflect"

	"github.com/pzqf/zUtil/zMap"
)

/*
please store object point
*/

type ObjectManager struct {
	objects zMap.Map
}

func (om *ObjectManager) AddObject(key interface{}, obj interface{}) error {
	if reflect.ValueOf(obj).Kind() != reflect.Ptr {
		return errors.New("object must point")
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
	om.objects.Clear()
}

func (om *ObjectManager) ObjectsRange(f func(key, value interface{}) bool) {
	om.objects.Range(f)
}

func (om *ObjectManager) GetAllObject() []interface{} {
	var list []interface{}
	om.objects.Range(func(key, value interface{}) bool {
		list = append(list, value)
		return true
	})

	return list
}
func (om *ObjectManager) GetObjectsCount() int32 {
	return om.objects.Len()
}
