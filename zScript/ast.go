package zScript

import (
	"fmt"
	"go/ast"
	"go/parser"
	"go/token"
	"log"
	"reflect"
	"strconv"
)

func astParser(str string) *ast.ExprStmt {
	log.Println("Parse:", str, ", len：", len(str))

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
		log.Printf("Parse error: %s", err)
		return nil
	}

	if f == nil || len(f.Decls) == 0 {
		log.Println("Parse result is nil or empty")
		return nil
	}

	funcDecl, ok := f.Decls[0].(*ast.FuncDecl)
	if !ok {
		log.Println("Failed to get FuncDecl")
		return nil
	}

	if funcDecl.Body == nil || len(funcDecl.Body.List) == 0 {
		log.Println("FuncDecl body is nil or empty")
		return nil
	}

	exprStmt, ok := funcDecl.Body.List[0].(*ast.ExprStmt)
	if !ok {
		log.Println("Failed to get ExprStmt")
		return nil
	}

	_ = ast.Print(fSet, exprStmt)

	return exprStmt
}

func expressionEval(holder *ScriptHolder, e interface{}) interface{} {
	if e == nil {
		log.Println("expressionEval: e is nil")
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
			v, _ := strconv.Atoi(lit.Value)
			return v
		case token.FLOAT:
			v, _ := strconv.ParseFloat(lit.Value, 64)
			return v
		case token.CHAR:
			if len(lit.Value) > 0 {
				return lit.Value[0]
			} else {
				log.Println("Empty char value")
				return 0
			}
		default:
			log.Println(fmt.Sprintf("This kind of operation:%s is not supported, in *ast.BasicLit", reflect.TypeOf(lit).String()))
		}
	case *ast.CompositeLit:
		log.Println("This kind of CompositeLit is not supported")
	case *ast.ParenExpr:
		if parenExpr, ok := e.(*ast.ParenExpr); ok {
			return expressionEval(holder, parenExpr.X)
		} else {
			log.Println("Failed to cast to *ast.ParenExpr")
			return nil
		}
	case *ast.CallExpr:
		if callExpr, ok := e.(*ast.CallExpr); ok {
			if funcIdent, ok := callExpr.Fun.(*ast.Ident); ok {
				funcName := funcIdent.Name
				var funArgList []interface{}
				for i := 0; i < len(callExpr.Args); i++ {
					funArgList = append(funArgList, expressionEval(holder, callExpr.Args[i]))
				}
				return functionCall(holder, funcName, funArgList)
			} else {
				log.Println("Failed to get function name from CallExpr")
				return nil
			}
		} else {
			log.Println("Failed to cast to *ast.CallExpr")
			return nil
		}
	case *ast.UnaryExpr:
		if ue, ok := e.(*ast.UnaryExpr); ok {
			if ue.Op == token.NOT {
				ret := expressionEval(holder, ue.X)
				if ret != nil && reflect.TypeOf(ret).Name() == "bool" {
					return !ret.(bool)
				} else {
					log.Println("This kind of unary operation is not supported")
				}
			} else {
				log.Println("This kind of unary operation is not supported," + ue.Op.String())
			}
		} else {
			log.Println("Failed to cast to *ast.UnaryExpr")
		}
	case *ast.BinaryExpr:
		if be, ok := e.(*ast.BinaryExpr); ok {
			x := expressionEval(holder, be.X)
			y := expressionEval(holder, be.Y)
			return binaryExprEval(x, y, be.Op)
		} else {
			log.Println("Failed to cast to *ast.BinaryExpr")
			return nil
		}
	case *ast.ExprStmt:
		if es, ok := e.(*ast.ExprStmt); ok {
			return expressionEval(holder, es.X)
		} else {
			log.Println("Failed to cast to *ast.ExprStmt")
			return nil
		}
	default:
		log.Println(fmt.Sprintf("This kind of operation:%s is not supported", reflect.TypeOf(e).String()))

		return nil
	}

	return nil
}

func functionCall(holder *ScriptHolder, funcName string, args []interface{}) interface{} {
	function, err := GetScriptFunc(funcName)
	if err != nil {
		return false
	}
	return function(holder, args...)
}

func binaryExprEval(x, y interface{}, op token.Token) interface{} {
	//fmt.Println(x)
	//fmt.Println(y)
	//fmt.Println(op)

	// 检查x或y是否为nil
	if x == nil || y == nil {
		log.Println("binaryExprEval: x or y is nil")
		return nil
	}

	errInfo := fmt.Sprintf("invalid operation: x %s y (mismatched types %s and %s)", op.String(), reflect.TypeOf(x).String(), reflect.TypeOf(y).String())

	switch reflect.TypeOf(x).String() {
	case "string":
		{
			switch op {
			case token.ADD:
				switch reflect.TypeOf(y).String() {
				case "string":
					return x.(string) + y.(string)
				default:
					panic(errInfo)
					return nil
				}
			case token.EQL:
				switch reflect.TypeOf(y).String() {
				case "string":
					return x.(string) == y.(string)
				default:
					panic(errInfo)
					return nil
				}
			case token.LSS:
				switch reflect.TypeOf(y).String() {
				case "string":
					return x.(string) < y.(string)
				default:
					panic(errInfo)
					return nil
				}
			case token.LEQ:
				switch reflect.TypeOf(y).String() {
				case "string":
					return x.(string) <= y.(string)
				default:
					panic(errInfo)
					return nil
				}
			case token.GTR:
				switch reflect.TypeOf(y).String() {
				case "string":
					return x.(string) > y.(string)
				default:
					panic(errInfo)
					return nil
				}
			case token.GEQ:
				switch reflect.TypeOf(y).String() {
				case "string":
					return x.(string) >= y.(string)
				default:
					panic(errInfo)
					return nil
				}
			case token.NEQ:
				switch reflect.TypeOf(y).String() {
				case "string":
					return x.(string) != y.(string)
				default:
					panic(errInfo)
					return nil
				}
			default:
				panic(errInfo)
				return nil
			}
		}
	case "int":
		{
			switch op {
			case token.ADD:
				switch reflect.TypeOf(y).String() {
				case "int":
					return x.(int) + y.(int)
				case "float64":
					return float64(x.(int)) + y.(float64)
				case "uint8":
					return x.(int) + int(y.(uint8))
				default:
					panic(errInfo)
					return nil
				}
			case token.SUB: // -
				switch reflect.TypeOf(y).String() {
				case "int":
					return x.(int) - y.(int)
				case "float64":
					return float64(x.(int)) - y.(float64)
				case "uint8":
					return x.(int) - int(y.(uint8))
				default:
					panic(errInfo)
					return nil
				}
			case token.MUL: // *
				switch reflect.TypeOf(y).String() {
				case "int":
					return x.(int) * y.(int)
				case "float64":
					return float64(x.(int)) * y.(float64)
				case "uint8":
					return x.(int) * int(y.(uint8))
				default:
					panic(errInfo)
					return nil
				}
			case token.QUO: // /
				switch reflect.TypeOf(y).String() {
				case "int":
					if y.(int) == 0 {
						panic("invalid operation: division by zero")
						return nil
					}
					return x.(int) / y.(int)
				case "float64":
					if y.(float64) == 0 {
						panic("invalid operation: division by zero")
						return nil
					}
					return float64(x.(int)) / y.(float64)
				case "uint8":
					if y.(uint8) == 0 {
						panic("invalid operation: division by zero")
						return nil
					}
					return x.(int) / int(y.(uint8))
				default:
					panic(errInfo)
					return nil
				}
			case token.REM: // %
				switch reflect.TypeOf(y).String() {
				case "int":
					return x.(int) % y.(int)
				case "uint8":
					return x.(int) % int(y.(uint8))
				default:
					panic(errInfo)
					return nil
				}
			case token.AND: // &
				switch reflect.TypeOf(y).String() {
				case "int":
					return x.(int) & y.(int)
				case "uint8":
					return x.(int) & int(y.(uint8))
				default:
					panic(errInfo)
					return nil
				}
			case token.OR: // |
				switch reflect.TypeOf(y).String() {
				case "int":
					return x.(int) | y.(int)
				case "uint8":
					return x.(int) | int(y.(uint8))
				default:
					panic(errInfo)
					return nil
				}
			case token.XOR: // ^
				switch reflect.TypeOf(y).String() {
				case "int":
					return x.(int) ^ y.(int)
				case "uint8":
					return x.(int) ^ int(y.(uint8))
				default:
					panic(errInfo)
					return nil
				}
			case token.SHL: // <<
				switch reflect.TypeOf(y).String() {
				case "int":
					return x.(int) ^ y.(int)
				case "uint8":
					return x.(int) ^ int(y.(uint8))
				default:
					panic(errInfo)
					return nil
				}
			case token.SHR: // >>
				switch reflect.TypeOf(y).String() {
				case "int":
					return x.(int) >> y.(int)
				case "uint8":
					return x.(int) >> int(y.(uint8))
				default:
					panic(errInfo)
					return nil
				}
			case token.AND_NOT: // &^
				switch reflect.TypeOf(y).String() {
				case "int":
					return x.(int) &^ y.(int)
				case "uint8":
					return x.(int) &^ int(y.(uint8))
				default:
					panic(errInfo)
					return nil
				}
			case token.EQL: // ==
				switch reflect.TypeOf(y).String() {
				case "int":
					return x.(int) == y.(int)
				case "float64":
					return float64(x.(int)) == y.(float64)
				case "uint8":
					return x.(int) == int(y.(uint8))
				default:
					panic(errInfo)
					return nil
				}
			case token.LSS: // <
				switch reflect.TypeOf(y).String() {
				case "int":
					return x.(int) < y.(int)
				case "float64":
					return float64(x.(int)) < y.(float64)
				case "uint8":
					return x.(int) < int(y.(uint8))
				default:
					panic(errInfo)
					return nil
				}
			case token.GTR: // >
				switch reflect.TypeOf(y).String() {
				case "int":
					return x.(int) > y.(int)
				case "float64":
					return float64(x.(int)) > y.(float64)
				case "uint8":
					return x.(int) > int(y.(uint8))
				default:
					panic(errInfo)
					return nil
				}
			case token.NEQ: // !=
				switch reflect.TypeOf(y).String() {
				case "int":
					return x.(int) != y.(int)
				case "float64":
					return float64(x.(int)) != y.(float64)
				case "uint8":
					return x.(int) != int(y.(uint8))
				default:
					panic(errInfo)
					return nil
				}
			case token.LEQ: // <=
				switch reflect.TypeOf(y).String() {
				case "int":
					return x.(int) <= y.(int)
				case "float64":
					return float64(x.(int)) <= y.(float64)
				case "uint8":
					return x.(int) <= int(y.(uint8))
				default:
					panic(errInfo)
					return nil
				}
			case token.GEQ: // >=
				switch reflect.TypeOf(y).String() {
				case "int":
					return x.(int) >= y.(int)
				case "float64":
					return float64(x.(int)) >= y.(float64)
				case "uint8":
					return x.(int) >= int(y.(uint8))
				default:
					panic(errInfo)
					return nil
				}
			default:
				panic(errInfo)
				return nil
			}
		}
	case "float64":
		{
			switch op {
			case token.ADD: // +
				switch reflect.TypeOf(y).String() {
				case "int":
					return x.(float64) + float64(y.(int))
				case "float64":
					return x.(float64) + y.(float64)
				case "uint8":
					return x.(float64) + float64(y.(uint8))
				default:
					panic(errInfo)
					return nil
				}
			case token.SUB: // -
				switch reflect.TypeOf(y).String() {
				case "int":
					return x.(float64) - float64(y.(int))
				case "float64":
					return x.(float64) - y.(float64)
				case "uint8":
					return x.(float64) - float64(y.(uint8))
				default:
					panic(errInfo)
					return nil
				}
			case token.MUL: // *
				switch reflect.TypeOf(y).String() {
				case "int":
					return x.(float64) * float64(y.(int))
				case "float64":
					return x.(float64) * y.(float64)
				case "uint8":
					return x.(float64) * float64(y.(uint8))
				default:
					panic(errInfo)
					return nil
				}
			case token.QUO: // /
				switch reflect.TypeOf(y).String() {
				case "int":
					if y.(int) == 0 {
						panic("invalid operation: division by zero")
						return nil
					}
					return x.(float64) / float64(y.(int))
				case "float64":
					if y.(float64) == 0 {
						panic("invalid operation: division by zero")
						return nil
					}
					return x.(float64) / y.(float64)
				case "uint8":
					if y.(uint8) == 0 {
						panic("invalid operation: division by zero")
						return nil
					}
					return x.(float64) / float64(y.(uint8))
				default:
					panic(errInfo)
					return nil
				}
			case token.EQL: // ==
				switch reflect.TypeOf(y).String() {
				case "int":
					return x.(float64) == float64(y.(int))
				case "float64":
					return x.(float64) == y.(float64)
				case "uint8":
					return x.(float64) == float64(y.(uint8))
				default:
					panic(errInfo)
					return nil
				}
			case token.LSS: // <
				switch reflect.TypeOf(y).String() {
				case "int":
					return x.(float64) < float64(y.(int))
				case "float64":
					return x.(float64) < y.(float64)
				case "uint8":
					return x.(float64) < float64(y.(uint8))
				default:
					panic(errInfo)
					return nil
				}
			case token.GTR: // >
				switch reflect.TypeOf(y).String() {
				case "int":
					return x.(float64) > float64(y.(int))
				case "float64":
					return x.(float64) > y.(float64)
				case "uint8":
					return x.(float64) > float64(y.(uint8))
				default:
					panic(errInfo)
					return nil
				}
			case token.NEQ: // !=
				switch reflect.TypeOf(y).String() {
				case "int":
					return x.(float64) != float64(y.(int))
				case "float64":
					return x.(float64) != y.(float64)
				case "uint8":
					return x.(float64) != float64(y.(uint8))
				default:
					panic(errInfo)
					return nil
				}
			case token.LEQ: // <=
				switch reflect.TypeOf(y).String() {
				case "int":
					return x.(float64) <= float64(y.(int))
				case "float64":
					return x.(float64) <= y.(float64)
				case "uint8":
					return x.(float64) <= float64(y.(uint8))
				default:
					panic(errInfo)
					return nil
				}
			case token.GEQ: // >=
				switch reflect.TypeOf(y).String() {
				case "int":
					return x.(float64) >= float64(y.(int))
				case "float64":
					return x.(float64) >= y.(float64)
				case "uint8":
					return x.(float64) >= float64(y.(uint8))
				default:
					panic(errInfo)
					return nil
				}
			default:
				panic(errInfo)
				return nil
			}
		}
	case "uint8":
		{
			switch op {
			case token.ADD: // +
				switch reflect.TypeOf(y).String() {
				case "int":
					return int(x.(uint8)) + y.(int)
				case "float64":
					return float64(x.(uint8)) + y.(float64)
				case "uint8":
					return x.(uint8) + y.(uint8)
				default:
					panic(errInfo)
					return nil
				}
			case token.SUB: // -
				switch reflect.TypeOf(y).String() {
				case "int":
					return int(x.(uint8)) - y.(int)
				case "float64":
					return float64(x.(uint8)) - y.(float64)
				case "uint8":
					return x.(uint8) - y.(uint8)
				default:
					panic(errInfo)
					return nil
				}
			case token.MUL: // *
				switch reflect.TypeOf(y).String() {
				case "int":
					return int(x.(uint8)) * y.(int)
				case "float64":
					return float64(x.(uint8)) * y.(float64)
				case "uint8":
					return x.(uint8) * y.(uint8)
				default:
					panic(errInfo)
					return nil
				}
			case token.QUO: // /
				switch reflect.TypeOf(y).String() {
				case "int":
					if y.(int) == 0 {
						panic("invalid operation: division by zero")
						return nil
					}
					return int(x.(uint8)) / y.(int)
				case "float64":
					if y.(float64) == 0 {
						panic("invalid operation: division by zero")
						return nil
					}
					return float64(x.(uint8)) / y.(float64)
				case "uint8":
					if y.(uint8) == 0 {
						panic("invalid operation: division by zero")
						return nil
					}
					return x.(uint8) / y.(uint8)
				default:
					panic(errInfo)
					return nil
				}
			case token.REM: // %
				switch reflect.TypeOf(y).String() {
				case "int":
					return int(x.(uint8)) % y.(int)
				case "uint8":
					return x.(uint8) % y.(uint8)
				default:
					panic(errInfo)
					return nil
				}
			case token.AND: // &
				switch reflect.TypeOf(y).String() {
				case "int":
					return int(x.(uint8)) & y.(int)
				case "uint8":
					return x.(uint8) & y.(uint8)
				default:
					panic(errInfo)
					return nil
				}
			case token.OR: // |
				switch reflect.TypeOf(y).String() {
				case "int":
					return int(x.(uint8)) | y.(int)
				case "uint8":
					return x.(uint8) | y.(uint8)
				default:
					panic(errInfo)
					return nil
				}
			case token.XOR: // ^
				switch reflect.TypeOf(y).String() {
				case "int":
					return int(x.(uint8)) ^ y.(int)
				case "uint8":
					return x.(uint8) ^ y.(uint8)
				default:
					panic(errInfo)
					return nil
				}
			case token.SHL: // <<
				switch reflect.TypeOf(y).String() {
				case "int":
					return x.(uint8) << y.(int)
				case "uint8":
					return x.(uint8) << y.(uint8)
				default:
					panic(errInfo)
					return nil
				}
			case token.SHR: // >>
				switch reflect.TypeOf(y).String() {
				case "int":
					return x.(uint8) >> y.(int)
				case "uint8":
					return x.(uint8) >> y.(uint8)
				default:
					panic(errInfo)
					return nil
				}
			case token.AND_NOT: // &^
				switch reflect.TypeOf(y).String() {
				case "int":
					return int(x.(uint8)) &^ y.(int)
				case "uint8":
					return x.(uint8) &^ y.(uint8)
				default:
					panic(errInfo)
					return nil
				}
			case token.EQL: // ==
				switch reflect.TypeOf(y).String() {
				case "int":
					return int(x.(uint8)) == y.(int)
				case "float64":
					return float64(x.(uint8)) == y.(float64)
				case "uint8":
					return x.(uint8) == y.(uint8)
				default:
					panic(errInfo)
					return nil
				}
			case token.LSS: // <
				switch reflect.TypeOf(y).String() {
				case "int":
					return int(x.(uint8)) < y.(int)
				case "float64":
					return float64(x.(uint8)) < y.(float64)
				case "uint8":
					return x.(uint8) < y.(uint8)
				default:
					panic(errInfo)
					return nil
				}
			case token.GTR: // >
				switch reflect.TypeOf(y).String() {
				case "int":
					return int(x.(uint8)) > y.(int)
				case "float64":
					return float64(x.(uint8)) > y.(float64)
				case "uint8":
					return x.(uint8) > y.(uint8)
				default:
					panic(errInfo)
					return nil
				}
			case token.NEQ: // !=
				switch reflect.TypeOf(y).String() {
				case "int":
					return int(x.(uint8)) != y.(int)
				case "float64":
					return float64(x.(uint8)) != y.(float64)
				case "uint8":
					return x.(uint8) != y.(uint8)
				default:
					panic(errInfo)
					return nil
				}
			case token.LEQ: // <=
				switch reflect.TypeOf(y).String() {
				case "int":
					return int(x.(uint8)) <= y.(int)
				case "float64":
					return float64(x.(uint8)) <= y.(float64)
				case "uint8":
					return x.(uint8) <= y.(uint8)
				default:
					panic(errInfo)
					return nil
				}
			case token.GEQ: // >=
				switch reflect.TypeOf(y).String() {
				case "int":
					return int(x.(uint8)) >= y.(int)
				case "float64":
					return float64(x.(uint8)) >= y.(float64)
				case "uint8":
					return x.(uint8) >= y.(uint8)
				default:
					panic(errInfo)
					return nil
				}
			default:
				panic(errInfo)
				return nil
			}
		}
	case "bool":
		{
			switch op {
			case token.LAND: // &&
				switch reflect.TypeOf(y).String() {
				case "bool":
					return x.(bool) && y.(bool)
				default:
					panic(errInfo)
					return nil
				}
			case token.LOR: // ||
				switch reflect.TypeOf(y).String() {
				case "bool":
					return x.(bool) || y.(bool)
				default:
					panic(errInfo)
					return nil
				}
			case token.EQL: // ==
				switch reflect.TypeOf(y).String() {
				case "bool":
					return x.(bool) == y.(bool)
				default:
					panic(errInfo)
					return nil
				}
			case token.NEQ: // !=
				switch reflect.TypeOf(y).String() {
				case "bool":
					return x.(bool) != y.(bool)
				default:
					panic(errInfo)
					return nil
				}
			default:
				panic(errInfo)
				return nil
			}
		}
	}
	return nil
}
