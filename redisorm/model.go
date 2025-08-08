package redisorm

import (
	"errors"
	"fmt"
	"reflect"
	"strings"
	"time"

	"github.com/google/uuid"
)

type Defaultable interface{ SetDefaults() }

func typeName(v any) string {
	t := reflect.TypeOf(v)
	for t.Kind() == reflect.Pointer {
		t = t.Elem()
	}
	return t.Name()
}

// >>>>>>>>> CHANGED <<<<<<<<<
func ensurePrimaryKey(v any, meta *ModelMetadata) (string, error) {
	rv := reflect.ValueOf(v)
	if rv.Kind() != reflect.Pointer || rv.IsNil() {
		return "", errors.New("dst must be non-nil pointer")
	}
	rv = rv.Elem()

	if len(meta.PKFields) == 0 {
		return "", errors.New("no pk field (tag `redis:\"pk\"` or field ID)")
	}
	pkFieldName := meta.PKFields[0]
	fv := rv.FieldByName(pkFieldName)

	if fv.Kind() != reflect.String || !fv.CanSet() {
		return "", errors.New("pk must be settable string")
	}

	id := fv.String()
	if id == "" {
		id = uuid.NewString()
		fv.SetString(id)
	}
	return id, nil
}

// >>>>>>>>> CHANGED <<<<<<<<<
func readPrimaryKey(v any, meta *ModelMetadata) (string, error) {
	rv := reflect.ValueOf(v)
	if rv.Kind() != reflect.Pointer || rv.IsNil() {
		return "", errors.New("dst must be non-nil pointer")
	}
	rv = rv.Elem()

	if len(meta.PKFields) == 0 {
		return "", errors.New("no pk field")
	}
	pkFieldName := meta.PKFields[0]
	fv := rv.FieldByName(pkFieldName)

	if fv.Kind() != reflect.String {
		return "", errors.New("pk must be string")
	}
	return fv.String(), nil
}

// >>>>>>>>> CHANGED <<<<<<<<<
func touchTimestamps(v any, meta *ModelMetadata) {
	rv := reflect.ValueOf(v)
	if rv.Kind() != reflect.Pointer || rv.IsNil() {
		return
	}
	rv = rv.Elem()
	now := time.Now().UTC()

	for _, fieldName := range meta.TimestampFields {
		f := rv.FieldByName(fieldName)
		if f.IsValid() && f.CanSet() && f.Type().String() == "time.Time" {
			if strings.EqualFold(fieldName, "UpdatedAt") {
				f.Set(reflect.ValueOf(now))
			} else if strings.EqualFold(fieldName, "CreatedAt") {
				if f.Interface().(time.Time).IsZero() {
					f.Set(reflect.ValueOf(now))
				}
			}
		}
	}
}

// >>>>>>>>> CHANGED <<<<<<<<<
func applyDefaults(v any, meta *ModelMetadata) {
	rv := reflect.ValueOf(v)
	if rv.Kind() != reflect.Pointer || rv.IsNil() {
		return
	}
	rv = rv.Elem()
	now := time.Now().UTC()

	for fieldName, tag := range meta.DefaultFields {
		fv := rv.FieldByName(fieldName)
		if !fv.CanSet() || !isZero(fv) {
			continue
		}
		switch fv.Kind() {
		case reflect.String:
			switch tag {
			case "uuid":
				fv.SetString(uuid.NewString())
			case "now_rfc3339":
				fv.SetString(now.Format(time.RFC3339))
			default:
				fv.SetString(tag)
			}
		case reflect.Int, reflect.Int64:
			switch tag {
			case "unix":
				fv.SetInt(now.Unix())
			case "unixms":
				fv.SetInt(now.UnixMilli())
			default:
				if n, err := parseInt64(tag); err == nil {
					fv.SetInt(n)
				}
			}
		case reflect.Bool:
			if tag == "true" {
				fv.SetBool(true)
			}
			if tag == "false" {
				fv.SetBool(false)
			}
		default:
			if fv.Type().String() == "time.Time" && tag == "now" {
				fv.Set(reflect.ValueOf(now))
			}
		}
	}
}

func isZero(v reflect.Value) bool {
	switch v.Kind() {
	case reflect.String:
		return v.Len() == 0
	case reflect.Bool:
		return !v.Bool()
	case reflect.Int, reflect.Int8, reflect.Int16, reflect.Int32, reflect.Int64:
		return v.Int() == 0
	case reflect.Uint, reflect.Uint8, reflect.Uint16, reflect.Uint32, reflect.Uint64, reflect.Uintptr:
		return v.Uint() == 0
	case reflect.Float32, reflect.Float64:
		return v.Float() == 0
	case reflect.Pointer, reflect.Interface, reflect.Slice, reflect.Map:
		return v.IsNil()
	case reflect.Struct:
		if v.Type().String() == "time.Time" {
			return v.Interface().(time.Time).IsZero()
		}
	}
	zero := reflect.Zero(v.Type()).Interface()
	return reflect.DeepEqual(v.Interface(), zero)
}

func parseInt64(s string) (int64, error) {
	var n int64
	_, err := fmt.Sscanf(s, "%d", &n)
	return n, err
}

func versionPointer(v any) (*int64, int) {
	rv := reflect.ValueOf(v)
	if rv.Kind() != reflect.Pointer || rv.IsNil() {
		return nil, -1
	}
	rv = rv.Elem()
	rt := rv.Type()
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
	if ptr == nil {
		return
	}
	rv := reflect.ValueOf(v)
	if rv.Kind() != reflect.Pointer || rv.IsNil() {
		return
	}
	rv = rv.Elem()
	rv.Field(idx).SetInt(val)
}

func toJSONNative(v reflect.Value) any {
	if v.Kind() == reflect.Pointer {
		if v.IsNil() {
			return nil
		}
		return toJSONNative(v.Elem())
	}
	switch v.Kind() {
	case reflect.String:
		return v.String()
	case reflect.Bool:
		return v.Bool()
	case reflect.Int, reflect.Int8, reflect.Int16, reflect.Int32, reflect.Int64:
		return v.Int()
	case reflect.Uint, reflect.Uint8, reflect.Uint16, reflect.Uint32, reflect.Uint64, reflect.Uintptr:
		return v.Uint()
	case reflect.Float32, reflect.Float64:
		return v.Float()
	case reflect.Slice:
		if v.Type().Elem().Kind() == reflect.Uint8 {
			return v.Bytes()
		}
		n := v.Len()
		a := make([]any, n)
		for i := 0; i < n; i++ {
			a[i] = toJSONNative(v.Index(i))
		}
		return a
	case reflect.Map:
		iter := v.MapRange()
		m := make(map[string]any)
		for iter.Next() {
			m[fmt.Sprint(iter.Key().Interface())] = toJSONNative(iter.Value())
		}
		return m
	case reflect.Struct:
		return v.Interface()
	default:
		return v.Interface()
	}
}