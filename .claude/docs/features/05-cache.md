# 05 — LRU cache with TTL

**Package:** `internal/cache` (`cache.go`)

## What it does
Stores whole responses (JSON, or the complete SSE stream for replay) by key, with a TTL and a limit in
**bytes**. When it fills up, it evicts the least recently used. Each entry stores the original cost, which
becomes "savings" on every hit.

## Why this way (decisions)
- **`container/list` + a map**, from the stdlib: the list gives the usage order in O(1) and the map gives lookup in O(1). No dependency.
- **A byte limit:** LLM responses range from 100 B to hundreds of KB, so counting items says nothing about memory.
- **`Lock` on `Get`:** a hit moves the item in the list, so an `RWMutex` wouldn't help.
- **Lazy expiration:** an expired entry is removed when someone asks for it or when the LRU evicts it. There's no cleanup goroutine.
- Ponytail: the global mutex is marked in the code, with the upgrade path (shards by hash) in case contention shows up in a profile.
- The cache is **opt-in** and the key is computed **after LGPD masking** (feature 08): the cache never sees personal data.

## How to test
`go test ./internal/cache/ -v`: hit, TTL, LRU eviction, entry larger than the cache, replacement, concurrency (`-race`).

## Go vs TypeScript
- A JS `Map` already preserves insertion order, and the classic JS LRU uses `delete` + `set`. In Go the map has no order, which is why the list exists.
- `el.Value.(*item)` is a *type assertion*: the list stores `any`, like an `unknown` that you convert.

## Exercise
Replace the global mutex with 16 shards (`fnv` of the key → index) and measure with `go test -bench` before and after.

## Interview question
"Why does your LRU limit bytes and not items, and what happens to a response bigger than the whole cache?"
