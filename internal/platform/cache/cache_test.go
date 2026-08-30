package cache_test

import (
	"context"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"clericot/internal/platform/cache"
	"clericot/internal/platform/tenant"
)

func TestCacheEngine_L1AndSingleflight(t *testing.T) {
	ctx := context.Background()
	engine, err := cache.NewCacheEngine(nil) // Local L1-only mode without redis
	require.NoError(t, err)
	defer engine.Close()

	var computeCalls atomic.Int64
	expensiveCompute := func() (string, error) {
		computeCalls.Add(1)
		time.Sleep(50 * time.Millisecond) // Simulate slow query
		return "computed-payload", nil
	}

	key := cache.Key("users", "usr_100")

	// 1. Test Singleflight Thundering Herd Suppression (10 concurrent callers)
	var wg sync.WaitGroup
	results := make([]string, 10)

	for i := 0; i < 10; i++ {
		wg.Add(1)
		go func(idx int) {
			defer wg.Done()
			val, err := cache.FetchOrCompute(engine, ctx, key, 5*time.Minute, expensiveCompute)
			assert.NoError(t, err)
			results[idx] = val
		}(i)
	}

	wg.Wait()

	// All 10 concurrent requests must receive the same result
	for _, res := range results {
		assert.Equal(t, "computed-payload", res)
	}

	// Compute must have only been executed ONCE due to singleflight!
	assert.Equal(t, int64(1), computeCalls.Load())

	// 2. Subsequent call must hit L1 cache immediately without recomputing
	cachedVal, err := cache.FetchOrCompute(engine, ctx, key, 5*time.Minute, expensiveCompute)
	require.NoError(t, err)
	assert.Equal(t, "computed-payload", cachedVal)
	assert.Equal(t, int64(1), computeCalls.Load())

	// 3. Test Invalidate
	err = engine.Invalidate(ctx, key)
	require.NoError(t, err)

	// After invalidation, compute should be triggered again
	freshVal, err := cache.FetchOrCompute(engine, ctx, key, 5*time.Minute, expensiveCompute)
	require.NoError(t, err)
	assert.Equal(t, "computed-payload", freshVal)
	assert.Equal(t, int64(2), computeCalls.Load())
}

func TestCacheKey_TenantScoping(t *testing.T) {
	ctx := context.Background()
	globalKey := cache.TenantKey(ctx, "orders", "ord_1")
	assert.Equal(t, "tenants:global:orders:ord_1", globalKey)

	tenantCtx := tenant.WithTenant(ctx, "tenant-42")
	tenantKey := cache.TenantKey(tenantCtx, "orders", "ord_1")
	assert.Equal(t, "tenants:tenant-42:orders:ord_1", tenantKey)
}
