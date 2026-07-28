package zScript

import (
	"errors"
	"fmt"
	"go/ast"
	"go/parser"
	"go/token"
	"io"
	"log"
	"reflect"
	"strconv"
	"time"

	"github.com/pzqf/zEngine/zLog"
)

// 脚本引擎错误定义
var (
	ErrParseFailed           = errors.New("parse failed")            // 解析失败
	ErrTypeMismatched        = errors.New("type mismatched")         // 类型不匹配
	ErrOperationNotSupported = errors.New("operation not supported") // 操作不支持
	ErrDivisionByZero        = errors.New("division by zero")        // 除零错误
	ErrFunctionNotFound      = errors.New("function not found")      // 函数未找到
	ErrInvalidArgument       = errors.New("invalid argument")        // 无效参数

	logger zLog.Logger // 日志记录器
)

// GetVersion 获取脚本引擎版本
// 返回: 版本号字符串
func GetVersion() string {
	return "2.0.0"
}

// SetLogger 设置日志记录器
// 参数:
//   - l: 实现zLog.Logger接口的日志记录器
func SetLogger(l zLog.Logger) {
	logger = l
}

// astParser 解析表达式字符串为AST语句
// 将表达式字符串包装成Go函数，然后使用Go的AST解析器解析
// 参数:
//   - str: 表达式字符串
//
// 返回:
//   - *ast.ExprStmt: 解析后的表达式语句AST节点，失败返回nil
func astParser(str string) *ast.ExprStmt {
	if str == "" {
		return nil
	}

	// 包装表达式为合法的Go函数语法
	src := `
	package xxx
	func Main() {
		%s
	}`
	src = fmt.Sprintf(src, str)

	// 解析Go源代码
	fSet := token.NewFileSet()
	f, err := parser.ParseFile(fSet, "", src, 0)
	if err != nil {
		return nil
	}

	// 验证解析结果结构
	if f == nil || len(f.Decls) == 0 {
		return nil
	}

	funcDecl, ok := f.Decls[0].(*ast.FuncDecl)
	if !ok {
		return nil
	}

	if funcDecl.Body == nil || len(funcDecl.Body.List) == 0 {
		return nil
	}

	// 提取第一条表达式语句
	exprStmt, ok := funcDecl.Body.List[0].(*ast.ExprStmt)
	if !ok {
		return nil
	}

	if logger != nil {
		logger.Debug("Expression parsed successfully: %q", str)
	}
	return exprStmt
}

// expressionEval 表达式求值
// 递归评估AST节点，支持标识符、字面量、括号表达式、函数调用、一元表达式、二元表达式
// 参数:
//   - holder: 脚本持有者，包含上下文信息
//   - e: AST节点
//
// 返回:
//   - interface{}: 求值结果
func expressionEval(holder *ScriptHolder, e interface{}) interface{} {
	if e == nil {
		return nil
	}

	switch e := e.(type) {
	case *ast.Ident:
		// 标识符求值（布尔常量）
		if e.Name == "true" {
			return true
		}
		return false
	case *ast.BasicLit:
		// 基本字面量求值
		switch e.Kind {
		case token.STRING:
			return e.Value
		case token.INT:
			v, err := strconv.Atoi(e.Value)
			if err != nil {
				_ = err
				return 0
			}
			return v
		case token.FLOAT:
			v, err := strconv.ParseFloat(e.Value, 64)
			if err != nil {
				_ = err
				return 0.0
			}
			return v
		case token.CHAR:
			if len(e.Value) > 0 {
				return e.Value[0]
			}
			return 0
		default:
			return nil
		}
	case *ast.CompositeLit:
		// 复合字面量（暂不支持）
		return nil
	case *ast.ParenExpr:
		// 括号表达式：递归求值内部表达式
		return expressionEval(holder, e.X)
	case *ast.CallExpr:
		// 函数调用表达式
		funcIdent, ok := e.Fun.(*ast.Ident)
		if !ok {
			return nil
		}

		funcName := funcIdent.Name

		// 求值参数列表
		var funArgList []interface{}
		for _, arg := range e.Args {
			argValue := expressionEval(holder, arg)
			funArgList = append(funArgList, argValue)
		}

		// 执行函数调用
		result := functionCall(holder, funcName, funArgList)
		return result
	case *ast.UnaryExpr:
		// 一元表达式（仅支持逻辑非!）
		if e.Op == token.NOT {
			ret := expressionEval(holder, e.X)
			if ret != nil && reflect.TypeOf(ret).Name() == "bool" {
				return !ret.(bool)
			}
			return nil
		}
		return nil
	case *ast.BinaryExpr:
		// 逻辑与/或**短路求值**：右侧多半是 IsCastingSpell() 这类会打到游戏世界的函数，
		// 左侧已定胜负时不该再调它（既浪费也可能有副作用），语义也与 Go 保持一致。
		if e.Op == token.LAND || e.Op == token.LOR {
			x := expressionEval(holder, e.X)
			xb, ok := x.(bool)
			if !ok {
				return nil
			}
			if e.Op == token.LAND && !xb {
				return false
			}
			if e.Op == token.LOR && xb {
				return true
			}
			y := expressionEval(holder, e.Y)
			yb, ok := y.(bool)
			if !ok {
				return nil
			}
			return yb
		}

		// 其余二元运算：先求值左右操作数，再执行运算
		x := expressionEval(holder, e.X)
		y := expressionEval(holder, e.Y)
		result := binaryExprEval(x, y, e.Op)
		return result
	case *ast.ExprStmt:
		// 表达式语句：递归求值内部表达式
		return expressionEval(holder, e.X)
	default:
		return nil
	}
}

// functionCall 执行脚本函数调用
// 从注册表查找函数并执行，记录执行耗时
// 参数:
//   - holder: 脚本持有者
//   - funcName: 函数名称
//   - args: 函数参数列表
//
// 返回:
//   - interface{}: 函数执行结果
func functionCall(holder *ScriptHolder, funcName string, args []interface{}) interface{} {
	function, err := GetScriptFunc(funcName)
	if err != nil {
		if logger != nil {
			logger.Error("Function not found: %s - %v", funcName, err)
		}
		return false
	}

	// 计时并执行函数
	start := time.Now()
	result := function(holder, args...)
	elapsed := time.Since(start)

	if logger != nil {
		logger.Debug("Function %s completed in %s, result: %T = %#v",
			funcName, elapsed, result, result)
	}

	return result
}

// binaryExprEval 二元表达式求值
// 根据操作数类型和运算符执行相应的运算
// 支持的类型: bool, string, int, float64, uint8
// 支持的运算符: &&, ||, +, -, *, /, %, &, |, ^, <<, >>, &^, ==, !=, <, >, <=, >=
// 参数:
//   - x: 左操作数
//   - y: 右操作数
//   - op: 运算符
//
// 返回:
//   - interface{}: 运算结果
func binaryExprEval(x, y interface{}, op token.Token) interface{} {
	if x == nil || y == nil {
		return nil
	}

	switch reflect.TypeOf(x).String() {
	case "bool":
		// 布尔类型运算。**必须有这一支**：条件边几乎全是 IsA() && !IsB() 这类布尔表达式
		// （本包自带的 test.json 就满是 &&/||）。缺了它这些表达式一律求值为 nil，
		// 于是所有复合条件边都走不通，且 nil 会一路传到调用方引发崩溃。
		// 注：&&/|| 的短路在 expressionEval 里做（避免白调右侧的游戏函数），这里兜非短路调用方。
		yb, ok := y.(bool)
		if !ok {
			return nil
		}
		switch op {
		case token.LAND:
			return x.(bool) && yb
		case token.LOR:
			return x.(bool) || yb
		case token.EQL:
			return x.(bool) == yb
		case token.NEQ:
			return x.(bool) != yb
		}
		return nil
	case "string":
		// 字符串类型运算
		switch op {
		case token.ADD:
			if reflect.TypeOf(y).String() == "string" {
				return x.(string) + y.(string)
			}
		case token.EQL:
			if reflect.TypeOf(y).String() == "string" {
				return x.(string) == y.(string)
			}
		case token.LSS:
			if reflect.TypeOf(y).String() == "string" {
				return x.(string) < y.(string)
			}
		case token.LEQ:
			if reflect.TypeOf(y).String() == "string" {
				return x.(string) <= y.(string)
			}
		case token.GTR:
			if reflect.TypeOf(y).String() == "string" {
				return x.(string) > y.(string)
			}
		case token.GEQ:
			if reflect.TypeOf(y).String() == "string" {
				return x.(string) >= y.(string)
			}
		case token.NEQ:
			if reflect.TypeOf(y).String() == "string" {
				return x.(string) != y.(string)
			}
		}
		return nil
	case "int":
		// 整数类型运算
		switch op {
		case token.ADD:
			switch reflect.TypeOf(y).String() {
			case "int":
				return x.(int) + y.(int)
			case "float64":
				return float64(x.(int)) + y.(float64)
			case "uint8":
				return x.(int) + int(y.(uint8))
			}
		case token.SUB:
			switch reflect.TypeOf(y).String() {
			case "int":
				return x.(int) - y.(int)
			case "float64":
				return float64(x.(int)) - y.(float64)
			case "uint8":
				return x.(int) - int(y.(uint8))
			}
		case token.MUL:
			switch reflect.TypeOf(y).String() {
			case "int":
				return x.(int) * y.(int)
			case "float64":
				return float64(x.(int)) * y.(float64)
			case "uint8":
				return x.(int) * int(y.(uint8))
			}
		case token.QUO:
			switch reflect.TypeOf(y).String() {
			case "int":
				if y.(int) == 0 {
					return nil
				}
				return x.(int) / y.(int)
			case "float64":
				if y.(float64) == 0 {
					return nil
				}
				return float64(x.(int)) / y.(float64)
			case "uint8":
				if y.(uint8) == 0 {
					return nil
				}
				return x.(int) / int(y.(uint8))
			}
		case token.REM:
			switch reflect.TypeOf(y).String() {
			case "int":
				return x.(int) % y.(int)
			case "uint8":
				return x.(int) % int(y.(uint8))
			}
		case token.AND:
			switch reflect.TypeOf(y).String() {
			case "int":
				return x.(int) & y.(int)
			case "uint8":
				return x.(int) & int(y.(uint8))
			}
		case token.OR:
			switch reflect.TypeOf(y).String() {
			case "int":
				return x.(int) | y.(int)
			case "uint8":
				return x.(int) | int(y.(uint8))
			}
		case token.XOR:
			switch reflect.TypeOf(y).String() {
			case "int":
				return x.(int) ^ y.(int)
			case "uint8":
				return x.(int) ^ int(y.(uint8))
			}
		case token.SHL:
			switch reflect.TypeOf(y).String() {
			case "int":
				return x.(int) << y.(int)
			case "uint8":
				return x.(int) << int(y.(uint8))
			}
		case token.SHR:
			switch reflect.TypeOf(y).String() {
			case "int":
				return x.(int) >> y.(int)
			case "uint8":
				return x.(int) >> int(y.(uint8))
			}
		case token.AND_NOT:
			switch reflect.TypeOf(y).String() {
			case "int":
				return x.(int) &^ y.(int)
			case "uint8":
				return x.(int) &^ int(y.(uint8))
			}
		case token.EQL:
			switch reflect.TypeOf(y).String() {
			case "int":
				return x.(int) == y.(int)
			case "float64":
				return float64(x.(int)) == y.(float64)
			case "uint8":
				return x.(int) == int(y.(uint8))
			}
		case token.LSS:
			switch reflect.TypeOf(y).String() {
			case "int":
				return x.(int) < y.(int)
			case "float64":
				return float64(x.(int)) < y.(float64)
			case "uint8":
				return x.(int) < int(y.(uint8))
			}
		case token.GTR:
			switch reflect.TypeOf(y).String() {
			case "int":
				return x.(int) > y.(int)
			case "float64":
				return float64(x.(int)) > y.(float64)
			case "uint8":
				return x.(int) > int(y.(uint8))
			}
		case token.NEQ:
			switch reflect.TypeOf(y).String() {
			case "int":
				return x.(int) != y.(int)
			case "float64":
				return float64(x.(int)) != y.(float64)
			case "uint8":
				return x.(int) != int(y.(uint8))
			}
		case token.LEQ:
			switch reflect.TypeOf(y).String() {
			case "int":
				return x.(int) <= y.(int)
			case "float64":
				return float64(x.(int)) <= y.(float64)
			case "uint8":
				return x.(int) <= int(y.(uint8))
			}
		case token.GEQ:
			switch reflect.TypeOf(y).String() {
			case "int":
				return x.(int) >= y.(int)
			case "float64":
				return float64(x.(int)) >= y.(float64)
			case "uint8":
				return x.(int) >= int(y.(uint8))
			}
		}
		return nil
	case "float64":
		// 浮点数类型运算
		switch op {
		case token.ADD:
			switch reflect.TypeOf(y).String() {
			case "int":
				return x.(float64) + float64(y.(int))
			case "float64":
				return x.(float64) + y.(float64)
			case "uint8":
				return x.(float64) + float64(y.(uint8))
			}
		case token.SUB:
			switch reflect.TypeOf(y).String() {
			case "int":
				return x.(float64) - float64(y.(int))
			case "float64":
				return x.(float64) - y.(float64)
			case "uint8":
				return x.(float64) - float64(y.(uint8))
			}
		case token.MUL:
			switch reflect.TypeOf(y).String() {
			case "int":
				return x.(float64) * float64(y.(int))
			case "float64":
				return x.(float64) * y.(float64)
			case "uint8":
				return x.(float64) * float64(y.(uint8))
			}
		case token.QUO:
			switch reflect.TypeOf(y).String() {
			case "int":
				if y.(int) == 0 {
					return nil
				}
				return x.(float64) / float64(y.(int))
			case "float64":
				if y.(float64) == 0 {
					return nil
				}
				return x.(float64) / y.(float64)
			case "uint8":
				if y.(uint8) == 0 {
					return nil
				}
				return x.(float64) / float64(y.(uint8))
			}
		case token.EQL:
			switch reflect.TypeOf(y).String() {
			case "int":
				return x.(float64) == float64(y.(int))
			case "float64":
				return x.(float64) == y.(float64)
			case "uint8":
				return x.(float64) == float64(y.(uint8))
			}
		case token.LSS:
			switch reflect.TypeOf(y).String() {
			case "int":
				return x.(float64) < float64(y.(int))
			case "float64":
				return x.(float64) < y.(float64)
			case "uint8":
				return x.(float64) < float64(y.(uint8))
			}
		case token.GTR:
			switch reflect.TypeOf(y).String() {
			case "int":
				return x.(float64) > float64(y.(int))
			case "float64":
				return x.(float64) > y.(float64)
			case "uint8":
				return x.(float64) > float64(y.(uint8))
			}
		case token.NEQ:
			switch reflect.TypeOf(y).String() {
			case "int":
				return x.(float64) != float64(y.(int))
			case "float64":
				return x.(float64) != y.(float64)
			}
		}
		return nil
	default:
		return nil
	}
}

// SetOutput 设置日志输出
// 参数:
//   - out: 输出写入器
func SetOutput(out io.Writer) {
	log.SetOutput(out)
}

// GetDebugLevel 获取调试级别
// 返回:
//   - 调试级别（固定返回0）
func GetDebugLevel() int {
	return 0
}
