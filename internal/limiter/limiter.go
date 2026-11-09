package limiter

import (
	"context"
	"time"
)

// a janela de contagem é sempre de 1 segundo, já que os limites são em req/s
const window = time.Second

type Config struct {
	IPLimit       int
	TokenLimit    int            // limite padrão pra qualquer token
	TokenLimits   map[string]int // limites específicos, sobrescrevem o padrão
	BlockDuration time.Duration
}

type Limiter struct {
	storage Storage
	config  Config
}

func NewLimiter(storage Storage, config Config) *Limiter {
	if config.TokenLimits == nil {
		config.TokenLimits = map[string]int{}
	}
	return &Limiter{storage: storage, config: config}
}

// Allow diz se a requisição pode passar. Quando vem token, a regra do token é a
// que vale e o IP é ignorado.
func (l *Limiter) Allow(ctx context.Context, ip, token string) (bool, error) {
	key, limit := l.resolve(ip, token)

	blocked, err := l.storage.IsBlocked(ctx, key)
	if err != nil {
		return false, err
	}
	if blocked {
		return false, nil
	}

	count, err := l.storage.Increment(ctx, key, window)
	if err != nil {
		return false, err
	}

	if count > int64(limit) {
		if err := l.storage.Block(ctx, key, l.config.BlockDuration); err != nil {
			return false, err
		}
		return false, nil
	}
	return true, nil
}

func (l *Limiter) resolve(ip, token string) (string, int) {
	if token != "" {
		if limit, ok := l.config.TokenLimits[token]; ok {
			return "token:" + token, limit
		}
		return "token:" + token, l.config.TokenLimit
	}
	return "ip:" + ip, l.config.IPLimit
}
