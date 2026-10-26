package cache

import (
	"context"
	"testing"
	"time"
	"uuid"

	"github.com/alicebob/miniredis/v2"
	"github.com/redis/go-redis/v9"

	"xxxdonacion/modules/identity/api/domain"
)

func newTestCache(t *testing.T) (*UserCache, *miniredis.Miniredis) {
	t.Helper()
	mr := miniredis.RunT(t)
	rdb := redis.NewClient(&redis.Options{Addr: mr.Addr(), MaxRetries: -1})
	t.Cleanup(func() { _ = rdb.Close() })
	return NewUserCache(rdb), mr
}

func TestUserCacheSetGetInvalidate(t *testing.T) {
	cache, mr := newTestCache(t)
	ctx := context.Background()
	user := domain.User{
		ID:        uuid.NewV7(),
		AuthID:    "uid-1",
		Email:     "user@example.com",
		Status:    domain.StatusActive,
		Roles:     []domain.Role{domain.RoleUser},
		CreatedAt: time.Now(),
		UpdatedAt: time.Now(),
	}

	cache.Set(ctx, user.AuthID, user)
	got, ok := cache.Get(ctx, user.AuthID)
	if !ok {
		t.Fatal("cache miss after Set")
	}
	if got.Email != user.Email || got.Status != user.Status || len(got.Roles) != 1 {
		t.Fatalf("cached user = %+v, want %+v", got, user)
	}
	if ttl := mr.TTL(userKey(user.AuthID)); ttl != userTTL {
		t.Fatalf("ttl = %s, want %s", ttl, userTTL)
	}

	if err := cache.Invalidate(ctx, user.AuthID); err != nil {
		t.Fatalf("invalidate: %v", err)
	}
	if _, ok := cache.Get(ctx, user.AuthID); ok {
		t.Fatal("cache hit after Invalidate")
	}
}

func TestUserCacheRejectsInvalidJSON(t *testing.T) {
	cache, mr := newTestCache(t)
	if err := mr.Set(userKey("uid-1"), "not-json"); err != nil {
		t.Fatal(err)
	}
	if _, ok := cache.Get(context.Background(), "uid-1"); ok {
		t.Fatal("invalid cache value should be a miss")
	}
}

func TestUserCacheInvalidateReturnsErrorWhenRedisIsDown(t *testing.T) {
	cache, mr := newTestCache(t)
	mr.Close()

	if err := cache.Invalidate(context.Background(), "uid-1"); err == nil {
		t.Fatal("expected invalidation error")
	}
}
