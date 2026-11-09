package limiter_test

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/rafaelcorrea26/Desafio-RateLimiter/internal/infra/storage"
	"github.com/rafaelcorrea26/Desafio-RateLimiter/internal/limiter"
	"github.com/stretchr/testify/assert"
)

func newLimiter(cfg limiter.Config) *limiter.Limiter {
	return limiter.NewLimiter(storage.NewMemoryStorage(), cfg)
}

// faz n requisições seguidas e conta quantas passaram
func hit(t *testing.T, l *limiter.Limiter, ip, token string, n int) int {
	t.Helper()
	passed := 0
	for i := 0; i < n; i++ {
		ok, err := l.Allow(context.Background(), ip, token)
		assert.NoError(t, err)
		if ok {
			passed++
		}
	}
	return passed
}

func TestLimitByIP(t *testing.T) {
	l := newLimiter(limiter.Config{IPLimit: 5, TokenLimit: 100, BlockDuration: time.Minute})

	assert.Equal(t, 5, hit(t, l, "10.0.0.1", "", 10))
}

func TestIPsAreIndependent(t *testing.T) {
	l := newLimiter(limiter.Config{IPLimit: 3, BlockDuration: time.Minute})

	assert.Equal(t, 3, hit(t, l, "10.0.0.1", "", 5))
	// o primeiro IP estourou, mas o segundo não tem nada a ver com isso
	assert.Equal(t, 3, hit(t, l, "10.0.0.2", "", 5))
}

func TestTokenOverridesIP(t *testing.T) {
	// o exemplo do enunciado: 10 req/s por IP e 100 req/s pro token
	l := newLimiter(limiter.Config{
		IPLimit:       10,
		TokenLimits:   map[string]int{"abc123": 100},
		BlockDuration: time.Minute,
	})

	assert.Equal(t, 100, hit(t, l, "10.0.0.1", "abc123", 150))
}

func TestTokenWithLowerLimitThanIP(t *testing.T) {
	// a precedência vale pros dois lados: se o token tem limite menor, é ele que conta
	l := newLimiter(limiter.Config{
		IPLimit:       10,
		TokenLimits:   map[string]int{"free": 2},
		BlockDuration: time.Minute,
	})

	assert.Equal(t, 2, hit(t, l, "10.0.0.1", "free", 10))
}

func TestUnknownTokenUsesDefaultTokenLimit(t *testing.T) {
	l := newLimiter(limiter.Config{
		IPLimit:       5,
		TokenLimit:    20,
		TokenLimits:   map[string]int{"abc123": 100},
		BlockDuration: time.Minute,
	})

	assert.Equal(t, 20, hit(t, l, "10.0.0.1", "outro-token", 30))
}

func TestBlockedIPDoesNotAffectToken(t *testing.T) {
	l := newLimiter(limiter.Config{
		IPLimit:       2,
		TokenLimits:   map[string]int{"abc123": 10},
		BlockDuration: time.Minute,
	})

	// estoura o IP sem token
	assert.Equal(t, 2, hit(t, l, "10.0.0.1", "", 5))
	// mesmo IP, mas agora com token: segue a regra do token
	assert.Equal(t, 10, hit(t, l, "10.0.0.1", "abc123", 10))
	// e sem token continua bloqueado
	assert.Equal(t, 0, hit(t, l, "10.0.0.1", "", 1))
}

func TestCounterResetsAfterOneSecond(t *testing.T) {
	// limite 3 e bloqueio de zero: dá pra ver só a janela de 1s funcionando
	l := newLimiter(limiter.Config{IPLimit: 3, BlockDuration: 0})

	assert.Equal(t, 3, hit(t, l, "10.0.0.1", "", 3))
	time.Sleep(1100 * time.Millisecond)
	assert.Equal(t, 3, hit(t, l, "10.0.0.1", "", 3))
}

func TestBlockLastsLongerThanWindow(t *testing.T) {
	l := newLimiter(limiter.Config{IPLimit: 2, BlockDuration: 2 * time.Second})

	assert.Equal(t, 2, hit(t, l, "10.0.0.1", "", 3))

	// a janela de 1s já passou, mas o bloqueio de 2s ainda não
	time.Sleep(1100 * time.Millisecond)
	assert.Equal(t, 0, hit(t, l, "10.0.0.1", "", 1))

	time.Sleep(1100 * time.Millisecond)
	assert.Equal(t, 2, hit(t, l, "10.0.0.1", "", 2))
}

// storage que sempre falha, pra ver se o erro chega em quem chamou
type brokenStorage struct{}

func (brokenStorage) Increment(context.Context, string, time.Duration) (int64, error) {
	return 0, errors.New("fora do ar")
}
func (brokenStorage) Block(context.Context, string, time.Duration) error { return nil }
func (brokenStorage) IsBlocked(context.Context, string) (bool, error)    { return false, nil }

func TestStorageErrorIsReturned(t *testing.T) {
	l := limiter.NewLimiter(brokenStorage{}, limiter.Config{IPLimit: 10})

	_, err := l.Allow(context.Background(), "10.0.0.1", "")
	assert.EqualError(t, err, "fora do ar")
}
