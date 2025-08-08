package redisorm

import (
	"encoding/json"
	"reflect"
	"strings"
)

func applyUpdatesByJSONName(dst any, updates map[string]any) {
	rv := reflect.ValueOf(dst); if rv.Kind() != reflect.Pointer || rv.IsNil() { return }
	rv = rv.Elem(); rt := rv.Type()
	nameToIndex := map[string]int{}
	for i := 0; i < rt.NumField(); i++ {
		f := rt.Field(i)
		if f.PkgPath != "" { continue }
		name := f.Name
		if tag := f.Tag.Get("json"); tag != "" {
			parts := strings.Split(tag, ",")
			if parts[0] != "" && parts[0] != "-" { name = parts[0] }
		}
		nameToIndex[name] = i
		nameToIndex[f.Name] = i
	}
	for k, val := range updates {
		if idx, ok := nameToIndex[k]; ok {
			fv := rv.Field(idx)
			if !fv.CanSet() { continue }
			setReflectValue(fv, val)
		}
	}
}

func setReflectValue(fv reflect.Value, val any) {
	if val == nil { return }
	t := fv.Type()
	switch t.Kind() {
	case reflect.String:
		if s, ok := val.(string); ok { fv.SetString(s) }
	case reflect.Bool:
		if b, ok := val.(bool); ok { fv.SetBool(b) }
	case reflect.Int, reflect.Int8, reflect.Int16, reflect.Int32, reflect.Int64:
		switch x := val.(type) { case int64: fv.SetInt(x); case int: fv.SetInt(int64(x)); case float64: fv.SetInt(int64(x)); case json.Number: if n, _ := x.Int64(); true { fv.SetInt(n) } }
	case reflect.Uint, reflect.Uint8, reflect.Uint16, reflect.Uint32, reflect.Uint64, reflect.Uintptr:
		switch x := val.(type) { case uint64: fv.SetUint(x); case uint: fv.SetUint(uint64(x)); case float64: fv.SetUint(uint64(x)) }
	case reflect.Float32, reflect.Float64:
		if f, ok := val.(float64); ok { fv.SetFloat(f) }
	case reflect.Struct:
		bs, _ := json.Marshal(val); _ = json.Unmarshal(bs, fv.Addr().Interface())
	case reflect.Pointer:
		if fv.IsNil() { fv.Set(reflect.New(t.Elem())) }
		setReflectValue(fv.Elem(), val)
	case reflect.Slice, reflect.Map:
		bs, _ := json.Marshal(val); _ = json.Unmarshal(bs, fv.Addr().Interface())
	}
}
