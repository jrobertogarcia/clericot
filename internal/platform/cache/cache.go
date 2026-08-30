package cache

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	"github.com/google/uuid"
	"github.com/maypok86/otter/v2"
	"github.com/redis/go-redis/v9"
	"golang.org/x/sync/singleflight"

	"clericot/internal/platform/tenant"
)

const InvalidationChannel = "cache:invalidations"

type CacheEngine struct {
	l1       *otter.Cache[string, []byte]
	l2       *redis.Client
	group    singleflight.Group
	nodeID   string
	cancelFn context.CancelFunc
}

func NewCacheEngine(rdb *redis.Client) (*CacheEngine, error) {
	l1Cache, err := otter.New(&otter.Options[string, []byte]{
		MaximumSize:      100_000,
		ExpiryCalculator: otter.ExpiryWriting[string, []byte](2 * time.Minute),
	})
	if err != nil {
		return nil, fmt.Errorf("failed to init otter l1 cache: %w", err)
	}

	ctx, cancel := context.WithCancel(context.Background())
	engine := &CacheEngine{
		l1:       l1Cache,
		l2:       rdb,
		nodeID:   uuid.NewString(),
		cancelFn: cancel,
	}

	if rdb != nil {
		go engine.listenInvalidations(ctx)
	}

	return engine, nil
}

type CachedEntry[T any] struct {
	Value   T    `json:"value,omitempty"`
	IsEmpty bool `json:"is_empty,omitempty"`
}

// TenantKey formats a cache key automatically scoped to the active tenant in ctx.
func TenantKey(ctx context.Context, domain string, id string) string {
	tenantID := tenant.FromContext(ctx)
	if tenantID == "" {
		tenantID = "global"
	}
	return fmt.Sprintf("tenants:%s:%s:%s", tenantID, domain, id)
}

// Key formats a global non-tenant cache key.
func Key(domain string, id string) string {
	return fmt.Sprintf("global:%s:%s", domain, id)
}

// FetchOrCompute retrieves a typed value from L1 -> L2 -> Compute with Singleflight protection.
func FetchOrCompute[T any](c *CacheEngine, ctx context.Context, key string, l2TTL time.Duration, compute func() (T, error)) (T, error) {
	var zero T

	// 1. Check L1 Memory Cache (Sub-microsecond)
	if c.l1 != nil {
		if val, found := c.l1.GetIfPresent(key); found {
			var entry CachedEntry[T]
			if err := json.Unmarshal(val, &entry); err == nil {
				if entry.IsEmpty {
					return zero, nil
				}
				return entry.Value, nil
			}
		}
	}

	// 2. Check L2 Redis Cache
	if c.l2 != nil {
		if val, err := c.l2.Get(ctx, key).Bytes(); err == nil {
			var entry CachedEntry[T]
			if err := json.Unmarshal(val, &entry); err == nil {
				if c.l1 != nil {
					c.l1.Set(key, val)
				}
				if entry.IsEmpty {
					return zero, nil
				}
				return entry.Value, nil
			}
		}
	}

	// 3. Singleflight Compute Guard (Prevent Cache Stampede / Thundering Herd)
	res, err, _ := c.group.Do(key, func() (any, error) {
		data, err := compute()
		if err != nil {
			return nil, err
		}

		entry := CachedEntry[T]{Value: data}
		encoded, err := json.Marshal(entry)
		if err != nil {
			return nil, fmt.Errorf("failed to serialize cache payload: %w", err)
		}

		// Store in L2 Redis if available
		if c.l2 != nil && l2TTL > 0 {
			_ = c.l2.Set(ctx, key, encoded, l2TTL).Err()
		}

		// Store in L1 Memory with 2-minute fallback TTL
		if c.l1 != nil {
			c.l1.Set(key, encoded)
		}

		return data, nil
	})

	if err != nil {
		return zero, err
	}
	return res.(T), nil
}

// Invalidate clears local L1, removes L2, and publishes invalidation signal to other pods.
func (c *CacheEngine) Invalidate(ctx context.Context, keys ...string) error {
	if c.l1 != nil {
		for _, key := range keys {
			c.l1.Invalidate(key)
		}
	}

	if c.l2 != nil {
		if err := c.l2.Del(ctx, keys...).Err(); err != nil {
			return err
		}

		// Broadcast invalidation message
		msg := struct {
			NodeID string   `json:"node_id"`
			Keys   []string `json:"keys"`
		}{
			NodeID: c.nodeID,
			Keys:   keys,
		}
		payload, _ := json.Marshal(msg)
		return c.l2.Publish(ctx, InvalidationChannel, payload).Err()
	}

	return nil
}

// Close releases cache resources and terminates background subscriber.
func (c *CacheEngine) Close() error {
	if c.cancelFn != nil {
		c.cancelFn()
	}
	if c.l1 != nil {
		c.l1.StopAllGoroutines()
	}
	return nil
}

func (c *CacheEngine) listenInvalidations(ctx context.Context) {
	pubsub := c.l2.Subscribe(ctx, InvalidationChannel)
	defer pubsub.Close()

	ch := pubsub.Channel()
	for {
		select {
		case <-ctx.Done():
			return
		case msg, ok := <-ch:
			if !ok {
				return
			}
			var payload struct {
				NodeID string   `json:"node_id"`
				Keys   []string `json:"keys"`
			}
			if err := json.Unmarshal([]byte(msg.Payload), &payload); err == nil {
				if payload.NodeID != c.nodeID && c.l1 != nil {
					for _, key := range payload.Keys {
						c.l1.Invalidate(key)
					}
				}
			}
		}
	}
}
