package redisorm

// ... (luaUnlock, luaSave, luaDelete scripts remain the same)
const luaUnlock = `
if redis.call("GET", KEYS[1]) == ARGV[1] then
  return redis.call("DEL", KEYS[1])
else
  return 0
end`

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
if expected ~= nil and expected ~= '' then
  local cur = tonumber(redis.call('GET', verKey) or '0')
  if cur ~= tonumber(expected) then return redis.error_reply('VERSION_CONFLICT') end
end
for i=0,nNewUniq-1 do
  local k = KEYS[idx + i]
  local v = redis.call('GET', k)
  if v and v ~= id then return redis.error_reply('UNIQUE_CONFLICT') end
end
idx = idx + nNewUniq
if ttl > 0 then
  redis.call('PSETEX', valKey, ttl, enc)
else
  redis.call('SET', valKey, enc)
end
for i=0,nNewUniq-1 do
  local k = KEYS[idx - nNewUniq + i]
  redis.call('SET', k, id)
end
for i=0,nDelUniq-1 do
  local k = KEYS[idx + i]
  if redis.call('GET', k) == id then redis.call('DEL', k) end
end
idx = idx + nDelUniq
for i=0,nAddIdx-1 do
  redis.call('SADD', KEYS[idx + i], id)
end
idx = idx + nAddIdx
for i=0,nRemIdx-1 do
  redis.call('SREM', KEYS[idx + i], id)
end
idx = idx + nRemIdx
for i=0,nAddIdxEnc-1 do
  redis.call('SADD', KEYS[idx + i], id)
end
idx = idx + nAddIdxEnc
for i=0,nRemIdxEnc-1 do
  redis.call('SREM', KEYS[idx + i], id)
end
if expected ~= nil and expected ~= '' then
  redis.call('SET', verKey, tonumber(expected) + 1)
end
return id
`

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

// >>>>>>>>> CHANGED <<<<<<<<<
// Payload save: Simplified to no longer manage a separate DEK.
const luaPayloadSave = `
-- KEYS: [pkey]
-- ARGV: [val, ttl_ms]
local pkey = KEYS[1]
local val = ARGV[1]
local ttl = tonumber(ARGV[2]) or 0
if ttl > 0 then
  redis.call('PSETEX', pkey, ttl, val)
else
  redis.call('SET', pkey, val)
end
return 1
`

const luaUpdateFieldsFast = `
-- KEYS: [valKey]
-- ARGV: [updates_json_string]
local valKey = KEYS[1]
local updatesJson = ARGV[1]
if redis.call("EXISTS", valKey) == 0 then
  return redis.error_reply('NOT_FOUND')
end
local currentJson = redis.call("GET", valKey)
local currentData = cjson.decode(currentJson)
local updatesData = cjson.decode(updatesJson)
for k, v in pairs(updatesData) do
  currentData[k] = v
end
local newJson = cjson.encode(currentData)
redis.call("SET", valKey, newJson)
return 1
`
