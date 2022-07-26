package main

import (
	"fmt"
	"time"

	"github.com/pzqf/zEngine/zScript"
	"github.com/pzqf/zUtil/zTime"
)

func main() {

	scriptFile := `.\zScript\test.graphml`

	err := zScript.LoadScriptFile(scriptFile)
	if err != nil {
		return
	}

	RegisterFunc()

	fmt.Println("========")

	//marshal, err := json.Marshal(scriptFileList)
	//if err != nil {
	//	return
	//}
	//fmt.Println(string(marshal))

	holder := zScript.ScriptHolder{}
	err = holder.BindScript(scriptFile)
	if err != nil {
		return
	}

	for {
		fmt.Println("----------------------------updated", zTime.Time2String(time.Now()))
		holder.Update(10)
		time.Sleep(time.Second * 1)
	}

}

func RegisterFunc() {
	zScript.RegisterScriptFunc(MoveToTarget)
	zScript.RegisterScriptFunc(CastSpell)
	zScript.RegisterScriptFunc(IsMoveInControl)
	zScript.RegisterScriptFunc(ShouldRefreshMoveToTarget)
	zScript.RegisterScriptFunc(IsAttackInRange)
	zScript.RegisterScriptFunc(StartTimer)
	zScript.RegisterScriptFunc(IsTargetDead)
	zScript.RegisterScriptFunc(IsDead)
}

func MoveToTarget(holder *zScript.ScriptHolder, args ...interface{}) interface{} {
	//objectIndex := args[0]
	//fmt.Println("call MoveToTarget:", objectIndex)
	return nil
}

func CastSpell(holder *zScript.ScriptHolder, args ...interface{}) interface{} {
	//objectIndex := args[0]
	//fmt.Println("call CastSpell:", objectIndex)
	return nil
}

func IsMoveInControl(holder *zScript.ScriptHolder, args ...interface{}) interface{} {
	//fmt.Println("call IsMoveInControl:", args)
	return true
}

func ShouldRefreshMoveToTarget(holder *zScript.ScriptHolder, args ...interface{}) interface{} {
	//fmt.Println("call ShouldRefreshMoveToTarget:")
	return false
}
func IsAttackInRange(holder *zScript.ScriptHolder, args ...interface{}) interface{} {
	//fmt.Println("call ShouldRefreshMoveToTarget:", args)
	return true
}

func StartTimer(holder *zScript.ScriptHolder, args ...interface{}) interface{} {
	//fmt.Println("call StartTimer:", len(args), args)
	return true
}

func IsTargetDead(holder *zScript.ScriptHolder, args ...interface{}) interface{} {
	//fmt.Println("call StartTimer:", len(args), args)
	return false
}
func IsDead(holder *zScript.ScriptHolder, args ...interface{}) interface{} {
	//fmt.Println("call StartTimer:", len(args), args)
	return true
}
