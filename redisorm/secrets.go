package redisorm

import (
	"context"
	"encoding/json"
	"fmt"
	"reflect"
	"strings"
)

// buildEncryptedMap encrypts secret fields using the master key directly.
func (c *Client) buildEncryptedMap(ctx context.Context, v any, meta *ModelMetadata) (map[string]any, error) {
	rv := reflect.ValueOf(v)
	if rv.Kind() == reflect.Pointer {
		rv = rv.Elem()
	}
	rt := rv.Type()
	out := make(map[string]any, rt.NumField())

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

			// >>>>>>>>> SIMPLIFIED: Use master key (kek) directly <<<<<<<<<
			ct, err := aesGCMEncrypt(c.kek, []byte(plain))
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

// decryptForType decrypts secret fields using the master key directly.
func (c *Client) decryptForType(ctx context.Context, meta *ModelMetadata, encJSON string) ([]byte, error) {
	var m map[string]any
	if err := json.Unmarshal([]byte(encJSON), &m); err != nil {
		return nil, fmt.Errorf("invalid stored JSON: %w", err)
	}

	if len(meta.SecretFields) == 0 {
		return []byte(encJSON), nil
	}

	for _, fieldName := range meta.SecretFields {
		jsonName := meta.JsonNames[fieldName]
		if raw, ok := m[jsonName]; ok {
			if s, ok := raw.(string); ok && strings.HasPrefix(s, fieldEncPrefix) {
				// >>>>>>>>> SIMPLIFIED: Use master key (kek) directly <<<<<<<<<
				plain, err := aesGCMDecrypt(c.kek, s)
				if err != nil {
					// Don't fail the whole load if one field fails decryption
					continue
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

// encryptUpdateMap encrypts fields in an update map that are marked as secret.
func (c *Client) encryptUpdateMap(ctx context.Context, meta *ModelMetadata, updates map[string]any) (map[string]any, error) {
	if len(meta.SecretFields) == 0 {
		return updates, nil
	}

	encrypted := make(map[string]any, len(updates))
	for k, v := range updates {
		encrypted[k] = v
	}

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
				// >>>>>>>>> SIMPLIFIED: Use master key (kek) directly <<<<<<<<<
				ct, err := aesGCMEncrypt(c.kek, []byte(plainStr))
				if err != nil {
					return nil, fmt.Errorf("encrypt update for %s: %w", jsonName, err)
				}
				encrypted[jsonName] = ct
			}
		}
	}
	return encrypted, nil
}
