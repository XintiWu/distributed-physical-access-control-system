package cache

import (
	"context"
	"errors"
	"fmt"
	"os"
	"time"

	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promauto"
	"github.com/redis/go-redis/v9"
	"github.com/tsmc/access-api/internal/model"
)

// BatchReadResult holds the results of a pipelined read for a single swipe request.
type BatchReadResult struct {
	MappedUserID  string            // card → userID mapping (empty if cardUID was empty or not found)
	IsDenied      bool              // true if user has an active permission-denied flag
	PassbackState model.PassbackState // last known passback state for the user
}

const (
	passbackTTL = 24 * time.Hour
	cardTTL     = 24 * time.Hour
	doorTTL     = 30 * time.Second
)

var (
	cacheOps = promauto.NewCounterVec(prometheus.CounterOpts{
		Name: "access_api_cache_ops_total",
		Help: "Redis cache operations by result",
	}, []string{"op", "result"}) // result: hit, miss, error
)

type RedisCache struct {
	client redis.UniversalClient
}

func NewRedisCache(addr string) *RedisCache {
	var client redis.UniversalClient
	if os.Getenv("REDIS_CLUSTER") == "true" {
		client = redis.NewClusterClient(&redis.ClusterOptions{
			Addrs:         []string{addr},
			PoolSize:      128,
			MinIdleConns:  32,
			PoolTimeout:   2 * time.Second,
		})
	} else {
		client = redis.NewClient(&redis.Options{
			Addr:         addr,
			PoolSize:     128,
			MinIdleConns: 32,
			PoolTimeout:  2 * time.Second,
		})
	}
	return &RedisCache{
		client: client,
	}
}

func (c *RedisCache) Ping(ctx context.Context) error {
	return c.client.Ping(ctx).Err()
}

// BatchRead fetches card mapping, permission-denied flag, and passback state in a
// single Redis pipeline, reducing serial RTTs from 3 to 1 on the hot path.
func (c *RedisCache) BatchRead(ctx context.Context, cardUID, userID string) (BatchReadResult, error) {
	pipe := c.client.Pipeline()

	var cardCmd *redis.StringCmd
	if cardUID != "" {
		cardCmd = pipe.Get(ctx, cardKey(cardUID))
	}
	deniedCmd := pipe.Exists(ctx, permDeniedKey(userID))
	passbackCmd := pipe.Get(ctx, passbackKey(userID))

	_, execErr := pipe.Exec(ctx)
	// execErr may be redis.Nil (some keys missing) — that is normal; check each cmd individually.
	if execErr != nil && !errors.Is(execErr, redis.Nil) {
		cacheOps.WithLabelValues("batch_read", "error").Inc()
		return BatchReadResult{}, execErr
	}
	cacheOps.WithLabelValues("batch_read", "ok").Inc()

	var result BatchReadResult

	// Card mapping
	if cardCmd != nil {
		val, err := cardCmd.Result()
		if err == nil {
			result.MappedUserID = val
		} else if !errors.Is(err, redis.Nil) {
			return BatchReadResult{}, err
		}
	}

	// Permission denied
	if n, err := deniedCmd.Result(); err == nil {
		result.IsDenied = n > 0
	} else if !errors.Is(err, redis.Nil) {
		return BatchReadResult{}, err
	}

	// Passback state
	if val, err := passbackCmd.Result(); err == nil {
		switch val {
		case string(model.PassbackIN):
			result.PassbackState = model.PassbackIN
		case string(model.PassbackOUT):
			result.PassbackState = model.PassbackOUT
		default:
			result.PassbackState = model.PassbackNone
		}
	} else {
		result.PassbackState = model.PassbackNone
	}

	return result, nil
}

func (c *RedisCache) IsDenied(ctx context.Context, userID string) (bool, error) {
	n, err := c.client.Exists(ctx, permDeniedKey(userID)).Result()
	if err != nil {
		cacheOps.WithLabelValues("is_denied", "error").Inc()
		return false, err
	}
	if n > 0 {
		cacheOps.WithLabelValues("is_denied", "hit").Inc()
	} else {
		cacheOps.WithLabelValues("is_denied", "miss").Inc()
	}
	return n > 0, nil
}

func (c *RedisCache) GetPassback(ctx context.Context, userID string) (model.PassbackState, error) {
	val, err := c.client.Get(ctx, passbackKey(userID)).Result()
	if err == redis.Nil {
		cacheOps.WithLabelValues("get_passback", "miss").Inc()
		return model.PassbackNone, nil
	}
	if err != nil {
		cacheOps.WithLabelValues("get_passback", "error").Inc()
		return model.PassbackNone, err
	}
	cacheOps.WithLabelValues("get_passback", "hit").Inc()
	switch val {
	case string(model.PassbackIN):
		return model.PassbackIN, nil
	case string(model.PassbackOUT):
		return model.PassbackOUT, nil
	default:
		return model.PassbackNone, nil
	}
}

func (c *RedisCache) SetPassback(ctx context.Context, userID string, state model.PassbackState) error {
	return c.client.Set(ctx, passbackKey(userID), string(state), passbackTTL).Err()
}

// LookupCard returns the userId mapped to the given cardUID.
// Returns ("", nil) if the card is not found in cache.
func (c *RedisCache) LookupCard(ctx context.Context, cardUID string) (string, error) {
	val, err := c.client.Get(ctx, cardKey(cardUID)).Result()
	if err == redis.Nil {
		cacheOps.WithLabelValues("lookup_card", "miss").Inc()
		return "", nil
	}
	if err != nil {
		cacheOps.WithLabelValues("lookup_card", "error").Inc()
		return "", err
	}
	cacheOps.WithLabelValues("lookup_card", "hit").Inc()
	return val, nil
}

// SetCardMapping writes a card→userId mapping into Redis cache.
func (c *RedisCache) SetCardMapping(ctx context.Context, cardUID, userID string) error {
	return c.client.Set(ctx, cardKey(cardUID), userID, cardTTL).Err()
}

func (c *RedisCache) GetDoorStatus(ctx context.Context, doorID string) (string, error) {
	val, err := c.client.Get(ctx, doorStatusKey(doorID)).Result()
	if err == redis.Nil {
		return "OFFLINE", nil
	}
	if err != nil {
		return "", err
	}
	return val, nil
}

func (c *RedisCache) SetDoorStatus(ctx context.Context, doorID, status string) error {
	return c.client.Set(ctx, doorStatusKey(doorID), status, doorTTL).Err()
}

func permDeniedKey(userID string) string {
	return fmt.Sprintf("perm:denied:%s", userID)
}

func passbackKey(userID string) string {
	return fmt.Sprintf("passback:%s", userID)
}

func cardKey(cardUID string) string {
	return fmt.Sprintf("card:%s", cardUID)
}

func doorStatusKey(doorID string) string {
	return fmt.Sprintf("door:status:%s", doorID)
}
