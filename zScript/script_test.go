package zScript

import (
	"go/token"
	"sync"
	"testing"
)

// 本文件此前是一个**永不退出的演示循环**（for{ Update; sleep 1s }），既没有断言、也不可能通过
// ——只会一路跑到测试超时（在修好崩溃前，它靠 2 秒后的空指针崩溃"提前结束"）。
// 现改为有界 + 有断言的真测试，并为修掉的两个真缺陷各留一条回归守护：
//   1. binaryExprEval 缺 bool 分支 → 脚本里的 &&/|| 一律求值为 nil（本包自带 test.json 满是这两个运算符）；
//   2. ScriptHolder.Update 用 reflect.TypeOf(ret).String() 判类型 → ret 为 nil 时空指针崩溃。

// 脚本函数全局注册表不可重复注册（RegisterScriptFunc 撞名直接 panic），
// 故整个包的测试共用一次注册。
var registerOnce sync.Once

// 测试脚本函数的调用计数，用于断言"图真的驱动了函数调用"。
var (
	callMu    sync.Mutex
	callCount = map[string]int{}
)

func recordCall(name string) {
	callMu.Lock()
	callCount[name]++
	callMu.Unlock()
}

func callsOf(name string) int {
	callMu.Lock()
	defer callMu.Unlock()
	return callCount[name]
}

func setupScript(t *testing.T) *ScriptHolder {
	t.Helper()

	// BindScript 内部会在未加载时自行 LoadScriptFile；重复 LoadScriptFile 会报 already loaded，
	// 所以这里不要额外加载（同包多个用例共用同一份脚本数据）。
	registerOnce.Do(RegisterFunc)

	holder := &ScriptHolder{}
	if err := holder.BindScript("test.json"); err != nil {
		t.Fatalf("绑定脚本失败: %v", err)
	}
	return holder
}

// TestScript_DrivesGraphWithoutPanic 有界地驱动脚本图：不崩溃、状态机真的从入口推进、
// 节点上的动作函数真的被调用。
func TestScript_DrivesGraphWithoutPanic(t *testing.T) {
	holder := setupScript(t)

	entry := holder.GetCurrentNodeId()
	if entry == "" {
		t.Fatalf("绑定后应停在入口节点")
	}

	// 图：n1(Entry) -[IsMoveInControl()]-> n3(StartTimer) -[无条件]-> n2(MoveToTarget) -...
	// 跑有限拍即可走完这几步；此前这里是死循环。
	for i := 0; i < 20; i++ {
		holder.Update(10)
	}

	if holder.GetCurrentNodeId() == entry {
		t.Fatalf("跑了 20 拍仍停在入口节点 %s，状态机没有推进", entry)
	}
	if callsOf("StartTimer") == 0 {
		t.Fatalf("n3 的 StartTimer 应被调用过")
	}
	if callsOf("MoveToTarget") == 0 {
		t.Fatalf("n2 的 MoveToTarget 应被调用过")
	}
}

// TestScript_CompoundConditionEdgeDoesNotPanic 回归守护（缺陷 2）：
// 复合条件边（!IsDead() || IsTargetDead() || OutOfTrackingRange()）求值结果可能不是布尔，
// Update 必须把它当作"这条边不通"，而不是崩溃。
// 注意 OutOfTrackingRange 故意**未注册**——脚本引用不存在的函数是常态，不能因此炸掉整个循环。
func TestScript_CompoundConditionEdgeDoesNotPanic(t *testing.T) {
	holder := setupScript(t)

	defer func() {
		if r := recover(); r != nil {
			t.Fatalf("条件边求值不得崩溃（修前：reflect.TypeOf(nil).String() 空指针）: %v", r)
		}
	}()

	// 推进到 n2——它出边全是复合条件，正是修前崩溃的位置。
	for i := 0; i < 30; i++ {
		holder.Update(10)
	}
	if callsOf("IsDead") == 0 {
		t.Fatalf("应已求值到含 !IsDead() 的复合条件边")
	}
}

// TestBinaryExprEval_Bool 回归守护（缺陷 1）：布尔二元运算必须真的算出结果。
// 修前 binaryExprEval 没有 bool 分支，&&/|| 一律返回 nil，脚本里所有复合条件边都走不通。
func TestBinaryExprEval_Bool(t *testing.T) {
	cases := []struct {
		x, y interface{}
		op   token.Token
		want interface{}
	}{
		{true, true, token.LAND, true},
		{true, false, token.LAND, false},
		{false, true, token.LOR, true},
		{false, false, token.LOR, false},
		{true, true, token.EQL, true},
		{true, false, token.NEQ, true},
	}
	for _, c := range cases {
		got := binaryExprEval(c.x, c.y, c.op)
		if got != c.want {
			t.Fatalf("binaryExprEval(%v, %v, %s) = %v, want %v", c.x, c.y, c.op, got, c.want)
		}
	}

	// 类型不匹配仍应返回 nil（而不是瞎猜一个值）。
	if got := binaryExprEval(true, "x", token.LAND); got != nil {
		t.Fatalf("bool && string 应为 nil, got %v", got)
	}
}

// TestExpressionEval_ShortCircuit 逻辑运算短路：右侧的游戏函数在左侧已定胜负时不该被调用
//（右侧多半会打到游戏世界，白调既浪费也可能有副作用）。
func TestExpressionEval_ShortCircuit(t *testing.T) {
	registerOnce.Do(RegisterFunc)

	before := callsOf("CastSpell")

	// IsDead() 恒 true → || 左侧已为真，右侧 IsAttackInRange() 不应被求值。
	beforeRange := callsOf("IsAttackInRange")
	if got := evalExprString(t, "IsDead() || IsAttackInRange()"); got != true {
		t.Fatalf("IsDead() || ... 应为 true, got %v", got)
	}
	if callsOf("IsAttackInRange") != beforeRange {
		t.Fatalf("|| 左侧为真时不应再求值右侧")
	}

	// ShouldRefreshMoveToTarget() 恒 false → && 左侧已为假，右侧不应被求值。
	beforeRange = callsOf("IsAttackInRange")
	if got := evalExprString(t, "ShouldRefreshMoveToTarget() && IsAttackInRange()"); got != false {
		t.Fatalf("false && ... 应为 false, got %v", got)
	}
	if callsOf("IsAttackInRange") != beforeRange {
		t.Fatalf("&& 左侧为假时不应再求值右侧")
	}

	if callsOf("CastSpell") != before {
		t.Fatalf("本用例不应触发 CastSpell")
	}
}

// evalExprString 把一段表达式源码解析成语句并求值（复用脚本自身的解析路径 astParser）。
func evalExprString(t *testing.T, src string) interface{} {
	t.Helper()
	stmt := astParser(src)
	if stmt == nil {
		t.Fatalf("解析表达式 %q 失败", src)
	}
	return expressionEval(&ScriptHolder{}, stmt.X)
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
	recordCall("MoveToTarget")
	return nil
}

func CastSpell(holder *ScriptHolder, args ...interface{}) interface{} {
	recordCall("CastSpell")
	return nil
}

func IsMoveInControl(holder *ScriptHolder, args ...interface{}) interface{} {
	recordCall("IsMoveInControl")
	return true
}

func ShouldRefreshMoveToTarget(holder *ScriptHolder, args ...interface{}) interface{} {
	recordCall("ShouldRefreshMoveToTarget")
	return false
}

func IsAttackInRange(holder *ScriptHolder, args ...interface{}) interface{} {
	recordCall("IsAttackInRange")
	return true
}

func StartTimer(holder *ScriptHolder, args ...interface{}) interface{} {
	recordCall("StartTimer")
	return true
}

func IsTargetDead(holder *ScriptHolder, args ...interface{}) interface{} {
	recordCall("IsTargetDead")
	return false
}

func IsDead(holder *ScriptHolder, args ...interface{}) interface{} {
	recordCall("IsDead")
	return true
}
