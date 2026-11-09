package limiter

import (
	"context"
	"time"
)

// Storage é a estratégia de persistência do limiter. O limiter só conhece essa
// interface, então trocar o Redis por outra coisa é só escrever uma nova
// implementação e passar no NewLimiter.
type Storage interface {
	// Increment soma 1 no contador da chave e devolve o valor atual. O contador
	// expira sozinho depois de window, o que zera a contagem.
	Increment(ctx context.Context, key string, window time.Duration) (int64, error)

	// Block marca a chave como bloqueada por duration
	Block(ctx context.Context, key string, duration time.Duration) error

	IsBlocked(ctx context.Context, key string) (bool, error)
}
