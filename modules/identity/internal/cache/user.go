package cache

import (
	"context"
	"encoding/json/v2"
	"fmt"
	"time"

	"github.com/redis/go-redis/v9"

	"xxxdonacion/modules/identity/api/domain"
	"xxxdonacion/platform/logger"
)

const (
	userTTL           = 10 * time.Minute
	invalidateRetries = 3
)

type UserCache struct {
	rdb *redis.Client
}

func NewUserCache(rdb *redis.Client) *UserCache { return &UserCache{rdb: rdb} }

func userKey(uid string) string { return "identity:user:" + uid }

func (c *UserCache) Get(ctx context.Context, uid string) (domain.User, bool) {
	raw, err := c.rdb.Get(ctx, userKey(uid)).Bytes()
	if err != nil {
		return domain.User{}, false
	}
	var u domain.User
	if err := json.Unmarshal(raw, &u); err != nil {
		return domain.User{}, false
	}
	return u, true
}

func (c *UserCache) Set(ctx context.Context, uid string, u domain.User) {
	raw, err := json.Marshal(u)
	if err != nil {
		return
	}
	if err := c.rdb.Set(ctx, userKey(uid), raw, userTTL).Err(); err != nil {
		// Cache tidak boleh menggagalkan request; DB tetap sumber kebenaran.
		logger.FromContext(ctx).Warn("identity cache set failed", "error", err)
	}
}

// Invalidate dipanggil setiap mutasi user/role, supaya perubahan langsung terasa.
// Error dikembalikan agar mutasi security bisa fail-closed saat Redis bermasalah.
func (c *UserCache) Invalidate(ctx context.Context, uid string) error {
	var err error
	for range invalidateRetries {
		if err = c.rdb.Del(ctx, userKey(uid)).Err(); err == nil {
			return nil
		}
	}
	logger.FromContext(ctx).Warn("identity cache del failed", "error", err)
	return fmt.Errorf("invalidate identity cache: %w", err)
}
