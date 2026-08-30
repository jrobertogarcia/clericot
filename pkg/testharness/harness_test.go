package testharness_test

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"clericot/pkg/testharness"
	"clericot/tests/testsuite"
)

func TestMain(m *testing.M) {
	testsuite.Main(m)
}

func TestHarness_SeedTenantAndUser(t *testing.T) {
	harness := testharness.New(t)
	defer harness.Close()

	ctx := context.Background()
	tenantID, err := harness.SeedTenant(ctx, "Harness Org")
	require.NoError(t, err)
	require.NotEmpty(t, tenantID)

	user, err := harness.SeedUser(ctx, tenantID, "test@harness.com", "Harness User", "admin")
	require.NoError(t, err)
	assert.Equal(t, "Harness User", user.Name)
	assert.Equal(t, tenantID, user.TenantID)
}
