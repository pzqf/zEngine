package zObject

import (
	"fmt"
	"reflect"
	"testing"
)

type MyObject struct {
	Object
	Name string
}

type MyObjectMgr struct {
	ObjectManager
}

func Test(t *testing.T) {
	mgr := MyObjectMgr{}
	obj := &MyObject{
		Name: "abc",
	}
	id := "aaa"
	obj.SetId(id)
	fmt.Println("typeof", reflect.TypeOf(obj))
	err := mgr.AddObject(obj.GetId(), obj)
	if err != nil {
		fmt.Println(err)
		return
	}
	fmt.Println(mgr.objects.Len())
	list := mgr.GetAllObject()
	for i := 0; i < len(list); i++ {
		fmt.Printf("list:%#v \n", list[i])
	}

	o, err := mgr.GetObject(id)
	if err != nil {
		fmt.Println(err)
		return
	}
	fmt.Println(o)

	//err = mgr.RemoveObject(obj.GetId())
	//if err != nil {
	//	return
	//}

	fmt.Println(mgr.objects.Len())

	obj2 := &MyObject{
		Name: "dec",
	}
	obj2.SetId("bbb")
	err = mgr.AddObject(obj2.GetId(), obj2)
	if err != nil {
		fmt.Println(err)
		return
	}

	fmt.Println(mgr.objects.Len())

	mgr.Range(func(key, value interface{}) bool {
		fmt.Println(key, value)
		return true
	})

	mgr.ClearAllObject()
	mgr.Range(func(key, value interface{}) bool {
		fmt.Println(key, value)
		return true
	})
	fmt.Println(mgr.objects.Len())
}
