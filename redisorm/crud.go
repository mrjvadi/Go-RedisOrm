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

// SaveOptimistic is like Save but uses Compare-And-Swap on the version field.
func (c *Client) SaveOptimistic(ctx context.Context, v any, ttl ...time.Duration) (string, error) {
	if v == nil {
		return "", errors.New("nil value")
	}
	meta, err := c.getModelMetadata(v)
	if err != nil {
		return "", err
	}

	vp, _ := versionPointer(v)
	if vp == nil {
		return "", errors.New("no Version int64 field for optimistic save")
	}

	if d, ok := v.(Defaultable); ok {
		d.SetDefaults()
	}
	applyDefaults(v, meta)
	id, err := ensurePrimaryKey(v, meta)
	if err != nil {
		return "", err
	}
	valKey := c.keyVal(meta.StructName, id)
	verKey := c.keyVer(meta.StructName, id)

	expected := *vp
	setVersion(v, expected+1)
	touchTimestamps(v, meta)

	plain, err := json.Marshal(v)
	if err != nil {
		return "", err
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
		return "", err
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
		expected, // CAS
		len(addUniq),
		len(delUniq),
		len(addIdx),
		len(remIdx),
		len(addIdxEnc),
		len(remIdxEnc),
	}

	_, err = c.luaSave.Run(ctx, c.rdb, keys, argv...).Result()
	if err != nil {
		if strings.Contains(err.Error(), "VERSION_CONFLICT") {
			return "", ErrVersionConflict
		}
		if strings.Contains(err.Error(), "UNIQUE_CONFLICT") {
			return "", fmt.Errorf("unique constraint violation")
		}
		return "", err
	}
	return id, nil
}

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

// >>>>>>>>> RE-IMPLEMENTED AND FIXED <<<<<<<<<
func (c *Client) Delete(ctx context.Context, v any, id string) error {
	meta, err := c.getModelMetadata(v)
	if err != nil {
		return err
	}
	if id == "" {
		id, err = readPrimaryKey(v, meta)
		if err != nil || id == "" {
			return errors.New("empty pk for Delete")
		}
	}
	valKey := c.keyVal(meta.StructName, id)
	verKey := c.keyVer(meta.StructName, id)

	var oldIdx, oldUniq, oldIdxEnc map[string]string
	if encJSON, _ := c.rdb.Get(ctx, valKey).Result(); encJSON != "" {
		if plain, _ := c.decryptForType(ctx, meta, id, encJSON); len(plain) > 0 {
			oldIdx = extractIndexable(v, plain, meta)
			oldUniq = extractUnique(v, plain, meta)
			oldIdxEnc = extractEncIndex(c, v, plain, meta)
		}
	}
	delUniq := keysFromMap(c, meta.StructName, oldUniq, func(field, val string) string { return c.keyUniq(meta.StructName, field, val) })
	remIdx := keysFromMap(c, meta.StructName, oldIdx, func(field, val string) string { return c.keyIdx(meta.StructName, field, val) })
	remIdxEnc := keysFromMap(c, meta.StructName, oldIdxEnc, func(field, mac string) string { return c.keyIdxEnc(meta.StructName, field, mac) })

	keys := make([]string, 0, 2+len(delUniq)+len(remIdx)+len(remIdxEnc))
	keys = append(keys, verKey, valKey)
	keys = append(keys, delUniq...)
	keys = append(keys, remIdx...)
	keys = append(keys, remIdxEnc...)

	argv := []interface{}{id, "", 1, len(delUniq), len(remIdx), len(remIdxEnc)} // rmVer=1
	_, err = c.luaDelete.Run(ctx, c.rdb, keys, argv...).Result()
	return err
}

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

func (c *Client) UpdateFieldsFast(ctx context.Context, sample any, id string, updates map[string]any) error {
	meta, err := c.getModelMetadata(sample)
	if err != nil {
		return err
	}
	if id == "" {
		return errors.New("empty id for UpdateFieldsFast")
	}

	valKey := c.keyVal(meta.StructName, id)

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

// >>>>>>>>> RE-IMPLEMENTED AND FIXED <<<<<<<<<
func (c *Client) Exists(ctx context.Context, sample any, id string) (bool, error) {
	meta, err := c.getModelMetadata(sample)
	if err != nil {
		return false, err
	}
	if id == "" {
		return false, errors.New("empty id")
	}
	return c.rdb.Exists(ctx, c.keyVal(meta.StructName, id)).Val() == 1, nil
}

func (c *Client) PageIDsByIndex(ctx context.Context, sample any, field, value string, cursor uint64, count int64) ([]string, uint64, error) {
	meta, err := c.getModelMetadata(sample)
	if err != nil {
		return nil, 0, err
	}
	key := c.keyIdx(meta.StructName, field, value)
	ids, next, err := c.rdb.SScan(ctx, key, cursor, "", count).Result()
	return ids, next, err
}

func (c *Client) PageIDsByEncIndex(ctx context.Context, sample any, field, plainValue string, cursor uint64, count int64) ([]string, uint64, error) {
	meta, err := c.getModelMetadata(sample)
	if err != nil {
		return nil, 0, err
	}
	mac := macString(c.kek, plainValue)
	key := c.keyIdxEnc(meta.StructName, field, mac)
	ids, next, err := c.rdb.SScan(ctx, key, cursor, "", count).Result()
	return ids, next, err
}

// >>>>>>>>> RE-IMPLEMENTED AND FIXED <<<<<<<<<
func (c *Client) SavePayload(ctx context.Context, sample any, id string, payload any, encrypt bool, ttl ...time.Duration) error {
	if id == "" {
		return errors.New("empty id")
	}
	meta, err := c.getModelMetadata(sample)
	if err != nil {
		return err
	}
	pkey := c.keyPayload(meta.StructName, id)
	dekKey := c.keyPayloadDEK(meta.StructName, id)

	bs, err := json.Marshal(payload)
	if err != nil {
		return err
	}
	var wrapped string
	if encrypt {
		var dek []byte
		if w, _ := c.rdb.Get(ctx, dekKey).Result(); w != "" {
			dek, err = unwrapDEK(c.kek, w)
			if err != nil {
				return err
			}
		} else {
			dek, err = randBytes(32)
			if err != nil {
				return err
			}
			wrapped = wrapDEK(c.kek, dek)
		}
		ct, err := aesGCMEncrypt(dek, bs)
		if err != nil {
			return err
		}
		bs = []byte(ct)
	}
	var exp time.Duration
	if len(ttl) > 0 {
		exp = ttl[0]
	}

	keys := []string{pkey, dekKey}
	argv := []interface{}{string(bs), int64(exp.Milliseconds()), wrapped}
	_, err = c.luaPayloadSave.Run(ctx, c.rdb, keys, argv...).Result()
	return err
}

func (c *Client) GetPayload(ctx context.Context, sample any, id string, decrypt bool) ([]byte, error) {
	if id == "" {
		return nil, errors.New("empty id")
	}
	meta, err := c.getModelMetadata(sample)
	if err != nil {
		return nil, err
	}
	pkey := c.keyPayload(meta.StructName, id)
	val, err := c.rdb.Get(ctx, pkey).Result()
	if err != nil {
		return nil, err
	}
	if !decrypt {
		return []byte(val), nil
	}
	if strings.HasPrefix(val, fieldEncPrefix) {
		dekKey := c.keyPayloadDEK(meta.StructName, id)
		wrapped, err := c.rdb.Get(ctx, dekKey).Result()
		if err != nil {
			return nil, err
		}
		dek, err := unwrapDEK(c.kek, wrapped)
		if err != nil {
			return nil, err
		}
		plain, err := aesGCMDecrypt(dek, val)
		if err != nil {
			return nil, err
		}
		return plain, nil
	}
	return []byte(val), nil
}

func (c *Client) Touch(ctx context.Context, sample any, id string, ttl time.Duration) error {
	if id == "" {
		return errors.New("empty id")
	}
	if ttl <= 0 {
		return errors.New("ttl must be > 0")
	}
	meta, err := c.getModelMetadata(sample)
	if err != nil {
		return err
	}
	key := c.keyVal(meta.StructName, id)
	return c.rdb.Expire(ctx, key, ttl).Err()
}

func (c *Client) TouchPayload(ctx context.Context, sample any, id string, ttl time.Duration) error {
	if id == "" {
		return errors.New("empty id")
	}
	if ttl <= 0 {
		return errors.New("ttl must be > 0")
	}
	meta, err := c.getModelMetadata(sample)
	if err != nil {
		return err
	}
	key := c.keyPayload(meta.StructName, id)
	return c.rdb.Expire(ctx, key, ttl).Err()
}
