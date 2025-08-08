package redisorm

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"reflect"
	"strings"
	"time"

	"github.com/redis/go-redis/v9"
)

// prepareSaveInternal منطق اصلی آماده‌سازی یک شیء برای ذخیره را در خود دارد.
// این تابع برای جلوگیری از تکرار کد بین Save و SaveAll استفاده می‌شود.
func (c *Client) prepareSaveInternal(ctx context.Context, v any, expectedVersion any, ttl ...time.Duration) (string, []string, []interface{}, error) {
	meta, err := c.getModelMetadata(v)
	if err != nil {
		return "", nil, nil, err
	}

	if d, ok := v.(Defaultable); ok {
		d.SetDefaults()
	}
	applyDefaults(v, meta)
	id, err := ensurePrimaryKey(v, meta)
	if err != nil {
		return "", nil, nil, err
	}
	touchTimestamps(v, meta)

	valKey := c.keyVal(meta.StructName, id)
	verKey := c.keyVer(meta.StructName, id)

	plain, err := json.Marshal(v)
	if err != nil {
		return "", nil, nil, fmt.Errorf("marshal plain: %w", err)
	}
	newIdx := extractIndexable(v, plain, meta)
	newUniq := extractUnique(v, plain, meta)
	newIdxEnc := extractEncIndex(c, v, plain, meta)

	var oldIdx, oldUniq, oldIdxEnc map[string]string
	if encOld, _ := c.rdb.Get(ctx, valKey).Result(); encOld != "" {
		if oldPlain, _ := c.decryptForType(ctx, meta, encOld); len(oldPlain) > 0 {
			oldIdx = extractIndexable(v, oldPlain, meta)
			oldUniq = extractUnique(v, oldPlain, meta)
			oldIdxEnc = extractEncIndex(c, v, oldPlain, meta)
		}
	}

	encMap, err := c.buildEncryptedMap(ctx, v, meta)
	if err != nil {
		return "", nil, nil, err
	}
	encJSON, err := json.Marshal(encMap)
	if err != nil {
		return "", nil, nil, fmt.Errorf("marshal enc: %w", err)
	}

	addUniq, delUniq := diffUniqueKeys(c, meta.StructName, newUniq, oldUniq)
	addIdx, remIdx := diffIndexKeys(c, meta.StructName, newIdx, oldIdx)
	addIdxEnc, remIdxEnc := diffEncIndexKeys(c, meta.StructName, newIdxEnc, oldIdxEnc)

	var exp time.Duration
	if len(ttl) > 0 {
		exp = ttl[0]
	}

	keys := make([]string, 0, 2+len(addUniq)+len(delUniq)+len(addIdx)+len(remIdx)+len(addIdxEnc)+len(remIdxEnc))
	keys = append(keys, verKey, valKey, addUniq...)
	keys = append(keys, delUniq...)
	keys = append(keys, addIdx...)
	keys = append(keys, remIdx...)
	keys = append(keys, addIdxEnc...)
	keys = append(keys, remIdxEnc...)

	argv := []interface{}{
		id, string(encJSON), int64(exp.Milliseconds()),
		expectedVersion,
		len(addUniq), len(delUniq),
		len(addIdx), len(remIdx),
		len(addIdxEnc), len(remIdxEnc),
	}

	return id, keys, argv, nil
}

func (c *Client) Save(ctx context.Context, v any, ttl ...time.Duration) (string, error) {
	if v == nil {
		return "", errors.New("nil value")
	}
	id, keys, argv, err := c.prepareSaveInternal(ctx, v, "", ttl...)
	if err != nil {
		return "", err
	}

	_, err = c.luaSave.Run(ctx, c.rdb, keys, argv...).Result()
	if err != nil {
		return "", err
	}
	return id, nil
}

// SaveAll یک اسلایس از اشیاء را با استفاده از Redis pipeline برای عملکرد بالا ذخیره می‌کند.
func (c *Client) SaveAll(ctx context.Context, slice any) ([]string, error) {
	rv := reflect.ValueOf(slice)
	if rv.Kind() != reflect.Slice {
		return nil, errors.New("input must be a slice of pointers to structs")
	}
	count := rv.Len()
	if count == 0 {
		return []string{}, nil
	}

	pipe := c.rdb.Pipeline()
	ids := make([]string, count)
	cmds := make([]*redis.StringCmd, count)

	for i := 0; i < count; i++ {
		v := rv.Index(i).Interface()
		id, keys, argv, err := c.prepareSaveInternal(ctx, v, "")
		if err != nil {
			return nil, fmt.Errorf("error preparing item %d: %w", i, err)
		}
		ids[i] = id
		cmds[i] = c.luaSave.Run(ctx, pipe, keys, argv...)
	}

	if _, err := pipe.Exec(ctx); err != nil {
		return nil, fmt.Errorf("pipeline execution failed: %w", err)
	}

	for i, cmd := range cmds {
		if err := cmd.Err(); err != nil {
			// تلاش برای برگرداندن یک پیام خطای مفیدتر
			if strings.Contains(err.Error(), "UNIQUE_CONFLICT") {
				return nil, fmt.Errorf("unique constraint violation on item %d (id: %s)", i, ids[i])
			}
			return nil, fmt.Errorf("failed to save item %d (id: %s): %w", i, ids[i], err)
		}
	}

	return ids, nil
}

func (c *Client) SaveOptimistic(ctx context.Context, v any, ttl ...time.Duration) (string, error) {
	if v == nil {
		return "", errors.New("nil value")
	}
	vp, _ := versionPointer(v)
	if vp == nil {
		return "", errors.New("no Version int64 field for optimistic save")
	}
	expectedVersion := *vp
	setVersion(v, expectedVersion+1)

	id, keys, argv, err := c.prepareSaveInternal(ctx, v, expectedVersion, ttl...)
	if err != nil {
		return "", err
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

// ... (سایر توابع CRUD مانند Load, Delete, UpdateFields و غیره در اینجا قرار دارند)
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
	plain, err := c.decryptForType(ctx, meta, encJSON)
	if err != nil {
		return err
	}
	return json.Unmarshal(plain, dst)
}

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
		if plain, _ := c.decryptForType(ctx, meta, encJSON); len(plain) > 0 {
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
	argv := []interface{}{id, "", 1, len(delUniq), len(remIdx), len(remIdxEnc)}
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
	encryptedUpdates, err := c.encryptUpdateMap(ctx, meta, updates)
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

func (c *Client) SavePayload(ctx context.Context, sample any, id string, payload any, encrypt bool, ttl ...time.Duration) error {
	if id == "" {
		return errors.New("empty id")
	}
	meta, err := c.getModelMetadata(sample)
	if err != nil {
		return err
	}
	pkey := c.keyPayload(meta.StructName, id)
	bs, err := json.Marshal(payload)
	if err != nil {
		return err
	}
	if encrypt {
		ct, err := aesGCMEncrypt(c.kek, bs)
		if err != nil {
			return err
		}
		bs = []byte(ct)
	}
	var exp time.Duration
	if len(ttl) > 0 {
		exp = ttl[0]
	}
	_, err = c.luaPayloadSave.Run(ctx, c.rdb, []string{pkey}, string(bs), int64(exp.Milliseconds())).Result()
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
	if decrypt && strings.HasPrefix(val, fieldEncPrefix) {
		plain, err := aesGCMDecrypt(c.kek, val)
		if err != nil {
			return nil, err
		}
		return plain, nil
	}
	return []byte(val), nil
}

// >>>>>>>>> CHANGED <<<<<<<<<
// Touch زمان انقضای (TTL) یک کلید را تمدید می‌کند.
// این تابع اکنون به جای یک نمونه struct، نام مدل را به عنوان رشته دریافت می‌کند.
func (c *Client) Touch(ctx context.Context, modelName string, id string, ttl time.Duration) error {
	if id == "" {
		return errors.New("empty id")
	}
	if ttl <= 0 {
		return errors.New("ttl must be > 0")
	}
	if modelName == "" {
		return errors.New("modelName cannot be empty")
	}
	key := c.keyVal(modelName, id)
	// با استفاده از EXISTS اطمینان حاصل می‌کنیم که کلید وجود دارد.
	// دستور EXPIRE اگر کلید وجود نداشته باشد 0 برمی‌گرداند که می‌تواند گمراه‌کننده باشد.
	exists, err := c.rdb.Exists(ctx, key).Result()
	if err != nil {
		return err
	}
	if exists == 0 {
		return redis.Nil // مانند GET، اگر کلید پیدا نشد Nil برگردان.
	}
	return c.rdb.Expire(ctx, key, ttl).Err()
}

// >>>>>>>>> CHANGED <<<<<<<<<
// TouchPayload زمان انقضای (TTL) یک payload را تمدید می‌کند.
func (c *Client) TouchPayload(ctx context.Context, modelName string, id string, ttl time.Duration) error {
	if id == "" {
		return errors.New("empty id")
	}
	if ttl <= 0 {
		return errors.New("ttl must be > 0")
	}
	if modelName == "" {
		return errors.New("modelName cannot be empty")
	}
	key := c.keyPayload(modelName, id)
	exists, err := c.rdb.Exists(ctx, key).Result()
	if err != nil {
		return err
	}
	if exists == 0 {
		return redis.Nil
	}
	return c.rdb.Expire(ctx, key, ttl).Err()
}
