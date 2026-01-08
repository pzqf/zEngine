package zService

import (
	"errors"

	"github.com/pzqf/zEngine/zObject"
)

type Service interface {
	zObject.ManagedObject
}

type ServiceManager struct {
	zObject.ObjectManager
}

// NewServiceManager 创建一个新的服务管理器
func NewServiceManager() *ServiceManager {
	return &ServiceManager{
		ObjectManager: *zObject.NewObjectManager(),
	}
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
		err := value.(Service).Close()
		if err != nil {
			panic(err)
		}
		return true
	})
}

func (sm *ServiceManager) ServeServices() {
	sm.ObjectsRange(func(key, value interface{}) bool {
		go func(s Service) {
			s.Serve()
		}(value.(Service))
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
