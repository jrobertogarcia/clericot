package notify_test

import (
	"context"
	"testing"

	"github.com/stretchr/testify/require"

	"clericot/internal/platform/notify"
)

func TestNotificationDispatcher_EnqueueAndDirect(t *testing.T) {
	ctx := context.Background()
	dispatcher := notify.NewNotificationDispatcher(nil, "no-reply@clericot.dev")

	msg := notify.EmailMessage{
		To:      "user@example.com",
		Subject: "Welcome to Clericot",
		Body:    "<h1>Hello</h1>",
		IsHTML:  true,
	}

	err := dispatcher.EnqueueEmail(ctx, msg)
	require.NoError(t, err)

	err = dispatcher.SendDirect(ctx, msg)
	require.NoError(t, err)
}
