package redisorm

import (
	"errors"
	"fmt"
	"reflect"
	"strings"
	"time"

	"github.com/google/uuid"
)

type Defaultable interface { SetDefaults() }

func typeName(v any) string {
	t := reflect.TypeOf(v)
	for t.Kind() == reflect.Pointer { t = t.Elem() }
	return t.Name()
}

func ensurePrimaryKey(v any) (string, error) {
	rv := reflect.ValueOf(v)
	if rv.Kind() != reflect.Pointer || rv.IsNil() { return "", errors.New("dst must be non-nil pointer") }
	rv = rv.Elem(); rt := rv.Type()
	fi := -1
	for i := 0; i < rt.NumField(); i++ {
		f := rt.Field(i)
		if f.Tag.Get("redis") == "pk" || strings.EqualFold(f.Name, "ID") { fi = i; break }
	}
	if fi == -1 { return "", errors.New("no pk field (tag `redis:\"pk\"` or field ID)") }
	fv := rv.Field(fi)
	if fv.Kind() != reflect.String || !fv.CanSet() { return "", errors.New("pk must be settable string") }
	id := fv.String()
	if id == "" { id = uuid.NewString(); fv.SetString(id) }
	return id, nil
}

func readPrimaryKey(v any) (string, error) {
	rv := reflect.ValueOf(v)
	if rv.Kind() != reflect.Pointer || rv.IsNil() { return "", errors.New("dst must be non-nil pointer") }
	rv = rv.Elem(); rt := rv.Type()
	for i := 0; i < rt.NumField(); i++ {
		f := rt.Field(i)
		if f.Tag.Get("redis") == "pk" || strings.EqualFold(f.Name, "ID") {
			fv := rv.Field(i)
			if fv.Kind() != reflect.String { return "", errors.New("pk must be string") }
			return fv.String(), nil
		}
	}
	return "", errors.New("no pk field")
}

func touchTimestamps(v any) {
	rv := reflect.ValueOf(v); if rv.Kind() != reflect.Pointer || rv.IsNil() { return }
	rv = rv.Elem(); now := time.Now().UTC()
	if f := rv.FieldByName("UpdatedAt"); f.IsValid() && f.CanSet() && f.Type().String() == "time.Time" { f.Set(reflect.ValueOf(now)) }
	if f := rv.FieldByName("CreatedAt"); f.IsValid() && f.CanSet() && f.Type().String() == "time.Time" {
		if f.Interface().(time.Time).IsZero() { f.Set(reflect.ValueOf(now)) }
	}
}

func applyDefaults(v any) {
	rv := reflect.ValueOf(v); if rv.Kind() != reflect.Pointer || rv.IsNil() { return }
	rv = rv.Elem(); rt := rv.Type(); now := time.Now().UTC()
	for i := 0; i < rt.NumField(); i++ {
		f := rt.Field(i)
		tag := f.Tag.Get("default"); if tag == "" { continue }
		fv := rv.Field(i); if !fv.CanSet() || !isZero(fv) { continue }
		switch fv.Kind() {
		case reflect.String:
			switch tag { case "uuid": fv.SetString(uuid.NewString()); case "now_rfc3339": fv.SetString(now.Format(time.RFC3339)); default: fv.SetString(tag) }
		case reflect.Int, reflect.Int64:
			switch tag { case "unix": fv.SetInt(now.Unix()); case "unixms": fv.SetInt(now.UnixMilli()); default: if n, err := parseInt64(tag); err == nil { fv.SetInt(n) } }
		case reflect.Bool:
			if tag == "true" { fv.SetBool(true) }; if tag == "false" { fv.SetBool(false) }
		default:
			if fv.Type().String() == "time.Time" && tag == "now" { fv.Set(reflect.ValueOf(now)) }
		}
	}
}

func isZero(v reflect.Value) bool {
	switch v.Kind() {
	case reflect.String: return v.Len() == 0
	case reflect.Bool: return !v.Bool()
	case reflect.Int, reflect.Int8, reflect.Int16, reflect.Int32, reflect.Int64: return v.Int() == 0
	case reflect.Uint, reflect.Uint8, reflect.Uint16, reflect.Uint32, reflect.Uint64, reflect.Uintptr: return v.Uint() == 0
	case reflect.Float32, reflect.Float64: return v.Float() == 0
	case reflect.Pointer, reflect.Interface, reflect.Slice, reflect.Map: return v.IsNil()
	case reflect.Struct:
		if v.Type().String() == "time.Time" { return v.Interface().(time.Time).IsZero() }
	}
	zero := reflect.Zero(v.Type()).Interface()
	return reflect.DeepEqual(v.Interface(), zero)
}

func parseInt64(s string) (int64, error) { var n int64; _, err := fmt.Sscanf(s, "%d", &n); return n, err }

// Version helpers
func versionPointer(v any) (*int64, int) {
	rv := reflect.ValueOf(v)
	if rv.Kind() != reflect.Pointer || rv.IsNil() { return nil, -1 }
	rv = rv.Elem(); rt := rv.Type()
	for i := 0; i < rt.NumField(); i++ {
		f := rt.Field(i)
		if f.Type.Kind() == reflect.Int64 && (strings.EqualFold(f.Name, "Version") || f.Tag.Get("redis") == "version") {
			fv := rv.Field(i)
			return fv.Addr().Interface().(*int64), i
		}
	}
	return nil, -1
}
func setVersion(v any, val int64) {
	ptr, idx := versionPointer(v)
	if ptr == nil { return }
	rv := reflect.ValueOf(v)
	if rv.Kind() != reflect.Pointer || rv.IsNil() { return }
	rv = rv.Elem()
	rv.Field(idx).SetInt(val)
}

func toJSONNative(v reflect.Value) any {
	if v.Kind() == reflect.Pointer { if v.IsNil() { return nil }; return toJSONNative(v.Elem()) }
	switch v.Kind() {
	case reflect.String: return v.String()
	case reflect.Bool: return v.Bool()
	case reflect.Int, reflect.Int8, reflect.Int16, reflect.Int32, reflect.Int64: return v.Int()
	case reflect.Uint, reflect.Uint8, reflect.Uint16, reflect.Uint32, reflect.Uint64, reflect.Uintptr: return v.Uint()
	case reflect.Float32, reflect.Float64: return v.Float()
	case reflect.Slice:
		if v.Type().Elem().Kind() == reflect.Uint8 { return v.Bytes() }
		n := v.Len(); a := make([]any, n)
		for i := 0; i < n; i++ { a[i] = toJSONNative(v.Index(i)) }
		return a
	case reflect.Map:
		iter := v.MapRange(); m := make(map[string]any)
		for iter.Next() { m[fmt.Sprint(iter.Key().Interface())] = toJSONNative(iter.Value()) }
		return m
	case reflect.Struct:
		return v.Interface()
	default:
		return v.Interface()
	}
}
