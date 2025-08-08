package redisorm

// unlock via token compare (DEL only if value matches)
const luaUnlock = `
if redis.call("GET", KEYS[1]) == ARGV[1] then
  return redis.call("DEL", KEYS[1])
else
  return 0
end`

// Save: اتمیک — value + uniques + indexes + enc-indexes + (اختیاری) CAS روی version
const luaSave = `
-- KEYS: [verKey, valKey, newUniq..., delUniq..., addIdx..., remIdx..., addIdxEnc..., remIdxEnc...]
-- ARGV: [id, encJSON, ttl_ms, expectedVersion_or_empty, nNewUniq, nDelUniq, nAddIdx, nRemIdx, nAddIdxEnc, nRemIdxEnc]
local idx = 1
local verKey = KEYS[idx]; idx = idx + 1
local valKey = KEYS[idx]; idx = idx + 1
local id = ARGV[1]
local enc = ARGV[2]
local ttl = tonumber(ARGV[3]) or 0
local expected = tostring(ARGV[4])
local nNewUniq = tonumber(ARGV[5]) or 0
local nDelUniq = tonumber(ARGV[6]) or 0
local nAddIdx = tonumber(ARGV[7]) or 0
local nRemIdx = tonumber(ARGV[8]) or 0
local nAddIdxEnc = tonumber(ARGV[9]) or 0
local nRemIdxEnc = tonumber(ARGV[10]) or 0

-- version check only (set at end)
if expected ~= nil and expected ~= '' then
  local cur = tonumber(redis.call('GET', verKey) or '0')
  if cur ~= tonumber(expected) then return redis.error_reply('VERSION_CONFLICT') end
end

-- uniqueness check (read-only)
for i=0,nNewUniq-1 do
  local k = KEYS[idx + i]
  local v = redis.call('GET', k)
  if v and v ~= id then return redis.error_reply('UNIQUE_CONFLICT') end
end
idx = idx + nNewUniq

-- apply value
if ttl > 0 then
  redis.call('PSETEX', valKey, ttl, enc)
else
  redis.call('SET', valKey, enc)
end

-- set new uniques
for i=0,nNewUniq-1 do
  local k = KEYS[idx - nNewUniq + i]
  redis.call('SET', k, id)
end

-- delete old uniques (only if owned by this id)
for i=0,nDelUniq-1 do
  local k = KEYS[idx + i]
  if redis.call('GET', k) == id then redis.call('DEL', k) end
end
idx = idx + nDelUniq

-- add idx
for i=0,nAddIdx-1 do
  redis.call('SADD', KEYS[idx + i], id)
end
idx = idx + nAddIdx
-- rem idx
for i=0,nRemIdx-1 do
  redis.call('SREM', KEYS[idx + i], id)
end
idx = idx + nRemIdx
-- add enc idx
for i=0,nAddIdxEnc-1 do
  redis.call('SADD', KEYS[idx + i], id)
end
idx = idx + nAddIdxEnc
-- rem enc idx
for i=0,nRemIdxEnc-1 do
  redis.call('SREM', KEYS[idx + i], id)
end

-- set new version after success
if expected ~= nil and expected ~= '' then
  redis.call('SET', verKey, tonumber(expected) + 1)
end
return id
`

// Delete: اتمیک — حذف value + پاکسازی ایندکس‌ها و یونیک‌ها (و حذف نسخه در صورت نیاز)
const luaDelete = `
-- KEYS: [verKey, valKey, delUniq..., remIdx..., remIdxEnc...]
-- ARGV: [id, expectedVersion_or_empty, removeVer(0/1), nDelUniq, nRemIdx, nRemIdxEnc]
local idx = 1
local verKey = KEYS[idx]; idx = idx + 1
local valKey = KEYS[idx]; idx = idx + 1
local id = ARGV[1]
local expected = tostring(ARGV[2])
local rmver = tostring(ARGV[3])
local nDelUniq = tonumber(ARGV[4]) or 0
local nRemIdx = tonumber(ARGV[5]) or 0
local nRemIdxEnc = tonumber(ARGV[6]) or 0

if expected ~= nil and expected ~= '' then
  local cur = tonumber(redis.call('GET', verKey) or '0')
  if cur ~= tonumber(expected) then return redis.error_reply('VERSION_CONFLICT') end
end

redis.call('DEL', valKey)

for i=0,nDelUniq-1 do
  local k = KEYS[idx + i]
  if redis.call('GET', k) == id then redis.call('DEL', k) end
end
idx = idx + nDelUniq
for i=0,nRemIdx-1 do redis.call('SREM', KEYS[idx + i], id) end
idx = idx + nRemIdx
for i=0,nRemIdxEnc-1 do redis.call('SREM', KEYS[idx + i], id) end

if rmver == '1' then redis.call('DEL', verKey) end
return 1
`

// Payload save: ست مقدار و (در صورت ارسال) ست DEK اگر نبود
const luaPayloadSave = `
-- KEYS: [pkey, dekKey]
-- ARGV: [val, ttl_ms, wrappedDEK_if_any]
local pkey = KEYS[1]
local dkey = KEYS[2]
local val = ARGV[1]
local ttl = tonumber(ARGV[2]) or 0
local wrapped = ARGV[3]
if wrapped and wrapped ~= '' and redis.call('EXISTS', dkey) == 0 then
  redis.call('SET', dkey, wrapped)
end
if ttl > 0 then
  redis.call('PSETEX', pkey, ttl, val)
else
  redis.call('SET', pkey, val)
end
return 1
`
