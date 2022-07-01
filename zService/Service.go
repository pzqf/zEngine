package zService

import (
	"errors"

	"github.com/pzqf/zEngine/zObject"
)

type Service interface {
	zObject.Object
	Init() error
	Close() error
	Serve()
}

type ServiceManager struct {
	zObject.ObjectManager
}

func (sm *ServiceManager) InitServices() {
	sm.ObjectsRange(func(key, value interface{}) bool {
		err := value.(Service).Init()
		if err != nil {
			panic(err)
		}
		return true
	})
}

func (sm *ServiceManager) CloseServices() {
	sm.ObjectsRange(func(key, value interface{}) bool {
		_ = value.(Service).Close()
		return true
	})
	sm.ClearAllObject()
}

func (sm *ServiceManager) ServeServices() {
	sm.ObjectsRange(func(key, value interface{}) bool {
		value.(Service).Serve()
		return true
	})
}

func (sm *ServiceManager) AddService(s Service) error {
	if s.GetId() == nil {
		return errors.New("service must had id")
	}
	_ = sm.AddObject(s.GetId(), s)
	return nil
}

func (sm *ServiceManager) GetService(id interface{}) (Service, error) {
	object, err := sm.GetObject(id)
	if err != nil {
		return nil, err
	}

	return object.(Service), nil
}
