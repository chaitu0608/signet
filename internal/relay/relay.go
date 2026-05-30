package relay

import (
	"context"
	"encoding/json"
	"log/slog"
	"os"

	"github.com/redis/go-redis/v9"
)

const channel = "forgepulse.events"

// Relay publishes event JSON for multi-instance fanout.
type Relay interface {
	Publish(ctx context.Context, payload []byte) error
	Subscribe(ctx context.Context, ch chan<- []byte) error
	Close() error
}

// NoopRelay is used when Redis is not configured.
type NoopRelay struct{}

func (NoopRelay) Publish(ctx context.Context, payload []byte) error { return nil }
func (NoopRelay) Subscribe(ctx context.Context, ch chan<- []byte) error {
	<-ctx.Done()
	return ctx.Err()
}
func (NoopRelay) Close() error { return nil }

// RedisRelay uses Redis pub/sub.
type RedisRelay struct {
	client *redis.Client
}

// OpenFromEnv creates Redis relay or noop.
func OpenFromEnv() Relay {
	url := os.Getenv("REDIS_URL")
	if url == "" {
		slog.Info("redis not configured, single-instance mode")
		return NoopRelay{}
	}
	opt, err := redis.ParseURL(url)
	if err != nil {
		slog.Warn("redis parse failed", "err", err)
		return NoopRelay{}
	}
	return &RedisRelay{client: redis.NewClient(opt)}
}

func (r *RedisRelay) Publish(ctx context.Context, payload []byte) error {
	return r.client.Publish(ctx, channel, payload).Err()
}

func (r *RedisRelay) Subscribe(ctx context.Context, ch chan<- []byte) error {
	sub := r.client.Subscribe(ctx, channel)
	defer sub.Close()
	for {
		msg, err := sub.ReceiveMessage(ctx)
		if err != nil {
			return err
		}
		payload := []byte(msg.Payload)
		select {
		case ch <- payload:
		case <-ctx.Done():
			return ctx.Err()
		}
	}
}

func (r *RedisRelay) Close() error {
	return r.client.Close()
}

// MarshalEvent helper.
func MarshalEvent(v any) ([]byte, error) {
	return json.Marshal(v)
}
