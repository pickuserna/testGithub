package apevaluator

import (
	"github.com/alangpierce/apgo/apast"
)

func panicBuiltin(ctx *Context, funcCall *apast.FuncCallExpr) Value {
	argExpr := funcCall.Args[0]
	arg := evaluateExpr(ctx, argExpr)
	panic(arg.get())
	return &NativeValue{nil}
}

type BuiltinFunc func(ctx *Context, funcCall *apast.FuncCallExpr) Value

var builtins map[string]BuiltinFunc
func init() {
	// Lazy-init to avoid a circular init loop.
	builtins = map[string]BuiltinFunc{
		"panic": panicBuiltin,
	}
}

// Builtins skip the normal evaluation step and are handled specially.
func resolveBuiltin(ctx *Context, funcCall *apast.FuncCallExpr) func() Value {
	switch funcExpr := funcCall.Func.(type) {
	case *apast.IdentExpr:
		if ctx.isNameValid(funcExpr.Name) {
			// If this is the name of a builtin, it's shadowed by a
			// user-defined name, so don't consider it to be a
			// builtin.
			// TODO: Consider cleaning this up and putting builtin
			// resolution as part of normal name resolution.
			return nil
		}
		builtin := builtins[funcExpr.Name]
		if builtin != nil {
			return func() Value {
				return builtin(ctx, funcCall)
			}
		}
	}
	return nil
}