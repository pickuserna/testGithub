package apevaluator

import (
	"github.com/alangpierce/apgo/apast"
	"reflect"
	"fmt"
)

// Creates a Go function corresponding to the given function in the package.
func CreatePackageFuncValue(pack *apast.Package, name string) Value {
	return &FunctionValue{
		pack.Funcs[name],
		make(map[string]Value),
	}
}

func EvaluateFunc(pack *apast.Package, funcValue *FunctionValue, args []Value) []Value {
	ctx := NewContext(pack)
	for i, argName := range funcValue.FuncDecl.ParamNames {
		ctx.assignValue(argName, args[i])
	}
	for name, val := range funcValue.BoundVariables {
		ctx.assignValue(name, val)
	}
	EvaluateStmt(ctx, funcValue.FuncDecl.Body)
	return ctx.returnValues
}

// Create an intermediate method function for the given method and receiver.
func createMethodValue(pack *apast.Package, method *apast.MethodDecl, receiver Value) Value {
	// Do a copy if we're using pass-by-value.
	if !method.IsPointer {
		receiver = receiver.Copy()
	}
	return &FunctionValue{
		method.Func,
		map[string]Value {
			method.ReceiverName: receiver,
		},
	}
}

func EvaluateStmt(ctx *Context, stmt apast.Stmt) {
	switch stmt := stmt.(type) {
	case *apast.ExprStmt:
		evaluateExpr(ctx, stmt.E)
	case *apast.BlockStmt:
		for _, line := range stmt.Stmts {
			EvaluateStmt(ctx, line)
			// If this sub-statement returned, we don't want to
			// continue any further.
			if ctx.returnValues != nil || ctx.shouldBreak {
				return
			}
		}
	case *apast.AssignStmt:
		if len(stmt.Lhs) != len(stmt.Rhs) {
			panic("Multiple assign with differing lengths not implemented.")
		}
		values := []ExprResult{}
		for _, rhsExpr := range stmt.Rhs {
			values = append(values, evaluateExpr(ctx, rhsExpr))
		}
		for i, value := range values {
			lvalue := evaluateExpr(ctx, stmt.Lhs[i])
			lvalue.set(value.get())
		}
	case *apast.EmptyStmt:
		// Do nothing.
	case *apast.IfStmt:
		// TODO: Handle scopes properly, if necessary.
		EvaluateStmt(ctx, stmt.Init)
		condValue := evaluateExpr(ctx, stmt.Cond)
		if condValue.get().AsNative().(bool) {
			EvaluateStmt(ctx, stmt.Body)
		} else {
			EvaluateStmt(ctx, stmt.Else)
		}
	case *apast.ForStmt:
		// TODO: Handle scopes properly, if necessary.
		EvaluateStmt(ctx, stmt.Init)
		for {
			condValue := evaluateExpr(ctx, stmt.Cond)
			if !condValue.get().AsNative().(bool) {
				break
			}
			EvaluateStmt(ctx, stmt.Body)
			if ctx.shouldBreak {
				ctx.shouldBreak = false
				break
			}
			EvaluateStmt(ctx, stmt.Post)
		}
	case *apast.BreakStmt:
		ctx.shouldBreak = true
	case *apast.ReturnStmt:
		returnValues := []Value{}
		for _, result := range stmt.Results {
			returnValues = append(returnValues, evaluateExpr(ctx, result).get())
		}
		ctx.returnValues = returnValues
	default:
		panic(fmt.Sprint("Statement eval not implemented: ", reflect.TypeOf(stmt)))
	}
}


func evaluateExpr(ctx *Context, expr apast.Expr) ExprResult {
	switch expr := expr.(type) {
	case *apast.FuncCallExpr:
		maybeBuiltin := resolveBuiltin(ctx, expr)
		if (maybeBuiltin != nil) {
			return &RValue{
				maybeBuiltin(),
			}
		}

		f := evaluateExpr(ctx, expr.Func).get()
		args := []Value{}
		for _, argExpr := range expr.Args {
			args = append(args, evaluateExpr(ctx, argExpr).get())
		}
		if interpretedFunc, ok := f.(*FunctionValue); ok {
			// TODO: Support multiple return values.
			results := EvaluateFunc(ctx.Package, interpretedFunc, args)
			if len(results) == 1 {
				return &RValue{
					results[0],
				}
			} else {
				return &RValue{
					&NativeValue{nil},
				}
			}
		} else if nativeFunc, ok := f.(*NativeValue); ok {
			return evaluateNativeFunc(nativeFunc, args)
		} else {
			panic(fmt.Sprint("Unexpected function call on ", f))
		}

	case *apast.IdentExpr:
		return ctx.resolveValue(expr.Name)
	case *apast.IndexExpr:
		// TODO: Handle maps.
		arrOrSlice := evaluateExpr(ctx, expr.E).get()
		index := evaluateExpr(ctx, expr.Index).get()
		return &ReflectValLValue{
			reflect.ValueOf(arrOrSlice.AsNative()).Index(index.AsNative().(int)),
		}
	case *apast.FieldAccessExpr:
		leftSide := evaluateExpr(ctx, expr.E)
		if sv, ok := leftSide.get().(*StructValue); ok {
			// If it matches a method name, resolve to a method.
			// Otherwise, resolve to a struct field.
			if typeDecl, ok := ctx.Package.Types[sv.TypeName]; ok {
				if method, ok := typeDecl.Methods[expr.Name]; ok {
					return &RValue{
						createMethodValue(ctx.Package, method, sv),
					}
				}
			}
			if _, ok := sv.Values[expr.Name]; ok {
				return &StructLValue{
					sv,
					expr.Name,
				}
			}
			panic(fmt.Sprint("Field not found: ", expr.Name))
		} else {
			panic(fmt.Sprint("Unsupported field access on ", leftSide.get()))
		}
	case *apast.LiteralExpr:
		return &RValue{
			&NativeValue{
				expr.Val,
			},
		}
	case *apast.SliceLiteralExpr:
		typ := evaluateType(expr.Type)
		result := reflect.MakeSlice(
			reflect.SliceOf(typ), len(expr.Vals), len(expr.Vals))
		for i, val := range expr.Vals {
			result.Index(i).Set(reflect.ValueOf(evaluateExpr(ctx, val).get().AsNative()))
		}
		return &RValue{
			&NativeValue{
				result.Interface(),
			},
		}
	case *apast.StructLiteralExpr:
		structVal := &StructValue{
			expr.TypeName,
			make(map[string]Value),
		}
		// Populate the initial values, which should include setting
		// fields to their proper zeros.
		for key, valueExpr := range expr.InitialValues {
			structVal.Values[key] = evaluateExpr(ctx, valueExpr).get()
		}
		return &RValue{
			structVal,
		}
	default:
		panic(fmt.Sprint("Expression eval not implemented: ", reflect.TypeOf(expr)))
	}
}

func evaluateType(expr apast.Expr) reflect.Type {
	switch expr := expr.(type) {
	case *apast.IdentExpr:
		if expr.Name == "int" {
			return reflect.TypeOf(0)
		} else {
			panic(fmt.Sprint("Type not implemented: ", expr.Name))
		}
	default:
		panic(fmt.Sprint("Type expression not implemented: ", reflect.TypeOf(expr)))
	}
}

func evaluateNativeFunc(nativeFunc *NativeValue, args []Value) ExprResult {
	argVals := []reflect.Value{}
	for _, arg := range args {
		argVals = append(argVals, reflect.ValueOf(arg.AsNative()))
	}
	resultVals := reflect.ValueOf(nativeFunc.AsNative()).Call(argVals)
	if len(resultVals) == 1 {
		return &RValue{
			&NativeValue{resultVals[0].Interface()},
		}
	} else {
		return &RValue{nil}
	}
}