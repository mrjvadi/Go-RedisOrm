package redisorm

import (
	"context"
	"encoding/json"
	"fmt"
	"reflect"
	"strings"
)

// ساخت JSON رمز‌شده: فقط فیلدهای secret:"true" (نوع string) رمز می‌شن
func (c *Client) buildEncryptedMap(ctx context.Context, v any, model, id string) (map[string]any, error) {
	rv := reflect.ValueOf(v); if rv.Kind() == reflect.Pointer { rv = rv.Elem() }
	rt := rv.Type()
	out := make(map[string]any, rt.NumField())

	for i := 0; i < rt.NumField(); i++ {
		f := rt.Field(i)
		if f.PkgPath != "" { continue }
		jsonName := f.Name
		if tag := f.Tag.Get("json"); tag != "" {
			name := strings.Split(tag, ",")[0]
			if name == "-" { continue }
			if name != "" { jsonName = name }
		}
		val := rv.Field(i)

		if f.Tag.Get("secret") == "true" {
			if val.Kind() != reflect.String { return nil, fmt.Errorf("secret field %s must be string", f.Name) }
			plain := val.String()
			if plain == "" { out[jsonName] = ""; continue }
			dek, err := c.getOrMakeFieldDEK(ctx, model, id, f.Name)
			if err != nil { return nil, fmt.Errorf("get DEK for %s: %w", f.Name, err) }
			ct, err := aesGCMEncrypt(dek, []byte(plain))
			if err != nil { return nil, fmt.Errorf("encrypt %s: %w", f.Name, err) }
			out[jsonName] = ct
			continue
		}
		out[jsonName] = toJSONNative(val)
	}
	return out, nil
}

// دیکریپت فقط فیلدهای secret متناسب با نوع مقصد
func (c *Client) decryptForType(ctx context.Context, model, id, encJSON string, dstType any) ([]byte, error) {
	var m map[string]any
	if err := json.Unmarshal([]byte(encJSON), &m); err != nil { return nil, fmt.Errorf("invalid stored JSON: %w", err) }

	rt := reflect.TypeOf(dstType)
	for rt.Kind() == reflect.Pointer { rt = rt.Elem() }
	for i := 0; i < rt.NumField(); i++ {
		f := rt.Field(i)
		if f.PkgPath != "" { continue }
		if f.Tag.Get("secret") != "true" { continue }
		if f.Type.Kind() != reflect.String { continue }

		jsonName := f.Name
		if tag := f.Tag.Get("json"); tag != "" {
			name := strings.Split(tag, ",")[0]
			if name != "" && name != "-" { jsonName = name }
		}
		if raw, ok := m[jsonName]; ok {
			if s, ok := raw.(string); ok && strings.HasPrefix(s, fieldEncPrefix) {
				dek, err := c.getOrMakeFieldDEK(ctx, model, id, f.Name)
				if err != nil { return nil, fmt.Errorf("DEK %s: %w", f.Name, err) }
				plain, err := aesGCMDecrypt(dek, s)
				if err != nil { return nil, fmt.Errorf("decrypt %s: %w", f.Name, err) }
				m[jsonName] = string(plain)
			}
		}
	}
	rebuilt, err := json.Marshal(m)
	if err != nil { return nil, err }
	return rebuilt, nil
}

func (c *Client) getOrMakeFieldDEK(ctx context.Context, model, id, field string) ([]byte, error) {
	key := c.keyDEKField(model, id, field)
	if wrapped, err := c.rdb.Get(ctx, key).Result(); err == nil && wrapped != "" {
		return unwrapDEK(c.kek, wrapped)
	}
	dek, err := randBytes(32) // AES-256
	if err != nil { return nil, err }
	if err := c.rdb.Set(ctx, key, wrapDEK(c.kek, dek), 0).Err(); err != nil { return nil, err }
	return dek, nil
}
