package zScript

import (
	"fmt"
	"reflect"
	"runtime"
	"strings"
	"sync"
)

type ScriptFunc func(holder *ScriptHolder, args ...interface{}) interface{}

var (
	funcList   = make(map[string]ScriptFunc)
	funcListMu sync.RWMutex
)

func RegisterScriptFunc(cf ScriptFunc) {
	funcName := runtime.FuncForPC(reflect.ValueOf(cf).Pointer()).Name()

	list := strings.Split(funcName, "/")
	funcName = list[len(list)-1]
	list = strings.Split(funcName, ".")
	funcName = list[len(list)-1]

	funcListMu.Lock()
	defer funcListMu.Unlock()

	if _, ok := funcList[funcName]; ok {
		panic(fmt.Sprintf(`Bind script function:[%s] twice`, funcName))
	}

	funcList[funcName] = cf
}

func GetScriptFunc(funcName string) (ScriptFunc, error) {
	funcListMu.RLock()
	defer funcListMu.RUnlock()

	if f, ok := funcList[funcName]; ok {
		return f, nil
	}
	return nil, fmt.Errorf("can't find function [%s]", funcName)
}
