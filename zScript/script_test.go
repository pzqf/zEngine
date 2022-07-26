package zScript

import (
	"fmt"
	"testing"
	"time"

	"github.com/pzqf/zUtil/zTime"
)

func Test(t *testing.T) {

	scriptFile := `test.graphml`

	err := LoadScriptFile(scriptFile)
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

	holder := ScriptHolder{}
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
	RegisterScriptFunc(MoveToTarget)
	RegisterScriptFunc(CastSpell)
	RegisterScriptFunc(IsMoveInControl)
	RegisterScriptFunc(ShouldRefreshMoveToTarget)
	RegisterScriptFunc(IsAttackInRange)
	RegisterScriptFunc(StartTimer)
	RegisterScriptFunc(IsTargetDead)
	RegisterScriptFunc(IsDead)
}

func MoveToTarget(holder *ScriptHolder, args ...interface{}) interface{} {
	//objectIndex := args[0]
	//fmt.Println("call MoveToTarget:", objectIndex)
	return nil
}

func CastSpell(holder *ScriptHolder, args ...interface{}) interface{} {
	//objectIndex := args[0]
	//fmt.Println("call CastSpell:", objectIndex)
	return nil
}

func IsMoveInControl(holder *ScriptHolder, args ...interface{}) interface{} {
	//fmt.Println("call IsMoveInControl:", args)
	return true
}

func ShouldRefreshMoveToTarget(holder *ScriptHolder, args ...interface{}) interface{} {
	//fmt.Println("call ShouldRefreshMoveToTarget:")
	return false
}
func IsAttackInRange(holder *ScriptHolder, args ...interface{}) interface{} {
	//fmt.Println("call ShouldRefreshMoveToTarget:", args)
	return true
}

func StartTimer(holder *ScriptHolder, args ...interface{}) interface{} {
	//fmt.Println("call StartTimer:", len(args), args)
	return true
}

func IsTargetDead(holder *ScriptHolder, args ...interface{}) interface{} {
	//fmt.Println("call StartTimer:", len(args), args)
	return false
}
func IsDead(holder *ScriptHolder, args ...interface{}) interface{} {
	//fmt.Println("call StartTimer:", len(args), args)
	return true
}
