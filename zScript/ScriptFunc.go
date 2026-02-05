package zScript

import (
	"fmt"
	"reflect"
	"runtime"
	"strings"
)

// ScriptFunc 脚本函数类型定义
// 参数:
//   - holder: 脚本持有者，包含上下文和状态信息
//   - args: 可变参数列表
//
// 返回:
//   - interface{}: 函数执行结果
type ScriptFunc func(holder *ScriptHolder, args ...interface{}) interface{}

// funcList 已注册的脚本函数映射
// key: 函数名, value: 脚本函数
var funcList = make(map[string]ScriptFunc)

// RegisterScriptFunc 注册脚本函数
// 通过反射获取函数名并注册到全局函数表
// 函数名会从完整路径中提取最后一段
// 参数:
//   - cf: 脚本函数
//
// 注意: 如果函数名已存在会触发panic
func RegisterScriptFunc(cf ScriptFunc) {
	// 通过反射获取函数的完整路径名
	funcName := runtime.FuncForPC(reflect.ValueOf(cf).Pointer()).Name()

	// 从完整路径中提取函数名（最后一段）
	list := strings.Split(funcName, "/")
	funcName = list[len(list)-1]
	list = strings.Split(funcName, ".")
	funcName = list[len(list)-1]

	// 检查是否重复注册
	if _, ok := funcList[funcName]; ok {
		panic(fmt.Sprintf(`Bind script function:[%s] twice`, funcName))
	}

	funcList[funcName] = cf
}

// GetScriptFunc 获取已注册的脚本函数
// 参数:
//   - funcName: 函数名称
//
// 返回:
//   - ScriptFunc: 脚本函数
//   - error: 未找到时返回错误
func GetScriptFunc(funcName string) (ScriptFunc, error) {
	if _, ok := funcList[funcName]; !ok {
		return nil, fmt.Errorf("Can't find function [%s]", funcName)
	}
	return funcList[funcName], nil
}
