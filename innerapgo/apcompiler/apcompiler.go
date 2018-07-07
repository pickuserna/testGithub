package apcompiler

import (
	"go/ast"
	"github.com/alangpierce/apgo/apast"
	"go/token"
	"github.com/alangpierce/apgo/apruntime"
	"fmt"
	"reflect"
	"strconv"
	"strings"
)

type CompileCtx struct {
	NativePackages map[string]*apruntime.NativePackage
	ActiveVars map[string]bool
	StructDefs map[string]*ast.StructType
}

func CompilePackage(ctx *CompileCtx, pack *ast.Package) *apast.Package {
	// Compile and populate structs.
	for _, file := range pack.Files {
		for _, decl := range file.Decls {
			if decl, ok := decl.(*ast.GenDecl); ok {
				for _, spec := range decl.Specs {
					compileGenDecl(ctx, spec)
				}
			}
		}
	}

	// TODO: This code is slightly weird in that it doesn't populate types
	// with zero methods. For now that shouldn't matter, but it may be good
	// to make more consistent at some point.
	funcs := make(map[string]*apast.FuncDecl)
	types := make(map[string]*apast.TypeDecl)
	for _, file := range pack.Files {
		for _, decl := range file.Decls {
			switch decl := decl.(type) {
			case *ast.FuncDecl:
				if decl.Recv == nil {
					funcs[decl.Name.Name] = compileFuncDecl(ctx, decl)
				} else {
					methodDecl, typeName := compileMethodDecl(ctx, decl)
					if _, ok := types[typeName]; !ok {
						types[typeName] = &apast.TypeDecl{
							make(map[string]*apast.MethodDecl),
						}
					}
					types[typeName].Methods[decl.Name.Name] = methodDecl
				}
			}
		}
	}
	return &apast.Package{
		funcs,
		types,
	}
}

// For now, this just populates the compile context with the given declaration,
// if necessary.
func compileGenDecl(ctx *CompileCtx, spec ast.Spec) {
	switch spec := spec.(type) {
	case *ast.TypeSpec:
		if structType, ok := spec.Type.(*ast.StructType); ok {
			ctx.StructDefs[spec.Name.Name] = structType
		}
	}
}

func compileFuncDecl(ctx *CompileCtx, funcDecl *ast.FuncDecl) *apast.FuncDecl {
	// Clear the list of variables since it might be left over from the
	// previous function compilation.
	ctx.ActiveVars = make(map[string]bool)

	// Populate all initial variables (receiver, args, outputs).
	if funcDecl.Recv != nil {
		ctx.ActiveVars[funcDecl.Recv.List[0].Names[0].Name] = true
	}
	for _, field := range funcDecl.Type.Params.List {
		for _, name := range field.Names {
			ctx.ActiveVars[name.Name] = true
		}
	}
	if funcDecl.Type.Results != nil {
		for _, field := range funcDecl.Type.Results.List {
			for _, name := range field.Names {
				ctx.ActiveVars[name.Name] = true
			}
		}
	}

	paramNames := []string{}
	for _, param := range funcDecl.Type.Params.List {
		if param.Names == nil {
			paramNames = append(paramNames, "_")
		} else if len(param.Names) == 1 {
			paramNames = append(paramNames, param.Names[0].Name)
		} else {
			panic("Unexpected number of parameter names.")
		}
	}
	return &apast.FuncDecl{
		CompileStmt(ctx, funcDecl.Body),
		paramNames,
	}
}

func compileMethodDecl(ctx *CompileCtx, methodDecl *ast.FuncDecl) (method *apast.MethodDecl, typeName string) {
	typeName, isPointer := getMethodReceiverType(methodDecl)
	return &apast.MethodDecl{
		ReceiverName: methodDecl.Recv.List[0].Names[0].Name,
		IsPointer: isPointer,
		Func: compileFuncDecl(ctx, methodDecl),
	}, typeName
}

// Get information about the receiver type. Receiver types can only be either
// a named type or a pointer to a named type.
func getMethodReceiverType(funcDecl *ast.FuncDecl) (typeName string, isPointer bool) {
	field := funcDecl.Recv.List[0]
	if fieldType, ok := field.Type.(*ast.Ident); ok {
		return fieldType.Name, false
	}
	if fieldType, ok := field.Type.(*ast.StarExpr); ok {
		if underlyingType, ok := fieldType.X.(*ast.Ident); ok {
			return underlyingType.Name, true
		}
	}
	panic("Unexpected receiver type.")
}

func CompileStmt(ctx *CompileCtx, stmt ast.Stmt) apast.Stmt {
	switch stmt := stmt.(type) {
	//case *ast.BadStmt:
	//	return nil
	case *ast.DeclStmt:
		switch decl := stmt.Decl.(type) {
		case *ast.GenDecl:
			// Turn a declaration into assignment to the zero value.
			// For example, `var x, y int` becomes `x, y = 0, 0`
			varsToInit := []apast.Expr{}
			zeroTerms := []apast.Expr{}
			for _, spec := range decl.Specs {
				switch spec := spec.(type) {
				case *ast.ValueSpec:
					zeroValueExpr := getZeroValueExpr(ctx, spec.Type)
					for _, ident := range spec.Names {
						ctx.ActiveVars[ident.Name] = true
						varsToInit = append(varsToInit, &apast.IdentExpr{
							ident.Name,
						})
						zeroTerms = append(zeroTerms, zeroValueExpr)
					}
				default:
					panic("Unexpected spec")
					return nil
				}
			}

			return &apast.AssignStmt{
				varsToInit,
				zeroTerms,
			}
		default:
			panic("Unexpected declaration")
			return nil
		}
	//case *ast.EmptyStmt:
	//	return nil
	//case *ast.LabeledStmt:
	//	return nil
	case *ast.ExprStmt:
		return &apast.ExprStmt{
			compileExpr(ctx, stmt.X),
		}
	//case *ast.SendStmt:
	//	return nil
	case *ast.IncDecStmt:
		compiledLhs := compileExpr(ctx, stmt.X)
		// TODO: This compiles into an expression that evaluates the
		// left side twice.
		return &apast.AssignStmt{
			[]apast.Expr{compiledLhs},
			[]apast.Expr{
				&apast.FuncCallExpr{
					&apast.LiteralExpr{
						apruntime.IncDecOperators[stmt.Tok],
					},
					[]apast.Expr{
						compiledLhs,
						&apast.LiteralExpr{1},
					},
				},
			},
		}
	case *ast.AssignStmt:
		if stmt.Tok == token.DEFINE || stmt.Tok == token.ASSIGN {
			lhs := []apast.Expr{}
			rhs := []apast.Expr{}
			for _, lhsExpr := range stmt.Lhs {
				// Note that this is a bit overkill because
				// we're counting assignments or definitions as
				// creating variables, but that should still be
				// correct.
				if lhsExpr, ok := lhsExpr.(*ast.Ident); ok {
					ctx.ActiveVars[lhsExpr.Name] = true
				}
				lhs = append(lhs, compileExpr(ctx, lhsExpr))
			}
			for _, rhsExpr := range stmt.Rhs {
				rhs = append(rhs, compileExpr(ctx, rhsExpr))
			}
			return &apast.AssignStmt{
				lhs,
				rhs,
			}
		} else {
			if len(stmt.Lhs) != 1 || len(stmt.Rhs) != 1 {
				panic("Unexpected multiple assign")
			}
			// TODO: We should only evaluate the left side once,
			// e.g. array index values.
			compiledLhs := compileExpr(ctx, stmt.Lhs[0])
			if _, ok := apruntime.AssignBinaryOperators[stmt.Tok]; !ok {
				panic(fmt.Sprint("Operator not implemented: ", stmt.Tok))
			}
			return &apast.AssignStmt{
				[]apast.Expr{compiledLhs},
				[]apast.Expr{
					&apast.FuncCallExpr{
						&apast.LiteralExpr{
							apruntime.AssignBinaryOperators[stmt.Tok],
						},
						[]apast.Expr{
							compiledLhs,
							compileExpr(ctx, stmt.Rhs[0]),
						},
					},
				},
			}
		}
	//case *ast.GoStmt:
	//	return nil
	//case *ast.DeferStmt:
	//	return nil
	case *ast.ReturnStmt:
		resultsExprs := []apast.Expr{}
		for _, result := range stmt.Results {
			resultsExprs = append(resultsExprs, compileExpr(ctx, result))
		}
		return &apast.ReturnStmt{
			resultsExprs,
		}
	case *ast.BranchStmt:
		switch stmt.Tok {
		case token.BREAK:
			return &apast.BreakStmt{}
		default:
			panic(fmt.Sprint("Unsupported branch statement: ", stmt.Tok))
			return nil
		}
	case *ast.BlockStmt:
		stmts := []apast.Stmt{}
		for _, subStmt := range stmt.List {
			stmts = append(stmts, CompileStmt(ctx, subStmt))
		}
		return &apast.BlockStmt{
			stmts,
		}
	case *ast.IfStmt:
		var result apast.IfStmt
		if stmt.Init != nil {
			result.Init = CompileStmt(ctx, stmt.Init)
		} else {
			result.Init = &apast.EmptyStmt{}
		}
		result.Cond = compileExpr(ctx, stmt.Cond)
		result.Body = CompileStmt(ctx, stmt.Body)
		if stmt.Else != nil {
			result.Else = CompileStmt(ctx, stmt.Else)
		} else {
			result.Else = &apast.EmptyStmt{}
		}
		return &result
	//case *ast.CaseClause:
	//	return nil
	//case *ast.SwitchStmt:
	//	return nil
	//case *ast.TypeSwitchStmt:
	//	return nil
	//case *ast.CommClause:
	//	return nil
	//case *ast.SelectStmt:
	//	return nil
	case *ast.ForStmt:
		var result apast.ForStmt
		if stmt.Init != nil {
			result.Init = CompileStmt(ctx, stmt.Init)
		} else {
			result.Init = &apast.EmptyStmt{}
		}
		if stmt.Cond != nil {
			result.Cond = compileExpr(ctx, stmt.Cond)
		} else {
			result.Cond = &apast.LiteralExpr{true}
		}
		if stmt.Post != nil {
			result.Post = CompileStmt(ctx, stmt.Post)
		} else {
			result.Post = &apast.EmptyStmt{}
		}
		result.Body = CompileStmt(ctx, stmt.Body)
		return &result
	//case *ast.RangeStmt:
	//	return nil
	default:
		panic(fmt.Sprint("Statement compile not implemented: ", reflect.TypeOf(stmt)))
		return nil
	}
}

func compileExpr(ctx *CompileCtx, expr ast.Expr) apast.Expr {
	switch expr := expr.(type) {
	//case *ast.BadExpr:
	//	return nil
	case *ast.Ident:
		return &apast.IdentExpr{
			expr.Name,
		}
	//case *ast.Ellipsis:
	//	return nil
	case *ast.BasicLit:
		return &apast.LiteralExpr{
			parseLiteral(expr.Value, expr.Kind),
		}
	//case *ast.FuncLit:
	//	return nil
	case *ast.CompositeLit:
		return compileCompositeLit(ctx, expr)
	//case *ast.ParenExpr:
	//	return nil
	case *ast.SelectorExpr:
		if leftSide, ok := expr.X.(*ast.Ident); ok {
			if _, ok := ctx.ActiveVars[leftSide.Name]; ok {
				return &apast.FieldAccessExpr{
					compileExpr(ctx, leftSide),
					expr.Sel.Name,
				}
			} else {
				return compilePackageFunc(ctx, leftSide, expr.Sel)
			}
		} else {
			panic(fmt.Sprint("Selector not found ", expr))
		}
	case *ast.IndexExpr:
		return &apast.IndexExpr{
			compileExpr(ctx, expr.X),
			compileExpr(ctx, expr.Index),
		}
	//case *ast.SliceExpr:
	//	return nil
	//case *ast.TypeAssertExpr:
	//	return nil
	case *ast.CallExpr:
		compiledArgs := []apast.Expr{}
		for _, arg := range expr.Args {
			compiledArgs = append(compiledArgs, compileExpr(ctx, arg))
		}
		return &apast.FuncCallExpr{
			compileExpr(ctx, expr.Fun),
			compiledArgs,
		}
	//case *ast.StarExpr:
	//	return nil
	//case *ast.UnaryExpr:
	//	return nil
	case *ast.BinaryExpr:
		if op, ok := apruntime.BinaryOperators[expr.Op]; ok {
			return &apast.FuncCallExpr{
				&apast.LiteralExpr{
					op,
				},
				[]apast.Expr{compileExpr(ctx, expr.X), compileExpr(ctx, expr.Y)},
			}
		} else {
			panic(fmt.Sprint("Operator not implemented: ", expr.Op))
		}
	//case *ast.KeyValueExpr:
	//	return nil
	//
	//case *ast.ArrayType:
	//	return nil
	//case *ast.StructType:
	//	return nil
	//case *ast.FuncType:
	//	return nil
	//case *ast.InterfaceType:
	//	return nil
	//case *ast.MapType:
	//	return nil
	//case *ast.ChanType:
	//	return nil
	default:
		panic(fmt.Sprint("Expression compile not implemented: ", reflect.TypeOf(expr)))
		return nil
	}
}

func compilePackageFunc(ctx *CompileCtx, leftSide *ast.Ident, sel *ast.Ident) apast.Expr {
	nativePackage := ctx.NativePackages[leftSide.Name]
	if nativePackage == nil {
		panic(fmt.Sprint("Unknown package ", leftSide.Name))
	}
	funcVal := nativePackage.Funcs[sel.Name]
	if funcVal == nil {
		panic(fmt.Sprint("Unknown function ", sel.Name))
	}
	return &apast.LiteralExpr{funcVal}
}

func compileCompositeLit(ctx *CompileCtx, expr *ast.CompositeLit) apast.Expr {
	// One reason this is in its own function is that IntelliJ currently
	// doesn't seem to properly handle nested type switches in the same
	// function.
	switch exprType := expr.Type.(type) {
	case *ast.ArrayType:
		vals := []apast.Expr{}
		for _, elt := range expr.Elts {
			vals = append(vals, compileExpr(ctx, elt))
		}
		// All we really need to know is if it's a slice or an array; if
		// it's an array, we know the length from the number of
		// elements, so we don't need to store it.
		if exprType.Len == nil {
			return &apast.SliceLiteralExpr{
				compileExpr(ctx, exprType.Elt),
				vals,
			}
		} else {
			return &apast.ArrayLiteralExpr{
				compileExpr(ctx, exprType.Elt),
				vals,
			}
		}
	case *ast.Ident:
		// Struct creation.
		if structDef, ok := ctx.StructDefs[exprType.Name]; ok {
			return compileStructLiteral(ctx, exprType.Name, structDef, expr)
		} else {
			panic(fmt.Sprint("Unknown struct ", exprType.Name))
		}
	default:
		panic(fmt.Sprint("Composite literal not implemented: ", reflect.TypeOf(exprType)))
		return nil
	}
}

func compileStructLiteral(
		ctx *CompileCtx, structName string, structDef *ast.StructType,
		expr *ast.CompositeLit) apast.Expr {
	// Start out all fields with the zero value, then later replace any that
	// are specified explicitly.
	literalExpr, fieldNames := getStructZeroValueExpr(ctx, structName, structDef)
	for i, elt := range expr.Elts {
		if kvElt, ok := elt.(*ast.KeyValueExpr); ok {
			if keyIdent, ok := kvElt.Key.(*ast.Ident); ok {
				literalExpr.InitialValues[keyIdent.Name] = compileExpr(ctx, kvElt.Value)
			} else {
				panic("Expected identifier as struct literal key.")
			}
		} else {
			literalExpr.InitialValues[fieldNames[i]] = compileExpr(ctx, elt)
		}
	}

	return literalExpr
}

// parseLiteral takes a primitive literal and returns it as a value.
func parseLiteral(val string, kind token.Token) interface{} {
	switch kind {
	case token.IDENT:
		panic("TODO")
		return nil
	case token.INT:
		// Note that base 0 means that octal and hex literals are also
		// handled. We also treat the number as an int instead of an
		// int64 so that comparisons work right.
		result, err := strconv.ParseInt(val, 0, 0)
		if err != nil {
			panic(err)
		}
		return int(result)
	case token.FLOAT:
		panic("TODO")
		return nil
	case token.IMAG:
		panic("TODO")
		return nil
	case token.CHAR:
		panic("TODO")
		return nil
	case token.STRING:
		return parseString(val)
	default:
		fmt.Print("Unrecognized kind: ", kind)
		return nil
	}
}

func parseString(codeString string) string {
	strWithoutQuotes := codeString[1:len(codeString) - 1]
	// TODO: Replace with an implementation that properly escapes
	// everything.
	return strings.Replace(strWithoutQuotes, "\\n", "\n", -1)
}

func getZeroValueExpr(ctx *CompileCtx, t ast.Expr) apast.Expr {
	// TODO: Consider using reflect.New here.
	if t, ok := t.(*ast.Ident); ok {
		switch t.Name {
		case "int":
			return &apast.LiteralExpr{0}
		}
		if structDef, ok := ctx.StructDefs[t.Name]; ok {
			result, _ := getStructZeroValueExpr(ctx, t.Name, structDef)
			return result
		} else {
			panic(fmt.Sprint("Unexpected type identifier: ", t.Name))
		}
	} else {
		panic("Zero for non-identifier types not implemented")
	}
}

// Returns an initialization expression for the struct and a list of the fields
// in the struct (for convenience, since some callers want to refer to field
// names by index).
func getStructZeroValueExpr(ctx *CompileCtx, structName string,
		structDef *ast.StructType) (
		*apast.StructLiteralExpr, []string) {
	fields := structDef.Fields.List
	initialValues := make(map[string]apast.Expr)
	fieldNames := make([]string, len(fields), len(fields))

	// Start out all fields with the zero value, then later replace any that
	// are specified explicitly.
	for i, field := range fields {
		if len(field.Names) != 1 {
			panic("Expected exactly one field name in struct def.")
		}
		fieldName := field.Names[0].Name
		fieldNames[i] = fieldName
		initialValues[fieldName] = getZeroValueExpr(ctx, field.Type)
	}
	return &apast.StructLiteralExpr{
		structName,
		initialValues,
	}, fieldNames
}