# Redis data migration

Move alert history from a local Redis instance to a remote one (e.g. a bastion host). Useful when you developed locally and want the dashboard on a shared server without re-ingesting alerts.

## Redis keys

| Key | Type | Needed for dashboard? |
|-----|------|----------------------|
| `kcontext:alerts` | List | **Yes** — JSON `StoredAlert` records (newest first) |
| `kcontext:active-fingerprints` | Set | No — poll dedup only |
| `kcontext:fp-state` | Hash | No — poll dedup only |
| `kcontext:fp-meta` | Hash | No — resolved detection for polling |

For dashboard history, copying **`kcontext:alerts`** is enough. Copy the other three only if you also run Alertmanager polling on the destination and want dedup/resolved state preserved.

## Why not DUMP / RESTORE?

`redis-cli DUMP` + `RESTORE` fails across **different Redis major versions** (e.g. 7.2 → 6.2) with:

```text
ERR DUMP payload version or checksum are wrong
```

Use the logical copy below instead — it works across versions because each alert is plain JSON text.

## Copy alerts (dashboard data)

Set `REMOTE` to your destination host, then export, transfer, and import:

```bash
REMOTE=root@your-bastion.example.com

# export from local Redis
redis-cli --raw LRANGE kcontext:alerts 0 -1 > /tmp/kcontext_alerts.txt

# transfer to remote
scp /tmp/kcontext_alerts.txt "$REMOTE:/tmp/"

# import on remote (replaces existing kcontext:alerts)
ssh "$REMOTE" 'redis-cli DEL kcontext:alerts
while IFS= read -r line; do
  [ -n "$line" ] && redis-cli LPUSH kcontext:alerts "$line"
done < /tmp/kcontext_alerts.txt
rm /tmp/kcontext_alerts.txt'
```

`LRANGE` returns newest-first; `LPUSH` preserves that order on the destination.

## Verify

```bash
redis-cli LLEN kcontext:alerts
ssh "$REMOTE" redis-cli LLEN kcontext:alerts
```

Counts should match. On the remote host, start KContext with `REDIS_ADDR` pointing at that Redis instance and open `/?range=7d`.

## Optional: copy poll state

Only needed when running Alertmanager polling on the destination.

**Set** (`kcontext:active-fingerprints`):

```bash
redis-cli SMEMBERS kcontext:active-fingerprints > /tmp/fps.txt
scp /tmp/fps.txt "$REMOTE:/tmp/"
ssh "$REMOTE" 'redis-cli DEL kcontext:active-fingerprints
while IFS= read -r fp; do
  [ -n "$fp" ] && redis-cli SADD kcontext:active-fingerprints "$fp"
done < /tmp/fps.txt'
```

**Hashes** (`kcontext:fp-state`, `kcontext:fp-meta`):

```bash
for key in kcontext:fp-state kcontext:fp-meta; do
  redis-cli EXISTS "$key" | grep -q '^1$' || continue
  redis-cli HGETALL "$key" > "/tmp/${key//:/_}.txt"
  scp "/tmp/${key//:/_}.txt" "$REMOTE:/tmp/hash.txt"
  ssh "$REMOTE" "redis-cli DEL '$key'
while read -r field && read -r val; do
  [ -n \"\$field\" ] && redis-cli HSET '$key' \"\$field\" \"\$val\"
done < /tmp/hash.txt"
done
```
