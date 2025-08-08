package redisorm

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"
)

// Save اتمیک با Lua: value + unique + index + enc-index
func (c *Client) Save(ctx context.Context, v any, ttl ...time.Duration) (string, error) {
	if v == nil {
		return "", errors.New("nil value")
	}
	if d, ok := v.(Defaultable); ok {
		d.SetDefaults()
	}
	applyDefaults(v)
	id, err := ensurePrimaryKey(v)
	if err != nil {
		return "", err
	}
	touchTimestamps(v)
	model := typeName(v)

	valKey := c.keyVal(model, id)
	verKey := c.keyVer(model, id)

	plain, err := json.Marshal(v)
	if err != nil {
		return "", fmt.Errorf("marshal plain: %w", err)
	}
	newIdx := extractIndexable(v, plain)
	newUniq := extractUnique(v, plain)
	newIdxEnc := extractEncIndex(c, v, plain)

	var oldIdx, oldUniq, oldIdxEnc map[string]string
	if encOld, _ := c.rdb.Get(ctx, valKey).Result(); encOld != "" {
		if oldPlain, _ := c.decryptForType(ctx, model, id, encOld, v); len(oldPlain) > 0 {
			oldIdx = extractIndexable(v, oldPlain)
			oldUniq = extractUnique(v, oldPlain)
			oldIdxEnc = extractEncIndex(c, v, oldPlain)
		}
	}

	encMap, err := c.buildEncryptedMap(ctx, v, model, id)
	if err != nil {
		return "", err
	}
	encJSON, err := json.Marshal(encMap)
	if err != nil {
		return "", fmt.Errorf("marshal enc: %w", err)
	}

	addUniq, delUniq := diffUniqueKeys(c, model, newUniq, oldUniq)
	addIdx, remIdx := diffIndexKeys(c, model, newIdx, oldIdx)
	addIdxEnc, remIdxEnc := diffEncIndexKeys(c, model, newIdxEnc, oldIdxEnc)

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
		"", // expectedVersion empty → بدون CAS
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

// SaveOptimistic: مثل Save ولی با CAS روی ver:<model>:<id> (Version باید در struct باشه)
func (c *Client) SaveOptimistic(ctx context.Context, v any, ttl ...time.Duration) (string, error) {
	if v == nil {
		return "", errors.New("nil value")
	}
	vp, _ := versionPointer(v)
	if vp == nil {
		return "", errors.New("no Version int64 field for optimistic save")
	}

	if d, ok := v.(Defaultable); ok {
		d.SetDefaults()
	}
	applyDefaults(v)
	id, err := ensurePrimaryKey(v)
	if err != nil {
		return "", err
	}
	model := typeName(v)
	valKey := c.keyVal(model, id)
	verKey := c.keyVer(model, id)

	expected := *vp
	setVersion(v, expected+1)
	touchTimestamps(v)

	plain, err := json.Marshal(v)
	if err != nil {
		return "", err
	}
	newIdx := extractIndexable(v, plain)
	newUniq := extractUnique(v, plain)
	newIdxEnc := extractEncIndex(c, v, plain)

	var oldIdx, oldUniq, oldIdxEnc map[string]string
	if encOld, _ := c.rdb.Get(ctx, valKey).Result(); encOld != "" {
		if oldPlain, _ := c.decryptForType(ctx, model, id, encOld, v); len(oldPlain) > 0 {
			oldIdx = extractIndexable(v, oldPlain)
			oldUniq = extractUnique(v, oldPlain)
			oldIdxEnc = extractEncIndex(c, v, oldPlain)
		}
	}

	encMap, err := c.buildEncryptedMap(ctx, v, model, id)
	if err != nil {
		return "", err
	}
	encJSON, err := json.Marshal(encMap)
	if err != nil {
		return "", err
	}

	addUniq, delUniq := diffUniqueKeys(c, model, newUniq, oldUniq)
	addIdx, remIdx := diffIndexKeys(c, model, newIdx, oldIdx)
	addIdxEnc, remIdxEnc := diffEncIndexKeys(c, model, newIdxEnc, oldIdxEnc)

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
	if id == "" {
		var err error
		id, err = readPrimaryKey(dst)
		if err != nil || id == "" {
			return errors.New("empty pk for Load")
		}
	}
	model := typeName(dst)
	valKey := c.keyVal(model, id)
	encJSON, err := c.rdb.Get(ctx, valKey).Result()
	if err != nil {
		return err
	} // redis.Nil اگه نبود
	plain, err := c.decryptForType(ctx, model, id, encJSON, dst)
	if err != nil {
		return err
	}
	return json.Unmarshal(plain, dst)
}

func (c *Client) Delete(ctx context.Context, v any, id string) error {
	model := typeName(v)
	if id == "" {
		var err error
		id, err = readPrimaryKey(v)
		if err != nil || id == "" {
			return errors.New("empty pk for Delete")
		}
	}
	valKey := c.keyVal(model, id)
	verKey := c.keyVer(model, id)

	var oldIdx, oldUniq, oldIdxEnc map[string]string
	if encJSON, _ := c.rdb.Get(ctx, valKey).Result(); encJSON != "" {
		if plain, _ := c.decryptForType(ctx, model, id, encJSON, v); len(plain) > 0 {
			oldIdx = extractIndexable(v, plain)
			oldUniq = extractUnique(v, plain)
			oldIdxEnc = extractEncIndex(c, v, plain)
		}
	}
	delUniq := keysFromMap(c, model, oldUniq, func(field, val string) string { return c.keyUniq(model, field, val) })
	remIdx := keysFromMap(c, model, oldIdx, func(field, val string) string { return c.keyIdx(model, field, val) })
	remIdxEnc := keysFromMap(c, model, oldIdxEnc, func(field, mac string) string { return c.keyIdxEnc(model, field, mac) })

	keys := make([]string, 0, 2+len(delUniq)+len(remIdx)+len(remIdxEnc))
	keys = append(keys, verKey, valKey)
	keys = append(keys, delUniq...)
	keys = append(keys, remIdx...)
	keys = append(keys, remIdxEnc...)

	argv := []interface{}{id, "", 1, len(delUniq), len(remIdx), len(remIdxEnc)} // rmVer=1
	_, err := c.luaDelete.Run(ctx, c.rdb, keys, argv...).Result()
	return err
}

func (c *Client) UpdateFields(ctx context.Context, dst any, id string, updates map[string]any) (string, error) {
	if id == "" {
		var err error
		id, err = readPrimaryKey(dst)
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

func (c *Client) Exists(ctx context.Context, sample any, id string) (bool, error) {
	model := typeName(sample)
	if id == "" {
		return false, errors.New("empty id")
	}
	return c.rdb.Exists(ctx, c.keyVal(model, id)).Val() == 1, nil
}

func (c *Client) PageIDsByIndex(ctx context.Context, sample any, field, value string, cursor uint64, count int64) ([]string, uint64, error) {
	key := c.keyIdx(typeName(sample), field, value)
	ids, next, err := c.rdb.SScan(ctx, key, cursor, "", count).Result()
	return ids, next, err
}
func (c *Client) PageIDsByEncIndex(ctx context.Context, sample any, field, plainValue string, cursor uint64, count int64) ([]string, uint64, error) {
	mac := macString(c.kek, plainValue)
	key := c.keyIdxEnc(typeName(sample), field, mac)
	ids, next, err := c.rdb.SScan(ctx, key, cursor, "", count).Result()
	return ids, next, err
}

// Payload
func (c *Client) SavePayload(ctx context.Context, sample any, id string, payload any, encrypt bool, ttl ...time.Duration) error {
	if id == "" {
		return errors.New("empty id")
	}
	model := typeName(sample)
	pkey := c.keyPayload(model, id)
	dekKey := c.keyPayloadDEK(model, id)

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
	model := typeName(sample)
	pkey := c.keyPayload(model, id)
	val, err := c.rdb.Get(ctx, pkey).Result()
	if err != nil {
		return nil, err
	}
	if !decrypt {
		return []byte(val), nil
	}
	if strings.HasPrefix(val, fieldEncPrefix) {
		dekKey := c.keyPayloadDEK(model, id)
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

// TTL
func (c *Client) Touch(ctx context.Context, sample any, id string, ttl time.Duration) error {
	if id == "" {
		return errors.New("empty id")
	}
	if ttl <= 0 {
		return errors.New("ttl must be > 0")
	}
	model := typeName(sample)
	key := c.keyVal(model, id)
	return c.rdb.Expire(ctx, key, ttl).Err()
}
func (c *Client) TouchPayload(ctx context.Context, sample any, id string, ttl time.Duration) error {
	if id == "" {
		return errors.New("empty id")
	}
	if ttl <= 0 {
		return errors.New("ttl must be > 0")
	}
	model := typeName(sample)
	key := c.keyPayload(model, id)
	return c.rdb.Expire(ctx, key, ttl).Err()
}
