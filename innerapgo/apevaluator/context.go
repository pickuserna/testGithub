package apevaluator

import (
	"github.com/alangpierce/apgo/apast"
)

type Context struct {
	Locals map[string]Value
	Package *apast.Package
	// Slice of return values, or nil if the function hasn't returned yet.
	// This is used both for the values themselves and to communicate
	// control flow. For example, a function returning nothing should have
	// returnValues set to the empty slice upon returning, which signals to
	// other code that we want to finish the function now.
	returnValues []Value
	shouldBreak bool
}

type MethodSet struct {
	Methods map[string]*apast.MethodDecl
}

func NewContext(pack *apast.Package) *Context {
	return &Context{
		Locals: make(map[string]Value),
		Package: pack,
	}
}

func (ctx *Context) resolveValue(name string) ExprResult {
	if _, ok := ctx.Locals[name]; ok {
		return &VariableLValue{
			ctx.Locals,
			name,
		}
	} else if _, ok := ctx.Package.Funcs[name]; ok {
		return &RValue{
			CreatePackageFuncValue(ctx.Package, name),
		}
	} else {
		// If we didn't find anything, then create it as a local
		// variable.
		// TODO: Maybe we need to init to a zero value?
		ctx.Locals[name] = nil
		return &VariableLValue{
			ctx.Locals,
			name,
		}
	}
}

func (ctx *Context) isNameValid(name string) bool {
	if _, ok := ctx.Locals[name]; ok {
		return true
	} else if _, ok := ctx.Package.Funcs[name]; ok {
		return true
	}
	return false
}

func (ctx *Context) assignValue(name string, value Value) {
	ctx.Locals[name] = value
}