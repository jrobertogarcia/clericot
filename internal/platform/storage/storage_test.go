package storage_test

import (
	"context"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"clericot/internal/platform/storage"
	"clericot/internal/platform/tenant"
)

func TestStorageEngine_Operations(t *testing.T) {
	ctx := context.Background()
	engine, err := storage.NewStorageEngine(ctx, "mem://")
	require.NoError(t, err)
	defer engine.Close()

	tenantCtx := tenant.WithTenant(ctx, "tenant-org-99")

	// 1. Generate Presigned Upload URL
	signedURL, key, err := engine.PresignedUpload(tenantCtx, "invoice.pdf", "application/pdf", 15*time.Minute)
	require.NoError(t, err)
	require.NotEmpty(t, signedURL)
	assert.True(t, strings.HasPrefix(key, "tenants/tenant-org-99/uploads/"))
	assert.True(t, strings.HasSuffix(key, "-invoice.pdf"))

	// 2. Write Data
	data := []byte("%PDF-1.4 Mock PDF Content")
	err = engine.Write(tenantCtx, key, data, "application/pdf")
	require.NoError(t, err)

	// 3. Confirm Upload Attributes
	attrs, err := engine.ConfirmUpload(tenantCtx, key)
	require.NoError(t, err)
	assert.Equal(t, int64(len(data)), attrs.Size)

	// 4. Read Data
	readData, err := engine.Read(tenantCtx, key)
	require.NoError(t, err)
	assert.Equal(t, data, readData)

	// 5. Delete Prefix
	prefix := "tenants/tenant-org-99/"
	err = engine.DeletePrefix(tenantCtx, prefix)
	require.NoError(t, err)

	// Read should now fail
	_, err = engine.Read(tenantCtx, key)
	assert.Error(t, err)
}
