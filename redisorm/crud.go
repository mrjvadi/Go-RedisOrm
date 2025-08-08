package redisorm

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/redis/go-redis/v9"
)

// Save atomically saves a value, including its indexes and unique constraints.
func (c *Client) Save(ctx context.Context, v any, ttl ...time.Duration) (string, error) {
	if v == nil {
		return "", errors.New("nil value")
	}

	meta, err := c.getModelMetadata(v)
	if err != nil {
		return "", err
	}

	if d, ok := v.(Defaultable); ok {
		d.SetDefaults()
	}
	applyDefaults(v, meta)
	id, err := ensurePrimaryKey(v, meta)
	if err != nil {
		return "", err
	}
	touchTimestamps(v, meta)

	valKey := c.keyVal(meta.StructName, id)
	verKey := c.keyVer(meta.StructName, id)

	plain, err := json.Marshal(v)
	if err != nil {
		return "", fmt.Errorf("marshal plain: %w", err)
	}
	newIdx := extractIndexable(v, plain, meta)
	newUniq := extractUnique(v, plain, meta)
	newIdxEnc := extractEncIndex(c, v, plain, meta)

	var oldIdx, oldUniq, oldIdxEnc map[string]string
	if encOld, _ := c.rdb.Get(ctx, valKey).Result(); encOld != "" {
		if oldPlain, _ := c.decryptForType(ctx, meta, id, encOld); len(oldPlain) > 0 {
			oldIdx = extractIndexable(v, oldPlain, meta)
			oldUniq = extractUnique(v, oldPlain, meta)
			oldIdxEnc = extractEncIndex(c, v, oldPlain, meta)
		}
	}

	encMap, err := c.buildEncryptedMap(ctx, v, meta, id)
	if err != nil {
		return "", err
	}
	encJSON, err := json.Marshal(encMap)
	if err != nil {
		return "", fmt.Errorf("marshal enc: %w", err)
	}

	addUniq, delUniq := diffUniqueKeys(c, meta.StructName, newUniq, oldUniq)
	addIdx, remIdx := diffIndexKeys(c, meta.StructName, newIdx, oldIdx)
	addIdxEnc, remIdxEnc := diffEncIndexKeys(c, meta.StructName, newIdxEnc, oldIdxEnc)

	var exp time.Duration
	if len(ttl) > 0 {
		exp = ttl[0]
	}

	keys := make([]string, 0, 2+len(addUniq)+len(delUniq)+len(addIdx)+len(remIdx)+len(addIdxEnc)+len(remIdxEnc))
	keys = append(keys, verKey, valKey)
	keys = append(keys, addUniq...)
	keys = append(keys, delUniq...)
	keys = append(keys, addIdx...)
	keys = append(keys, remIdx...)
	keys = append(keys, addIdxEnc...)
	keys = append(keys, remIdxEnc...)

	argv := []interface{}{
		id,
		string(encJSON),
		int64(exp.Milliseconds()),
		"", // expectedVersion empty → no CAS
		len(addUniq),
		len(delUniq),
		len(addIdx),
		len(remIdx),
		len(addIdxEnc),
		len(remIdxEnc),
	}

	_, err = c.luaSave.Run(ctx, c.rdb, keys, argv...).Result()
	if err != nil {
		return "", err
	}
	return id, nil
}

// ... (SaveOptimistic can be refactored similarly to Save)

func (c *Client) Load(ctx context.Context, dst any, id string) error {
	if dst == nil {
		return errors.New("nil dst")
	}
	meta, err := c.getModelMetadata(dst)
	if err != nil {
		return err
	}

	if id == "" {
		id, err = readPrimaryKey(dst, meta)
		if err != nil || id == "" {
			return errors.New("empty pk for Load")
		}
	}

	valKey := c.keyVal(meta.StructName, id)
	encJSON, err := c.rdb.Get(ctx, valKey).Result()
	if err != nil {
		return err
	}
	plain, err := c.decryptForType(ctx, meta, id, encJSON)
	if err != nil {
		return err
	}
	return json.Unmarshal(plain, dst)
}

// ... (Delete can be refactored similarly)

// UpdateFields loads the object, applies changes and saves it. Manages indexes.
// This can be slow. For performance-critical updates on non-indexed fields, use UpdateFieldsFast.
func (c *Client) UpdateFields(ctx context.Context, dst any, id string, updates map[string]any) (string, error) {
	meta, err := c.getModelMetadata(dst)
	if err != nil {
		return "", err
	}
	if id == "" {
		id, err = readPrimaryKey(dst, meta)
		if err != nil || id == "" {
			return "", errors.New("empty pk for UpdateFields")
		}
	}
	if err := c.Load(ctx, dst, id); err != nil {
		return "", err
	}
	applyUpdatesByJSONName(dst, updates)
	return c.Save(ctx, dst)
}

// >>>>>>>>> NEW FUNCTION <<<<<<<<<
// UpdateFieldsFast performs a very fast, partial update on the stored JSON.
// WARNING: This method does NOT update indexes. Use it only for fields that are not
// tagged with 'index', 'index_enc', or 'unique'.
func (c *Client) UpdateFieldsFast(ctx context.Context, sample any, id string, updates map[string]any) error {
	meta, err := c.getModelMetadata(sample)
	if err != nil {
		return err
	}
	if id == "" {
		return errors.New("empty id for UpdateFieldsFast")
	}

	valKey := c.keyVal(meta.StructName, id)

	// Encrypt any secret fields in the update map
	encryptedUpdates, err := c.encryptUpdateMap(ctx, meta, id, updates)
	if err != nil {
		return fmt.Errorf("could not encrypt updates: %w", err)
	}

	updatesJson, err := json.Marshal(encryptedUpdates)
	if err != nil {
		return err
	}

	_, err = c.luaUpdateFieldsFast.Run(ctx, c.rdb, []string{valKey}, string(updatesJson)).Result()
	if err != nil {
		if strings.Contains(err.Error(), "NOT_FOUND") {
			return redis.Nil
		}
		return err
	}
	return nil
}

// Other functions like Exists, PageIDsByIndex etc. can be refactored to use meta cache...