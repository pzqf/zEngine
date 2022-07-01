package zService

import (
	"fmt"
	"testing"

	"github.com/pzqf/zEngine/zObject"
)

type TestService struct {
	zObject.BaseObject
	Name string
}

func NewTestService(serviceId interface{}) *TestService {
	a := &TestService{}
	a.SetId(serviceId)
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
	sm := ServiceManager{}
	ts := NewTestService("test_server")
	if err := sm.AddService(ts); err != nil {
		fmt.Println("add service TestService failed ", err)
		return
	}

	ts.Name = "lalalalala"
	fmt.Println(fmt.Sprintf("%#v", ts))

	service, err := sm.GetService("test_server")
	if err != nil {
		return
	}
	fmt.Println(fmt.Sprintf("%#v", service))

	service.(*TestService).Name = "dddddddd"
	fmt.Println(fmt.Sprintf("%#v", service))

	service, err = sm.GetService("test_server")
	if err != nil {
		return
	}
	fmt.Println(fmt.Sprintf("%#v", service))
}
