package redisorm

import (
	"reflect"
	"strings"
)

// ModelMetadata نتایج تحلیل struct را برای جلوگیری از reflection تکراری، کش می‌کند.
type ModelMetadata struct {
	StructName string

	JsonNames map[string]string

	PKFields             []string
	VersionFields        []string
	IndexedFields        []string
	EncIndexedFields     []string
	UniqueFields         []string
	SecretFields         []string
	DefaultFields        map[string]string
	AutoCreateTimeFields []string // >>>>>>>>> NEW <<<<<<<<<
	AutoUpdateTimeFields []string // >>>>>>>>> NEW <<<<<<<<<
}

// getModelMetadata یک struct را تحلیل کرده و نتایج را در کش ذخیره می‌کند.
func (c *Client) getModelMetadata(v any) (*ModelMetadata, error) {
	rt := reflect.TypeOf(v)
	for rt.Kind() == reflect.Pointer {
		rt = rt.Elem()
	}

	if meta, ok := c.metaCache.Load(rt); ok {
		return meta.(*ModelMetadata), nil
	}

	meta := &ModelMetadata{
		JsonNames:     make(map[string]string),
		DefaultFields: make(map[string]string),
	}

	// >>>>>>>>> NEW: Check for ModelNamer interface <<<<<<<<<
	// بررسی می‌کند که آیا struct اینترفیس ModelNamer را پیاده‌سازی کرده است یا خیر.
	modelInstance := reflect.New(rt).Interface()
	if namer, ok := modelInstance.(ModelNamer); ok {
		meta.StructName = namer.ModelName()
	} else {
		meta.StructName = rt.Name()
	}

	for i := 0; i < rt.NumField(); i++ {
		f := rt.Field(i)
		if f.PkgPath != "" {
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
		// >>>>>>>>> NEW: Check for lifecycle tags <<<<<<<<<
		if strings.Contains(redisTag, "auto_create_time") {
			meta.AutoCreateTimeFields = append(meta.AutoCreateTimeFields, fieldName)
		}
		if strings.Contains(redisTag, "auto_update_time") {
			meta.AutoUpdateTimeFields = append(meta.AutoUpdateTimeFields, fieldName)
		}

		if f.Tag.Get("secret") == "true" {
			meta.SecretFields = append(meta.SecretFields, fieldName)
		}
		if defaultTag := f.Tag.Get("default"); defaultTag != "" {
			meta.DefaultFields[fieldName] = defaultTag
		}
	}

	c.metaCache.Store(rt, meta)
	return meta, nil
}
