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
	"runtime"
	"strconv"
	"time"

	"github.com/pzqf/zEngine/zLog"
)

var (
	ErrParseFailed           = errors.New("parse failed")
	ErrTypeMismatched        = errors.New("type mismatched")
	ErrOperationNotSupported = errors.New("operation not supported")
	ErrDivisionByZero        = errors.New("division by zero")
	ErrFunctionNotFound      = errors.New("function not found")
	ErrInvalidArgument       = errors.New("invalid argument")

	logger zLog.Logger
)

func GetVersion() string {
	return "2.0.0"
}

func SetLogger(l zLog.Logger) {
	logger = l
}

func astParser(str string) *ast.ExprStmt {
	if str == "" {
		return nil
	}

	src := `
	package xxx
	func Main() {
		%s
	}`
	src = fmt.Sprintf(src, str)

	fSet := token.NewFileSet()
	f, err := parser.ParseFile(fSet, "", src, 0)
	if err != nil {
		return nil
	}

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

	exprStmt, ok := funcDecl.Body.List[0].(*ast.ExprStmt)
	if !ok {
		return nil
	}

	if logger != nil {
		logger.Debug("Expression parsed successfully: %q", str)
	}
	return exprStmt
}

func expressionEval(holder *ScriptHolder, e interface{}) interface{} {
	if e == nil {
		return nil
	}

	switch e.(type) {
	case *ast.Ident:
		t := e.(*ast.Ident)
		if t.Name == "true" {
			return true
		} else {
			return false
		}
	case *ast.BasicLit:
		lit := e.(*ast.BasicLit)
		switch lit.Kind {
		case token.STRING:
			return lit.Value
		case token.INT:
			v, err := strconv.Atoi(lit.Value)
			if err != nil {
				_ = err
				return 0
			}
			return v
		case token.FLOAT:
			v, err := strconv.ParseFloat(lit.Value, 64)
			if err != nil {
				_ = err
				return 0.0
			}
			return v
		case token.CHAR:
			if len(lit.Value) > 0 {
				return lit.Value[0]
			} else {
				return 0
			}
		default:
			return nil
		}
	case *ast.CompositeLit:
		return nil
	case *ast.ParenExpr:
		parenExpr, ok := e.(*ast.ParenExpr)
		if !ok {
			return nil
		}
		return expressionEval(holder, parenExpr.X)
	case *ast.CallExpr:
		callExpr, ok := e.(*ast.CallExpr)
		if !ok {
			return nil
		}

		funcIdent, ok := callExpr.Fun.(*ast.Ident)
		if !ok {
			return nil
		}

		funcName := funcIdent.Name

		var funArgList []interface{}
		for _, arg := range callExpr.Args {
			argValue := expressionEval(holder, arg)
			funArgList = append(funArgList, argValue)
		}

		result := functionCall(holder, funcName, funArgList)
		return result
	case *ast.UnaryExpr:
		ue, ok := e.(*ast.UnaryExpr)
		if !ok {
			return nil
		}

		if ue.Op == token.NOT {
			ret := expressionEval(holder, ue.X)
			if ret != nil && reflect.TypeOf(ret).Name() == "bool" {
				return !ret.(bool)
			} else {
				return nil
			}
		} else {
			return nil
		}
	case *ast.BinaryExpr:
		be, ok := e.(*ast.BinaryExpr)
		if !ok {
			return nil
		}

		x := expressionEval(holder, be.X)
		y := expressionEval(holder, be.Y)
		result := binaryExprEval(x, y, be.Op)
		return result
	case *ast.ExprStmt:
		es, ok := e.(*ast.ExprStmt)
		if !ok {
			return nil
		}
		return expressionEval(holder, es.X)
	default:
		return nil
	}
}

func functionCall(holder *ScriptHolder, funcName string, args []interface{}) interface{} {
	function, err := GetScriptFunc(funcName)
	if err != nil {
		if logger != nil {
			logger.Error("Function not found: %s - %v", funcName, err)
		}
		return false
	}

	start := time.Now()
	result := function(holder, args...)
	elapsed := time.Since(start)

	if logger != nil {
		logger.Debug("Function %s completed in %s, result: %T = %#v",
			funcName, elapsed, result, result)
	}

	return result
}

func binaryExprEval(x, y interface{}, op token.Token) interface{} {
	if x == nil || y == nil {
		return nil
	}

	switch reflect.TypeOf(x).String() {
	case "string":
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

func getCallerInfo() (funcName, file string, line int) {
	pc, file, line, ok := runtime.Caller(2)
	if !ok {
		return "", "", 0
	}
	funcName = runtime.FuncForPC(pc).Name()
	return funcName, file, line
}

func SetOutput(out io.Writer) {
	log.SetOutput(out)
}

func GetDebugLevel() int {
	return 0
}
