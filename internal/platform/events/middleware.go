package events

import (
	"fmt"
	"time"

	"github.com/ThreeDotsLabs/watermill/message"
	"github.com/ThreeDotsLabs/watermill/message/router/middleware"
	"github.com/redis/go-redis/v9"
)

// IdempotencyMiddleware ensures each message is executed at most once using Redis SET NX.
func IdempotencyMiddleware(rdb *redis.Client, ttl time.Duration) message.HandlerMiddleware {
	return func(h message.HandlerFunc) message.HandlerFunc {
		return func(msg *message.Message) ([]*message.Message, error) {
			if rdb == nil {
				return h(msg)
			}

			key := fmt.Sprintf("idempotency:events:%s", msg.UUID)

			// Atomic test-and-set execution lock
			ok, err := rdb.SetNX(msg.Context(), key, "1", ttl).Result()
			if err != nil {
				return nil, fmt.Errorf("failed to verify idempotency key: %w", err)
			}
			if !ok {
				// Duplicate detected: ACK message and discard duplicate execution
				return nil, nil
			}

			return h(msg)
		}
	}
}

// ConfigureSubscriberRouter attaches idempotency, retry, and poison queue DLQ routing.
func ConfigureSubscriberRouter(router *message.Router, dlqPublisher message.Publisher, rdb *redis.Client) error {
	if rdb != nil {
		router.AddMiddleware(IdempotencyMiddleware(rdb, 24*time.Hour))
	}

	router.AddMiddleware(middleware.Retry{
		MaxRetries:      3,
		InitialInterval: 100 * time.Millisecond,
		Multiplier:      2.0,
		MaxInterval:     2 * time.Second,
	}.Middleware)

	if dlqPublisher != nil {
		poisonQueue, err := middleware.PoisonQueue(dlqPublisher, "events:dlq:poisoned")
		if err != nil {
			return fmt.Errorf("failed to configure poison queue: %w", err)
		}
		router.AddMiddleware(poisonQueue)
	}

	return nil
}
