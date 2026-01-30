// SPDX-License-Identifier: AGPL-3.0-or-later

package handlers

import (
	context "context"
	"encoding/json"
	"sync"
	"time"

	"github.com/flowd-org/flowd/internal/coredb"
)

type idempotencyStore interface {
	Lookup(ctx context.Context, key, endpoint string, now time.Time) (RunPayload, int, string, bool, error)
	Reserve(ctx context.Context, key, endpoint, bodyHash string, now, expiresAt time.Time) (bool, error)
	Store(ctx context.Context, key, endpoint, bodyHash string, payload RunPayload, status int, expiresAt time.Time) error
	Release(ctx context.Context, key, endpoint string) error
}

const idempotencyStatusInFlight = 0

// memoryIdempotencyCache is the in-process fallback used when Core DB is unavailable.
type memoryIdempotencyCache struct {
	mu    sync.RWMutex
	ttl   time.Duration
	items map[string]map[string]cacheEntry
}

type cacheEntry struct {
	payload  RunPayload
	status   int
	bodyHash string
	expires  time.Time
}

func newMemoryIdempotencyCache(ttl time.Duration) *memoryIdempotencyCache {
	if ttl <= 0 {
		ttl = defaultIdempotencyTTL
	}
	return &memoryIdempotencyCache{
		ttl:   ttl,
		items: make(map[string]map[string]cacheEntry),
	}
}

func (c *memoryIdempotencyCache) Lookup(_ context.Context, key, endpoint string, now time.Time) (RunPayload, int, string, bool, error) {
	c.mu.Lock()
	defer c.mu.Unlock()
	bucket, ok := c.items[key]
	if !ok {
		return RunPayload{}, 0, "", false, nil
	}
	entry, ok := bucket[endpoint]
	if !ok {
		return RunPayload{}, 0, "", false, nil
	}
	if now.After(entry.expires) {
		delete(bucket, endpoint)
		if len(bucket) == 0 {
			delete(c.items, key)
		}
		return RunPayload{}, 0, "", false, nil
	}
	return entry.payload, entry.status, entry.bodyHash, true, nil
}

func (c *memoryIdempotencyCache) Reserve(_ context.Context, key, endpoint, bodyHash string, now, expiresAt time.Time) (bool, error) {
	c.mu.Lock()
	defer c.mu.Unlock()
	bucket := c.items[key]
	if bucket != nil {
		entry, ok := bucket[endpoint]
		if ok {
			if now.After(entry.expires) {
				delete(bucket, endpoint)
				if len(bucket) == 0 {
					delete(c.items, key)
				}
			} else {
				return false, nil
			}
		}
	}
	if bucket == nil {
		bucket = make(map[string]cacheEntry)
		c.items[key] = bucket
	}
	bucket[endpoint] = cacheEntry{
		payload:  RunPayload{},
		status:   idempotencyStatusInFlight,
		bodyHash: bodyHash,
		expires:  expiresAt,
	}
	return true, nil
}

func (c *memoryIdempotencyCache) Store(_ context.Context, key, endpoint, bodyHash string, payload RunPayload, status int, expiresAt time.Time) error {
	c.mu.Lock()
	defer c.mu.Unlock()
	bucket := c.items[key]
	if bucket == nil {
		bucket = make(map[string]cacheEntry)
		c.items[key] = bucket
	}
	bucket[endpoint] = cacheEntry{
		payload:  payload,
		status:   status,
		bodyHash: bodyHash,
		expires:  expiresAt,
	}
	return nil
}

func (c *memoryIdempotencyCache) Release(_ context.Context, key, endpoint string) error {
	c.mu.Lock()
	defer c.mu.Unlock()
	bucket := c.items[key]
	if bucket == nil {
		return nil
	}
	entry, ok := bucket[endpoint]
	if ok && entry.status == idempotencyStatusInFlight {
		delete(bucket, endpoint)
		if len(bucket) == 0 {
			delete(c.items, key)
		}
	}
	return nil
}

// dbIdempotencyStore persists entries using Core DB.
type dbIdempotencyStore struct {
	store *coredb.IdempotencyStore
}

func newDBIdempotencyStore(db *coredb.DB) *dbIdempotencyStore {
	if db == nil {
		return nil
	}
	return &dbIdempotencyStore{store: coredb.NewIdempotencyStore(db)}
}

func (d *dbIdempotencyStore) Lookup(ctx context.Context, key, endpoint string, now time.Time) (RunPayload, int, string, bool, error) {
	if d == nil || d.store == nil {
		return RunPayload{}, 0, "", false, nil
	}
	body, status, hash, found, err := d.store.Lookup(ctx, key, endpoint, now)
	if err != nil || !found {
		return RunPayload{}, 0, "", found, err
	}
	var payload RunPayload
	if err := json.Unmarshal(body, &payload); err != nil {
		return RunPayload{}, 0, "", false, err
	}
	return payload, status, hash, true, nil
}

func (d *dbIdempotencyStore) Reserve(ctx context.Context, key, endpoint, bodyHash string, now, expiresAt time.Time) (bool, error) {
	if d == nil || d.store == nil {
		return false, nil
	}
	return d.store.Reserve(ctx, key, endpoint, bodyHash, now, expiresAt)
}

func (d *dbIdempotencyStore) Store(ctx context.Context, key, endpoint, bodyHash string, payload RunPayload, status int, expiresAt time.Time) error {
	if d == nil || d.store == nil {
		return nil
	}
	data, err := json.Marshal(payload)
	if err != nil {
		return err
	}
	return d.store.Store(ctx, key, endpoint, bodyHash, status, data, expiresAt)
}

func (d *dbIdempotencyStore) Release(ctx context.Context, key, endpoint string) error {
	if d == nil || d.store == nil {
		return nil
	}
	return d.store.Release(ctx, key, endpoint)
}
