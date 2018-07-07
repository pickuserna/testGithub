// Top-level class for managing the interpreter.
package interpreter

import (
	"github.com/alangpierce/apgo/apast"
	"github.com/alangpierce/apgo/apcompiler"
	"github.com/alangpierce/apgo/apevaluator"
	"github.com/alangpierce/apgo/apruntime"
	"go/ast"
	"go/parser"
	"go/token"
)

type Interpreter struct {
	packages       map[string]*apast.Package
	nativePackages map[string]*apruntime.NativePackage
}

func NewInterpreter() *Interpreter {
	return &Interpreter{
		packages:       make(map[string]*apast.Package),
		nativePackages: make(map[string]*apruntime.NativePackage),
	}
}

// Load and compile the package at the given path.
func (interpreter *Interpreter) LoadPackage(dirPath string) error {
	fset := token.NewFileSet()
	packageAsts, err := parser.ParseDir(fset, dirPath, nil, 0)
	if err != nil {
		return err
	}
	compileCtx := &apcompiler.CompileCtx{
		interpreter.nativePackages,
		make(map[string]bool),
		make(map[string]*ast.StructType),
	}
	for name, packageAst := range packageAsts {
		interpreter.packages[name] = apcompiler.CompilePackage(compileCtx, packageAst)
	}
	return nil
}

func (interpreter *Interpreter) LoadNativePackage(pack *apruntime.NativePackage) {
	interpreter.nativePackages[pack.Name] = pack
}

func (interpreter *Interpreter) RunMain() {
	mainPackage := interpreter.packages["main"]
	mainFunc := apevaluator.CreatePackageFuncValue(mainPackage, "main")
	apevaluator.EvaluateFunc(mainPackage,
		mainFunc.(*apevaluator.FunctionValue), []apevaluator.Value{})
}
