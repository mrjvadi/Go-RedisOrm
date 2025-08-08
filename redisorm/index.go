package redisorm

import (
	"encoding/json"
	"fmt"
)

func extractIndexable(v any, plain []byte, meta *ModelMetadata) map[string]string {
	idx := map[string]string{}
	if len(meta.IndexedFields) == 0 {
		return idx
	}

	var m map[string]any
	_ = json.Unmarshal(plain, &m)

	for _, fieldName := range meta.IndexedFields {
		jsonName := meta.JsonNames[fieldName]
		if v, ok := m[jsonName]; ok {
			idx[fieldName] = fmt.Sprint(v)
		}
	}
	return idx
}

func extractEncIndex(c *Client, v any, plain []byte, meta *ModelMetadata) map[string]string {
	encIdx := map[string]string{}
	if len(meta.EncIndexedFields) == 0 {
		return encIdx
	}

	var m map[string]any
	_ = json.Unmarshal(plain, &m)

	for _, fieldName := range meta.EncIndexedFields {
		jsonName := meta.JsonNames[fieldName]
		if v, ok := m[jsonName]; ok {
			encIdx[fieldName] = macString(c.kek, fmt.Sprint(v))
		}
	}
	return encIdx
}

func extractUnique(v any, plain []byte, meta *ModelMetadata) map[string]string {
	uniq := map[string]string{}
	if len(meta.UniqueFields) == 0 {
		return uniq
	}

	var m map[string]any
	_ = json.Unmarshal(plain, &m)

	for _, fieldName := range meta.UniqueFields {
		jsonName := meta.JsonNames[fieldName]
		if v, ok := m[jsonName]; ok {
			uniq[fieldName] = fmt.Sprint(v)
		}
	}
	return uniq
}

func diffUniqueKeys(c *Client, modelPrefix string, cur, prev map[string]string) (add, del []string) {
	for f, v := range cur {
		if prev == nil || prev[f] != v {
			add = append(add, c.keyUniq(modelPrefix, f, v))
		}
	}
	for f, v := range prev {
		if cur == nil || cur[f] != v {
			del = append(del, c.keyUniq(modelPrefix, f, v))
		}
	}
	return
}

func diffIndexKeys(c *Client, modelPrefix string, cur, prev map[string]string) (add, rem []string) {
	for f, v := range cur {
		if prev == nil || prev[f] != v {
			add = append(add, c.keyIdx(modelPrefix, f, v))
		}
	}
	for f, v := range prev {
		if cur == nil || cur[f] != v {
			rem = append(rem, c.keyIdx(modelPrefix, f, v))
		}
	}
	return
}

func diffEncIndexKeys(c *Client, modelPrefix string, cur, prev map[string]string) (add, rem []string) {
	for f, v := range cur {
		if prev == nil || prev[f] != v {
			add = append(add, c.keyIdxEnc(modelPrefix, f, v))
		}
	}
	for f, v := range prev {
		if cur == nil || cur[f] != v {
			rem = append(rem, c.keyIdxEnc(modelPrefix, f, v))
		}
	}
	return
}

func keysFromMap(c *Client, modelPrefix string, mp map[string]string, f func(prefix, field, val string) string) []string {
	out := make([]string, 0, len(mp))
	for k, v := range mp {
		out = append(out, f(modelPrefix, k, v))
	}
	return out
}