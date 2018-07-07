package apevaluator

import (
	"reflect"
	"fmt"
)

// ExprResult is what you get when evaluating an expression. It is a little more
// generic than just a value because sometimes it can implicitly be assignable
// and/or have a pointer associated with it.
type ExprResult interface {
	get() Value
	set(val Value)
}

type RValue struct {
	val Value
}

func (rv *RValue) get() Value {
	return rv.val
}

func (rv *RValue) set(val Value) {
	panic(fmt.Sprint("Called set on RValue ", rv.val))
}

func (rv *RValue) String() string {
	return fmt.Sprint("RValue{", rv.val, "}")
}

type VariableLValue struct {
	varMap map[string]Value
	name string
}

func (lv *VariableLValue) get() Value {
	return lv.varMap[lv.name]
}

func (lv *VariableLValue) set(val Value) {
	lv.varMap[lv.name] = val
}

type ReflectValLValue struct {
	val reflect.Value
}

func (lv *ReflectValLValue) get() Value {
	return &NativeValue{lv.val.Interface()}
}

func (lv *ReflectValLValue) set(val Value) {
	lv.val.Set(reflect.ValueOf(val.AsNative()))
}

type StructLValue struct {
	structVal *StructValue
	name      string
}

func (lv *StructLValue) get() Value {
	return lv.structVal.Values[lv.name]
}

func (lv *StructLValue) set(val Value) {
	lv.structVal.Values[lv.name] = val
}