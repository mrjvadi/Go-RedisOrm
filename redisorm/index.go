package redisorm

import (
	"encoding/json"
	"fmt"
	"reflect"
	"strings"
)

func extractIndexable(sample any, plain []byte) map[string]string {
	rt := reflect.TypeOf(sample); for rt.Kind() == reflect.Pointer { rt = rt.Elem() }
	idx := map[string]string{}
	var m map[string]any; _ = json.Unmarshal(plain, &m)
	for i := 0; i < rt.NumField(); i++ {
		f := rt.Field(i)
		tag := f.Tag.Get("redis")
		if strings.Contains(tag, ",index") || tag == "index" || strings.Contains(tag, ",index_enc") || tag == "index_enc" {
			if strings.Contains(tag, "index_enc") { continue } // ایندکس رمز‌شده جداگانه
			jn := f.Name
			if jt := f.Tag.Get("json"); jt != "" {
				name := strings.Split(jt, ",")[0]
				if name != "" && name != "-" { jn = name }
			}
			if v, ok := m[jn]; ok { idx[f.Name] = fmt.Sprint(v) }
		}
	}
	return idx
}

func extractEncIndex(c *Client, sample any, plain []byte) map[string]string {
	rt := reflect.TypeOf(sample); for rt.Kind() == reflect.Pointer { rt = rt.Elem() }
	encIdx := map[string]string{}
	var m map[string]any; _ = json.Unmarshal(plain, &m)
	for i := 0; i < rt.NumField(); i++ {
		f := rt.Field(i)
		tag := f.Tag.Get("redis")
		if !(strings.Contains(tag, ",index_enc") || tag == "index_enc") { continue }
		if f.Tag.Get("secret") != "true" || f.Type.Kind() != reflect.String { continue }
		jn := f.Name
		if jt := f.Tag.Get("json"); jt != "" {
			name := strings.Split(jt, ",")[0]
			if name != "" && name != "-" { jn = name }
		}
		if v, ok := m[jn]; ok { encIdx[f.Name] = macString(c.kek, fmt.Sprint(v)) }
	}
	return encIdx
}

func extractUnique(sample any, plain []byte) map[string]string {
	rt := reflect.TypeOf(sample); for rt.Kind() == reflect.Pointer { rt = rt.Elem() }
	uniq := map[string]string{}
	var m map[string]any; _ = json.Unmarshal(plain, &m)
	for i := 0; i < rt.NumField(); i++ {
		f := rt.Field(i)
		tag := f.Tag.Get("redis")
		if strings.Contains(tag, ",unique") || tag == "unique" {
			jn := f.Name
			if jt := f.Tag.Get("json"); jt != "" {
				name := strings.Split(jt, ",")[0]
				if name != "" && name != "-" { jn = name }
			}
			if v, ok := m[jn]; ok { uniq[f.Name] = fmt.Sprint(v) }
		}
	}
	return uniq
}

// diff → لیست کلیدهایی که باید set/remove بشن (برای Lua)
func diffUniqueKeys(c *Client, model string, cur, prev map[string]string) (add, del []string) {
	for f, v := range cur {
		if prev == nil || prev[f] != v { add = append(add, c.keyUniq(model, f, v)) }
	}
	for f, v := range prev {
		if cur == nil || cur[f] != v { del = append(del, c.keyUniq(model, f, v)) }
	}
	return
}
func diffIndexKeys(c *Client, model string, cur, prev map[string]string) (add, rem []string) {
	for f, v := range cur {
		if prev == nil || prev[f] != v { add = append(add, c.keyIdx(model, f, v)) }
	}
	for f, v := range prev {
		if cur == nil || cur[f] != v { rem = append(rem, c.keyIdx(model, f, v)) }
	}
	return
}
func diffEncIndexKeys(c *Client, model string, cur, prev map[string]string) (add, rem []string) {
	for f, v := range cur {
		if prev == nil || prev[f] != v { add = append(add, c.keyIdxEnc(model, f, v)) }
	}
	for f, v := range prev {
		if cur == nil || cur[f] != v { rem = append(rem, c.keyIdxEnc(model, f, v)) }
	}
	return
}
func keysFromMap(c *Client, model string, mp map[string]string, f func(field, val string) string) []string {
	out := make([]string, 0, len(mp))
	for k, v := range mp { out = append(out, f(k, v)) }
	return out
}
