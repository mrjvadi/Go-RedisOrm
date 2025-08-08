package redisorm

import (
	"reflect"
	"strings"
)

// ModelMetadata stores cached reflection results for a struct type.
type ModelMetadata struct {
	StructName string

	// Map of Go field name to its JSON name
	JsonNames map[string]string

	// Lists of Go field names
	PKFields         []string
	VersionFields    []string
	IndexedFields    []string
	EncIndexedFields []string
	UniqueFields     []string
	SecretFields     []string
	DefaultFields    map[string]string // field name -> default tag value
	TimestampFields  []string
}

// getModelMetadata analyzes a struct type using reflection and caches the result.
func (c *Client) getModelMetadata(v any) (*ModelMetadata, error) {
	rt := reflect.TypeOf(v)
	for rt.Kind() == reflect.Pointer {
		rt = rt.Elem()
	}

	if meta, ok := c.metaCache.Load(rt); ok {
		return meta.(*ModelMetadata), nil
	}

	meta := &ModelMetadata{
		StructName:    rt.Name(),
		JsonNames:     make(map[string]string),
		DefaultFields: make(map[string]string),
	}

	for i := 0; i < rt.NumField(); i++ {
		f := rt.Field(i)
		if f.PkgPath != "" { // Skip unexported fields
			continue
		}

		fieldName := f.Name
		jsonName := fieldName
		if tag := f.Tag.Get("json"); tag != "" {
			name := strings.Split(tag, ",")[0]
			if name != "" && name != "-" {
				jsonName = name
			}
		}
		meta.JsonNames[fieldName] = jsonName

		redisTag := f.Tag.Get("redis")
		if redisTag == "pk" || strings.EqualFold(fieldName, "ID") {
			meta.PKFields = append(meta.PKFields, fieldName)
		}
		if redisTag == "version" || strings.EqualFold(fieldName, "Version") {
			meta.VersionFields = append(meta.VersionFields, fieldName)
		}
		if strings.Contains(redisTag, "index_enc") {
			meta.EncIndexedFields = append(meta.EncIndexedFields, fieldName)
		} else if strings.Contains(redisTag, "index") {
			meta.IndexedFields = append(meta.IndexedFields, fieldName)
		}
		if strings.Contains(redisTag, "unique") {
			meta.UniqueFields = append(meta.UniqueFields, fieldName)
		}

		if f.Tag.Get("secret") == "true" {
			meta.SecretFields = append(meta.SecretFields, fieldName)
		}
		if defaultTag := f.Tag.Get("default"); defaultTag != "" {
			meta.DefaultFields[fieldName] = defaultTag
		}
		if strings.EqualFold(fieldName, "CreatedAt") || strings.EqualFold(fieldName, "UpdatedAt") {
			meta.TimestampFields = append(meta.TimestampFields, fieldName)
		}
	}

	c.metaCache.Store(rt, meta)
	return meta, nil
}
