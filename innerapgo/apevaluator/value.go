package apevaluator

import (
	"fmt"
	"github.com/alangpierce/apgo/apast"
)

type Value interface {
	// Make an attempt to convert this to a native value, for example to
	// pass it to native code. Note that not all interpreted values are
	// possible to represent as native values.
	AsNative() interface{}
	Copy() Value
}

type NativeValue struct {
	val interface{}
}

func (nv *NativeValue) AsNative() interface{} {
	return nv.val
}

func (nv *NativeValue) Copy() Value {
	// TODO: Verify this when the true fate of NativeValue is more certain.
	return &NativeValue{
		nv.val,
	}
}

func (nv *NativeValue) String() string {
	return fmt.Sprint("NativeValue{", nv.val, "}")
}

type StructValue struct {
	// This is the concrete type of this struct instance.
	TypeName string
	Values map[string]Value
}

func (sv *StructValue) AsNative() interface{} {
	panic("Cannot convert StructValue to native value.")
}

func (sv *StructValue) Copy() Value {
	newValues := make(map[string]Value)
	for key, value := range sv.Values {
		newValues[key] = value
	}
	return &StructValue{
		sv.TypeName,
		newValues,
	}
}

func (sv *StructValue) String() string {
	return fmt.Sprint("StructValue{", sv.TypeName, ", ", sv.Values, "}")
}

type FunctionValue struct {
	FuncDecl *apast.FuncDecl
	BoundVariables map[string]Value
}

func (fv *FunctionValue) AsNative() interface{} {
	panic("Cannot convert FunctionValue to native value.")
}

func (fv *FunctionValue) Copy() Value {
	return &FunctionValue{
		fv.FuncDecl,
		fv.BoundVariables,
	}
}