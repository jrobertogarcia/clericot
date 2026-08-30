package tenant_test

import (
	"context"
	"encoding/json"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/hibiken/asynq"
	"github.com/jackc/pgx/v5/pgtype"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gocloud.dev/blob"
	_ "gocloud.dev/blob/memblob"

	"clericot/internal/platform/storage"
	"clericot/internal/platform/tenant"
	"clericot/internal/sqlcgen"
	"clericot/tests/testsuite"
)

func TestPurgeTenantWorker_ProcessTask(t *testing.T) {
	ctx := context.Background()
	adminQueries := sqlcgen.New(testsuite.SharedAdminPool)
	appTxManager := testsuite.SharedTxManager

	// 1. Setup in-memory blob storage bucket
	bucket, err := blob.OpenBucket(ctx, "mem://")
	require.NoError(t, err)
	defer bucket.Close()

	storageEngine := storage.NewStorageEngineWithBucket(bucket)
	purgeWorker := tenant.NewPurgeTenantWorker(appTxManager, storageEngine)

	// 2. Seed Tenant A and Tenant B
	tenantA := "tenant-a-" + uuid.NewString()[:6]
	tenantB := "tenant-b-" + uuid.NewString()[:6]
	ts := pgtype.Timestamptz{Time: time.Now().UTC(), Valid: true}

	_, err = adminQueries.CreateTenant(ctx, sqlcgen.CreateTenantParams{
		ID:        tenantA,
		Name:      "Organization Alpha",
		Status:    "active",
		CreatedAt: ts,
		UpdatedAt: ts,
	})
	require.NoError(t, err)

	_, err = adminQueries.CreateTenant(ctx, sqlcgen.CreateTenantParams{
		ID:        tenantB,
		Name:      "Organization Beta",
		Status:    "active",
		CreatedAt: ts,
		UpdatedAt: ts,
	})
	require.NoError(t, err)

	// 3. Seed users for Tenant A and Tenant B
	userA := "usr-a-" + uuid.NewString()[:6]
	err = appTxManager.RunInTx(tenant.WithTenant(ctx, tenantA), func(txCtx context.Context) error {
		db := appTxManager.GetDB(txCtx)
		_, err := sqlcgen.New(db).CreateUser(txCtx, sqlcgen.CreateUserParams{
			ID:           userA,
			TenantID:     tenantA,
			Email:        "alice@orgalpha.com",
			Name:         "Alice Alpha",
			PasswordHash: "secret_argon2_hash",
			Role:         "admin",
			CreatedAt:    ts,
			UpdatedAt:    ts,
		})
		return err
	})
	require.NoError(t, err)

	userB := "usr-b-" + uuid.NewString()[:6]
	err = appTxManager.RunInTx(tenant.WithTenant(ctx, tenantB), func(txCtx context.Context) error {
		db := appTxManager.GetDB(txCtx)
		_, err := sqlcgen.New(db).CreateUser(txCtx, sqlcgen.CreateUserParams{
			ID:           userB,
			TenantID:     tenantB,
			Email:        "bob@orgbeta.com",
			Name:         "Bob Beta",
			PasswordHash: "secret_argon2_hash_b",
			Role:         "member",
			CreatedAt:    ts,
			UpdatedAt:    ts,
		})
		return err
	})
	require.NoError(t, err)

	// 4. Upload blobs for Tenant A and Tenant B into Storage
	blobA1 := "tenants/" + tenantA + "/uploads/doc1.pdf"
	blobA2 := "tenants/" + tenantA + "/uploads/avatar.png"
	blobB := "tenants/" + tenantB + "/uploads/invoice.pdf"

	require.NoError(t, storageEngine.Write(ctx, blobA1, []byte("alpha doc 1"), "application/pdf"))
	require.NoError(t, storageEngine.Write(ctx, blobA2, []byte("alpha avatar"), "image/png"))
	require.NoError(t, storageEngine.Write(ctx, blobB, []byte("beta invoice"), "application/pdf"))

	// 5. Construct and execute Purge Task for Tenant A
	payloadBytes, err := json.Marshal(tenant.PurgeTenantPayload{TenantID: tenantA})
	require.NoError(t, err)

	task := asynq.NewTask(tenant.TypeTenantPurge, payloadBytes)
	err = purgeWorker.ProcessTask(ctx, task)
	require.NoError(t, err)

	// 6. Verify Phase 1 (Database Assertions via Admin Pool)
	dbTenantA, err := adminQueries.GetTenantByID(ctx, tenantA)
	require.NoError(t, err)
	assert.Equal(t, "deleted", dbTenantA.Status)
	assert.True(t, dbTenantA.DeletedAt.Valid, "deleted_at must be set")

	// Verify User A was anonymized in DB
	var (
		scrubbedEmail string
		scrubbedName  string
		scrubbedHash  string
	)
	err = testsuite.SharedAdminPool.QueryRow(ctx, "SELECT email, name, password_hash FROM users WHERE id = $1", userA).Scan(
		&scrubbedEmail,
		&scrubbedName,
		&scrubbedHash,
	)
	require.NoError(t, err)
	assert.Equal(t, "anonymized_"+userA+"@deleted.local", scrubbedEmail)
	assert.Equal(t, "Anonymized User", scrubbedName)
	assert.Equal(t, "REDACTED", scrubbedHash)

	// Verify Tenant A RLS query now returns 0 rows (since tenant status is 'deleted')
	err = appTxManager.RunInTx(tenant.WithTenant(ctx, tenantA), func(txCtx context.Context) error {
		db := appTxManager.GetDB(txCtx)
		var count int
		err := db.QueryRow(txCtx, "SELECT COUNT(*) FROM users").Scan(&count)
		require.NoError(t, err)
		assert.Equal(t, 0, count)
		return nil
	})
	require.NoError(t, err)

	// Verify Tenant B user is untouched
	dbUserB, err := adminQueries.GetUserByID(ctx, sqlcgen.GetUserByIDParams{
		ID:       userB,
		TenantID: tenantB,
	})
	require.NoError(t, err)
	assert.Equal(t, "bob@orgbeta.com", dbUserB.Email)
	assert.Equal(t, "Bob Beta", dbUserB.Name)

	// 7. Verify Phase 2 (Storage Deletions)
	// Tenant A blobs must be deleted
	_, err = storageEngine.Read(ctx, blobA1)
	assert.Error(t, err, "tenant A blob 1 should be deleted")

	_, err = storageEngine.Read(ctx, blobA2)
	assert.Error(t, err, "tenant A blob 2 should be deleted")

	// Tenant B blob must be intact
	betaData, err := storageEngine.Read(ctx, blobB)
	require.NoError(t, err)
	assert.Equal(t, []byte("beta invoice"), betaData)
}

func TestPurgeTenantWorker_InvalidPayload(t *testing.T) {
	ctx := context.Background()
	purgeWorker := tenant.NewPurgeTenantWorker(testsuite.SharedTxManager, nil)

	// 1. Corrupted JSON payload
	badTask := asynq.NewTask(tenant.TypeTenantPurge, []byte("invalid-json"))
	err := purgeWorker.ProcessTask(ctx, badTask)
	assert.Error(t, err)

	// 2. Empty Tenant ID
	emptyPayload, _ := json.Marshal(tenant.PurgeTenantPayload{TenantID: ""})
	emptyTask := asynq.NewTask(tenant.TypeTenantPurge, emptyPayload)
	err = purgeWorker.ProcessTask(ctx, emptyTask)
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "tenant_id is missing")
}

func TestEnqueuePurgeTenant(t *testing.T) {
	ctx := context.Background()

	// Nil client error
	err := tenant.EnqueuePurgeTenant(ctx, nil, "tenant-123")
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "asynq client is nil")

	// Empty tenant ID error
	redisClient := asynq.NewClient(asynq.RedisClientOpt{Addr: "localhost:6379"})
	defer redisClient.Close()

	err = tenant.EnqueuePurgeTenant(ctx, redisClient, "")
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "tenant_id is required")
}
