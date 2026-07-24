package zService

import (
	"fmt"
	"testing"
)

type TestService struct {
	*BaseService
	Name string
}

func NewTestService(serviceId string) *TestService {
	bs := NewBaseService(serviceId)
	// 内嵌指针，避免拷贝含 atomic 的 BaseService（go vet: copylocks）
	a := &TestService{BaseService: bs}
	return a
}

func (ts *TestService) Init() error {

	return nil
}

func (ts *TestService) Close() error {

	return nil
}

func (ts *TestService) Serve() {

}

func Test(t *testing.T) {
	sm := NewServiceManager()
	ts := NewTestService("test_server")
	if err := sm.AddService(ts); err != nil {
		fmt.Println("add service TestService failed ", err)
		return
	}

	ts.Name = "lalalalala"
	fmt.Printf("%#v\n", ts)

	service, err := sm.GetService("test_server")
	if err != nil {
		return
	}
	fmt.Printf("%#v\n", service)

	service.(*TestService).Name = "dddddddd"
	fmt.Printf("%#v\n", service)

	service, err = sm.GetService("test_server")
	if err != nil {
		return
	}
	fmt.Printf("%#v\n", service)
}
