// Redis is used for: OTP storage/rate limiting, refresh-token/session
// denylists, distributed locks (e.g. inventory decrement, delivery
// assignment), idempotency keys, and cache-aside for hot reads (restaurant
// listings, menu). Keeping one client here avoids each module hand-rolling
// its own connection.
package db

import (
	"context"
	"fmt"

	"github.com/redis/go-redis/v9"

	"github.com/foodapp/backend/internal/config"
)

type Redis struct {
	Client *redis.Client
}

func NewRedis(ctx context.Context, cfg config.RedisConfig) (*Redis, error) {
	client := redis.NewClient(&redis.Options{
		Addr:     cfg.Addr,
		Password: cfg.Password,
		DB:       cfg.DB,
	})

	if err := client.Ping(ctx).Err(); err != nil {
		return nil, fmt.Errorf("ping redis: %w", err)
	}

	return &Redis{Client: client}, nil
}

func (r *Redis) Close() error {
	return r.Client.Close()
}

func (r *Redis) HealthCheck(ctx context.Context) error {
	return r.Client.Ping(ctx).Err()
}
