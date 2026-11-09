package storage

import (
	"context"
	"time"

	"github.com/redis/go-redis/v9"
)

// INCR e PEXPIRE no mesmo script pra ser atômico. Fazendo em dois comandos
// separados, se o processo caísse no meio a chave ficaria sem TTL e o contador
// nunca mais zerava.
var incrementScript = redis.NewScript(`
local current = redis.call("INCR", KEYS[1])
if current == 1 then
	redis.call("PEXPIRE", KEYS[1], ARGV[1])
end
return current
`)

type RedisStorage struct {
	client *redis.Client
}

func NewRedisStorage(client *redis.Client) *RedisStorage {
	return &RedisStorage{client: client}
}

func (s *RedisStorage) Increment(ctx context.Context, key string, window time.Duration) (int64, error) {
	return incrementScript.Run(ctx, s.client, []string{"ratelimit:count:" + key}, window.Milliseconds()).Int64()
}

func (s *RedisStorage) Block(ctx context.Context, key string, duration time.Duration) error {
	return s.client.Set(ctx, "ratelimit:block:"+key, 1, duration).Err()
}

func (s *RedisStorage) IsBlocked(ctx context.Context, key string) (bool, error) {
	n, err := s.client.Exists(ctx, "ratelimit:block:"+key).Result()
	if err != nil {
		return false, err
	}
	return n > 0, nil
}
