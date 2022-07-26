package zScript

import (
	"errors"
	"fmt"
	"reflect"
	"runtime"
	"strings"
)

type ScriptFunc func(holder *ScriptHolder, args ...interface{}) interface{}

var funcList = make(map[string]ScriptFunc)

func RegisterScriptFunc(cf ScriptFunc) {
	funcName := runtime.FuncForPC(reflect.ValueOf(cf).Pointer()).Name()
	list := strings.Split(funcName, "/")
	funcName = list[len(list)-1]
	list = strings.Split(funcName, ".")

	funcName = list[len(list)-1]
	if _, ok := funcList[funcName]; ok {
		panic(fmt.Sprintf(`Bind script function:[%s] twice`, funcName))
	}

	funcList[funcName] = cf
}

func GetScriptFunc(funcName string) (ScriptFunc, error) {
	if _, ok := funcList[funcName]; !ok {
		return nil, errors.New(fmt.Sprintf("Can't find function [%s]", funcName))
	}
	return funcList[funcName], nil
}
