package redisorm

import (
	"context"
	"encoding/json"
	"fmt"
	"reflect"
	"strings"

	"github.com/redis/go-redis/v9" // >>>>>>>>> FIX: ADDED MISSING IMPORT <<<<<<<<<
)

// >>>>>>>>> CHANGED <<<<<<<<<
// getOrMakeObjectDEK gets or creates a single Data Encryption Key for an entire object.
func (c *Client) getOrMakeObjectDEK(ctx context.Context, model, id string) ([]byte, error) {
	key := c.keyDEKObject(model, id) // Use the new per-object key
	if wrapped, err := c.rdb.Get(ctx, key).Result(); err == nil && wrapped != "" {
		return unwrapDEK(c.kek, wrapped)
	}
	dek, err := randBytes(32) // AES-256
	if err != nil {
		return nil, err
	}
	if err := c.rdb.Set(ctx, key, wrapDEK(c.kek, dek), 0).Err(); err != nil {
		return nil, err
	}
	return dek, nil
}

func (c *Client) buildEncryptedMap(ctx context.Context, v any, meta *ModelMetadata, id string) (map[string]any, error) {
	rv := reflect.ValueOf(v)
	if rv.Kind() == reflect.Pointer {
		rv = rv.Elem()
	}
	rt := rv.Type()
	out := make(map[string]any, rt.NumField())

	var dek []byte
	var dekErr error
	dekFetched := false

	for i := 0; i < rt.NumField(); i++ {
		f := rt.Field(i)
		if f.PkgPath != "" {
			continue
		}
		jsonName := meta.JsonNames[f.Name]
		if jsonName == "" || jsonName == "-" {
			continue
		}

		val := rv.Field(i)
		isSecret := false
		for _, secretField := range meta.SecretFields {
			if f.Name == secretField {
				isSecret = true
				break
			}
		}

		if isSecret {
			if val.Kind() != reflect.String {
				return nil, fmt.Errorf("secret field %s must be string", f.Name)
			}
			plain := val.String()
			if plain == "" {
				out[jsonName] = ""
				continue
			}

			if !dekFetched {
				dek, dekErr = c.getOrMakeObjectDEK(ctx, meta.StructName, id)
				if dekErr != nil {
					return nil, fmt.Errorf("get DEK for object %s: %w", id, dekErr)
				}
				dekFetched = true
			}

			ct, err := aesGCMEncrypt(dek, []byte(plain))
			if err != nil {
				return nil, fmt.Errorf("encrypt %s: %w", f.Name, err)
			}
			out[jsonName] = ct
			continue
		}
		out[jsonName] = toJSONNative(val)
	}
	return out, nil
}

func (c *Client) decryptForType(ctx context.Context, meta *ModelMetadata, id, encJSON string) ([]byte, error) {
	var m map[string]any
	if err := json.Unmarshal([]byte(encJSON), &m); err != nil {
		return nil, fmt.Errorf("invalid stored JSON: %w", err)
	}

	if len(meta.SecretFields) == 0 {
		return []byte(encJSON), nil
	}

	var dek []byte
	var dekErr error
	dekFetched := false

	// Create a map from json name back to field name for easy lookup
	jsonToField := make(map[string]string)
	for f, j := range meta.JsonNames {
		jsonToField[j] = f
	}

	for _, fieldName := range meta.SecretFields {
		jsonName := meta.JsonNames[fieldName]
		if raw, ok := m[jsonName]; ok {
			if s, ok := raw.(string); ok && strings.HasPrefix(s, fieldEncPrefix) {
				if !dekFetched {
					dek, dekErr = c.getOrMakeObjectDEK(ctx, meta.StructName, id)
					if dekErr != nil {
						// If key is not found, maybe it's an old object. Tolerate.
						if dekErr == redis.Nil {
							continue
						}
						return nil, fmt.Errorf("DEK for object %s: %w", id, dekErr)
					}
					dekFetched = true
				}
				if dek == nil {
					continue
				}

				plain, err := aesGCMDecrypt(dek, s)
				if err != nil {
					return nil, fmt.Errorf("decrypt %s: %w", fieldName, err)
				}
				m[jsonName] = string(plain)
			}
		}
	}

	rebuilt, err := json.Marshal(m)
	if err != nil {
		return nil, err
	}
	return rebuilt, nil
}

// >>>>>>>>> NEW HELPER FUNCTION <<<<<<<<<
// encryptUpdateMap encrypts fields in an update map that are marked as secret.
func (c *Client) encryptUpdateMap(ctx context.Context, meta *ModelMetadata, id string, updates map[string]any) (map[string]any, error) {
	if len(meta.SecretFields) == 0 {
		return updates, nil
	}

	encrypted := make(map[string]any, len(updates))
	for k, v := range updates {
		encrypted[k] = v
	}

	var dek []byte
	var dekErr error
	dekFetched := false

	for jsonName, plainVal := range encrypted {
		isSecret := false
		for _, sf := range meta.SecretFields {
			if meta.JsonNames[sf] == jsonName {
				isSecret = true
				break
			}
		}

		if isSecret {
			if plainStr, isStr := plainVal.(string); isStr && plainStr != "" {
				if !dekFetched {
					dek, dekErr = c.getOrMakeObjectDEK(ctx, meta.StructName, id)
					if dekErr != nil {
						return nil, fmt.Errorf("get DEK for object %s: %w", id, dekErr)
					}
					dekFetched = true
				}
				ct, err := aesGCMEncrypt(dek, []byte(plainStr))
				if err != nil {
					return nil, fmt.Errorf("encrypt update for %s: %w", jsonName, err)
				}
				encrypted[jsonName] = ct
			}
		}
	}
	return encrypted, nil
}
