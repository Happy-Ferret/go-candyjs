package duktape

import (
	"fmt"
	"reflect"
	"strings"
	"time"

	goduktape "github.com/olebedev/go-duktape"
)

const goFuncCallName = "__goFuncCall__"
const functionHandler = `
    function(){
	    return %s.apply(this, ['%s'].concat(Array.prototype.slice.apply(arguments)));
    };
`

type Context struct {
	*goduktape.Context
}

func NewContext() *Context {
	return &Context{goduktape.Default()}
}

func (ctx *Context) SetRequireFunction(f interface{}) error {
	ctx.PushGlobalObject()
	ctx.GetPropString(-1, "Duktape")
	if err := ctx.PushGoFunction(f); err != nil {
		return err
	}

	ctx.PutPropString(-2, "modSearch")
	ctx.Pop()

	return nil
}

func (ctx *Context) PushGlobalStruct(name string, s interface{}) error {
	ctx.PushGlobalObject()
	if err := ctx.PushStruct(s); err != nil {
		return err
	}

	ctx.PutPropString(-2, name)
	ctx.Pop()

	return nil
}

func (ctx *Context) PushStruct(s interface{}) error {
	t := reflect.TypeOf(s)
	v := reflect.ValueOf(s)

	obj := ctx.PushObject()
	if err := ctx.pushStructMethods(obj, t, v); err != nil {
		return err
	}

	if t.Kind() == reflect.Ptr {
		v = v.Elem()
		t = v.Type()
	}

	return ctx.pushStructFields(obj, t, v)
}

func (ctx *Context) pushStructFields(obj int, t reflect.Type, v reflect.Value) error {
	fCount := t.NumField()
	for i := 0; i < fCount; i++ {
		value := v.Field(i)

		if value.Kind() != reflect.Ptr || !value.IsNil() {
			fieldName := lowerCapital(t.Field(i).Name)
			if fieldName == t.Field(i).Name {
				continue
			}

			if err := ctx.PushValue(value); err != nil {
				return err
			}

			ctx.PutPropString(obj, fieldName)
		}
	}

	return nil
}

func (ctx *Context) pushStructMethods(obj int, t reflect.Type, v reflect.Value) error {
	mCount := t.NumMethod()
	for i := 0; i < mCount; i++ {
		if err := ctx.PushGoFunction(v.Method(i).Interface()); err != nil {
			return err
		}

		ctx.PutPropString(obj, lowerCapital(t.Method(i).Name))
	}

	return nil
}

func (ctx *Context) PushGlobalValue(name string, v reflect.Value) error {
	ctx.PushGlobalObject()
	if err := ctx.PushValue(v); err != nil {
		return err
	}

	ctx.PutPropString(-2, name)
	ctx.Pop()

	return nil
}

func (ctx *Context) PushValue(v reflect.Value) error {
	switch v.Kind() {
	case reflect.Bool:
		ctx.PushBoolean(v.Bool())
	case reflect.Int, reflect.Int8, reflect.Int16, reflect.Int32, reflect.Int64:
		ctx.PushInt(int(v.Int()))
	case reflect.Uint, reflect.Uint8, reflect.Uint16, reflect.Uint32, reflect.Uint64:
		ctx.PushInt(int(v.Uint()))
	case reflect.Float64:
		ctx.PushNumber(v.Float())
	case reflect.String:
		ctx.PushString(v.String())
	case reflect.Struct:
		return ctx.PushStruct(v.Interface())
	case reflect.Ptr:
		if v.Elem().Kind() == reflect.Struct {
			return ctx.PushStruct(v.Interface())
		}

		return ctx.PushValue(v.Elem())
	default:
		//Returns nul if the Kind is not supported
		ctx.PushNull()
	}

	return nil
}

func (ctx *Context) PushGlobalValues(name string, vs []reflect.Value) error {
	ctx.PushGlobalObject()
	if err := ctx.PushValues(vs); err != nil {
		return err
	}

	ctx.PutPropString(-2, name)
	ctx.Pop()

	return nil
}

func (ctx *Context) PushValues(vs []reflect.Value) error {
	arr := ctx.PushArray()
	for i, v := range vs {
		if err := ctx.PushValue(v); err != nil {
			return err
		}

		ctx.PutPropIndex(arr, uint(i))
	}

	return nil
}

func (ctx *Context) PushGlobalGoFunction(name string, f interface{}) error {
	tbaContext := ctx
	return ctx.Context.PushGlobalGoFunction(name, func(ctx *goduktape.Context) int {
		args := tbaContext.getFunctionArgs(f)
		return tbaContext.callFunction(f, args)
	})
}

func (ctx *Context) PushGoFunction(f interface{}) error {
	name := fmt.Sprintf("method_%d", time.Now().Nanosecond())
	if err := ctx.PushGlobalGoFunction(name, f); err != nil {
		return err
	}

	ctx.CompileString(goduktape.CompileFunction, fmt.Sprintf(
		functionHandler, goFuncCallName, name,
	))

	return nil
}

func (ctx *Context) getFunctionArgs(f interface{}) []reflect.Value {
	def := reflect.ValueOf(f).Type()
	isVariadic := def.IsVariadic()
	inCount := def.NumIn()

	top := ctx.GetTopIndex()
	args := make([]reflect.Value, 0)
	for index := 1; index <= top; index++ {
		i := index - 1
		var t reflect.Type
		if index < inCount || (index == inCount && !isVariadic) {
			t = def.In(i)
		} else if isVariadic {
			t = def.In(inCount - 1).Elem()
		}

		args = append(args, ctx.getValueFromContext(index, t))
	}

	//Optional args
	argc := len(args)
	if inCount > argc {
		for i := argc; i < inCount; i++ {
			args = append(args, reflect.Zero(def.In(i)))
		}
	}

	return args
}

func (ctx *Context) getValueFromContext(index int, t reflect.Type) reflect.Value {
	value := ctx.RequireInterface(index)
	if value == nil {
		return reflect.Zero(t)
	}

	switch t.Kind() {
	case reflect.Int:
		value = int(value.(float64))
	case reflect.Int8:
		value = int8(value.(float64))
	case reflect.Int16:
		value = int16(value.(float64))
	case reflect.Int32:
		value = int32(value.(float64))
	case reflect.Int64:
		value = int64(value.(float64))
	case reflect.Uint:
		value = uint(value.(float64))
	case reflect.Uint8:
		value = uint8(value.(float64))
	case reflect.Uint16:
		value = uint16(value.(float64))
	case reflect.Uint32:
		value = uint32(value.(float64))
	case reflect.Uint64:
		value = uint64(value.(float64))
	case reflect.Float32:
		value = float32(value.(float64))
	}

	return reflect.ValueOf(value)
}

func (ctx *Context) RequireInterface(index int) interface{} {
	var value interface{}

	switch ctx.GetType(index) {
	case goduktape.TypeString:
		value = ctx.RequireString(index)
	case goduktape.TypeNumber:
		value = ctx.RequireNumber(index)
	case goduktape.TypeBoolean:
		value = ctx.RequireBoolean(index)
	case goduktape.TypeObject:
		if ctx.IsArray(index) {
			value = ctx.RequireSlice(index)
		} else {
			value = ctx.RequireMap(index)
		}
	case goduktape.TypeNull, goduktape.TypeUndefined, goduktape.TypeNone:
		value = nil
	default:
		value = "undefined"
	}

	return value
}

func (ctx *Context) RequireSlice(index int) []interface{} {
	s := make([]interface{}, 0)
	var i uint
	for ctx.GetPropIndex(index, i) {
		i++
		s = append(s, ctx.RequireInterface(-1))
	}

	return s
}

func (ctx *Context) RequireMap(index int) map[string]interface{} {
	ctx.Enum(index, goduktape.EnumOwnPropertiesOnly)

	m := make(map[string]interface{}, 0)
	for ctx.IsObject(-1) {
		if !ctx.Next(-1, true) {
			break
		}

		m[ctx.RequireString(-2)] = ctx.RequireInterface(-1)
		ctx.Pop2()
	}

	return m
}

func (ctx *Context) callFunction(f interface{}, args []reflect.Value) int {
	var err error
	out := reflect.ValueOf(f).Call(args)
	out, err = ctx.handleReturnError(out)
	if err != nil {
		//ctx.PushGoError(err)
		return goduktape.ErrRetError
	}

	if len(out) == 0 {
		return 1
	}

	if len(out) > 1 {
		err = ctx.PushValues(out)
	} else {
		err = ctx.PushValue(out[0])
	}

	if err != nil {
		//ctx.PushGoError(err)
		return goduktape.ErrRetInternal
	}

	return 1
}

func (ctx *Context) handleReturnError(out []reflect.Value) ([]reflect.Value, error) {
	c := len(out)
	if c == 0 {
		return out, nil
	}

	last := out[c-1]
	if last.Type().Name() == "error" {
		if !last.IsNil() {
			return nil, last.Interface().(error)
		}

		return out[:c-1], nil
	}

	return out, nil
}

func (ctx *Context) PushGoError(err error) {
	//fmt.Println(err)
	ctx.Error(102, "foo %s", "qux")
}

func lowerCapital(name string) string {
	return strings.ToLower(name[:1]) + name[1:]
}
