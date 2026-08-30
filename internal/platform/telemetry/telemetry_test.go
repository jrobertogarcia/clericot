package telemetry_test

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"clericot/internal/platform/telemetry"
)

func TestInitTracerAndLogger(t *testing.T) {
	ctx := context.Background()

	tp, err := telemetry.InitTracer(ctx, "clericot-test", 0.05)
	require.NoError(t, err)
	require.NotNil(t, tp)

	err = tp.Shutdown(ctx)
	require.NoError(t, err)

	logger := telemetry.InitLogger("debug")
	assert.NotNil(t, logger)
}
