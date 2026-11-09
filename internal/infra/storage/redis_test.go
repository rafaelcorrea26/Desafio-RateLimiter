package storage

import (
	"context"
	"os"
	"testing"
	"time"

	"github.com/alicebob/miniredis/v2"
	"github.com/redis/go-redis/v9"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// Se REDIS_ADDR estiver definido (é o caso do docker compose) os testes rodam
// contra o Redis de verdade. Senão sobe um miniredis, que é um Redis em memória
// feito pra teste, e dá pra rodar sem docker nenhum.
func newRedisClient(t *testing.T) (*redis.Client, *miniredis.Miniredis) {
	t.Helper()

	if addr := os.Getenv("REDIS_ADDR"); addr != "" {
		// DB 1 pra não apagar os contadores da aplicação, que usa o DB 0
		client := redis.NewClient(&redis.Options{Addr: addr, DB: 1})
		require.NoError(t, client.Ping(context.Background()).Err())
		require.NoError(t, client.FlushDB(context.Background()).Err())
		t.Cleanup(func() { client.Close() })
		return client, nil
	}

	mr := miniredis.RunT(t)
	client := redis.NewClient(&redis.Options{Addr: mr.Addr()})
	t.Cleanup(func() { client.Close() })
	return client, mr
}

// no miniredis o tempo não anda sozinho, tem que avançar com FastForward
func advance(mr *miniredis.Miniredis, d time.Duration) {
	if mr != nil {
		mr.FastForward(d)
		return
	}
	time.Sleep(d)
}

func TestRedisIncrement(t *testing.T) {
	client, _ := newRedisClient(t)
	s := NewRedisStorage(client)
	ctx := context.Background()

	for i := int64(1); i <= 3; i++ {
		n, err := s.Increment(ctx, "ip:1.1.1.1", time.Second)
		assert.NoError(t, err)
		assert.Equal(t, i, n)
	}

	ttl := client.PTTL(ctx, "ratelimit:count:ip:1.1.1.1").Val()
	assert.Greater(t, ttl, time.Duration(0), "o contador precisa ter TTL")
	assert.LessOrEqual(t, ttl, time.Second)
}

func TestRedisCounterExpires(t *testing.T) {
	client, mr := newRedisClient(t)
	s := NewRedisStorage(client)
	ctx := context.Background()

	s.Increment(ctx, "ip:1.1.1.1", time.Second)
	s.Increment(ctx, "ip:1.1.1.1", time.Second)

	advance(mr, 1100*time.Millisecond)

	n, err := s.Increment(ctx, "ip:1.1.1.1", time.Second)
	assert.NoError(t, err)
	assert.Equal(t, int64(1), n)
}

func TestRedisBlock(t *testing.T) {
	client, mr := newRedisClient(t)
	s := NewRedisStorage(client)
	ctx := context.Background()

	blocked, err := s.IsBlocked(ctx, "token:abc")
	assert.NoError(t, err)
	assert.False(t, blocked)

	assert.NoError(t, s.Block(ctx, "token:abc", 2*time.Second))

	blocked, _ = s.IsBlocked(ctx, "token:abc")
	assert.True(t, blocked)

	advance(mr, 2100*time.Millisecond)

	blocked, _ = s.IsBlocked(ctx, "token:abc")
	assert.False(t, blocked)
}
