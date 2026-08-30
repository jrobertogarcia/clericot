package events

import (
	"fmt"
	"time"

	"github.com/ThreeDotsLabs/watermill"
	"github.com/ThreeDotsLabs/watermill-redisstream/pkg/redisstream"
	"github.com/ThreeDotsLabs/watermill/message"
	"github.com/redis/go-redis/v9"
)

type BrokerConfig struct {
	Driver string // "redis", "nats", "kafka"
}

// NopPublisher is a no-op publisher used when message broker is disabled or in local test mode.
type NopPublisher struct{}

func (n *NopPublisher) Publish(topic string, messages ...*message.Message) error {
	return nil
}

func (n *NopPublisher) Close() error {
	return nil
}

// NewPubSub creates a Watermill Publisher and Subscriber with Redis Streams defaults.
func NewPubSub(cfg BrokerConfig, rdb *redis.Client, logger watermill.LoggerAdapter) (message.Publisher, message.Subscriber, error) {
	if logger == nil {
		logger = watermill.NopLogger{}
	}

	switch cfg.Driver {
	case "redis", "":
		if rdb == nil {
			return nil, nil, fmt.Errorf("redis client is required for redisstream driver")
		}

		pub, err := redisstream.NewPublisher(redisstream.PublisherConfig{
			Client:        rdb,
			DefaultMaxlen: 100_000, // Stream trimming: bounded Redis memory
			Marshaller:    redisstream.DefaultMarshallerUnmarshaller{},
		}, logger)
		if err != nil {
			return nil, nil, fmt.Errorf("failed to create redisstream publisher: %w", err)
		}

		sub, err := redisstream.NewSubscriber(redisstream.SubscriberConfig{
			Client:        rdb,
			ConsumerGroup: "clericot-workers",
			ClaimInterval: 30 * time.Second, // Scans PEL for abandoned messages from crashed workers
			MaxIdleTime:   2 * time.Minute,  // Reclaims messages idle > 2m
			ClaimBatchSize: 50,
			Unmarshaller:  redisstream.DefaultMarshallerUnmarshaller{},
		}, logger)
		if err != nil {
			return nil, nil, fmt.Errorf("failed to create redisstream subscriber: %w", err)
		}

		return pub, sub, nil

	default:
		return nil, nil, fmt.Errorf("unsupported event broker driver: %s", cfg.Driver)
	}
}
