package main

import (
	"context"
	"fmt"
	"log"
	"net/http"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/go-chi/chi/v5/middleware"
	"github.com/rafaelcorrea26/Desafio-RateLimiter/configs"
	"github.com/rafaelcorrea26/Desafio-RateLimiter/internal/infra/storage"
	ratelimit "github.com/rafaelcorrea26/Desafio-RateLimiter/internal/infra/web/middleware"
	"github.com/rafaelcorrea26/Desafio-RateLimiter/internal/limiter"
	"github.com/redis/go-redis/v9"
)

func main() {
	cfg, err := configs.LoadConfig(".")
	if err != nil {
		panic(err)
	}

	store, err := newStorage(cfg.Storage, cfg.RedisAddr(), cfg.RedisPassword, cfg.RedisDB)
	if err != nil {
		panic(err)
	}

	rateLimiter := limiter.NewLimiter(store, limiter.Config{
		IPLimit:       cfg.RateLimitIP,
		TokenLimit:    cfg.RateLimitToken,
		TokenLimits:   cfg.TokenLimits,
		BlockDuration: cfg.BlockTime(),
	})

	r := chi.NewRouter()
	r.Use(middleware.Logger)
	r.Use(middleware.Recoverer)
	r.Use(ratelimit.RateLimiter(rateLimiter))

	r.Get("/", func(w http.ResponseWriter, r *http.Request) {
		w.Write([]byte("Hello, World!"))
	})

	log.Printf("servidor rodando na porta %s (storage: %s, limite por IP: %d req/s, limite por token: %d req/s, bloqueio: %s)",
		cfg.WebServerPort, cfg.Storage, cfg.RateLimitIP, cfg.RateLimitToken, cfg.BlockTime())
	if err := http.ListenAndServe(":"+cfg.WebServerPort, r); err != nil {
		log.Fatal(err)
	}
}

// aqui é o único lugar que sabe qual estratégia está sendo usada
func newStorage(kind, redisAddr, redisPassword string, redisDB int) (limiter.Storage, error) {
	switch kind {
	case "redis":
		client := redis.NewClient(&redis.Options{
			Addr:     redisAddr,
			Password: redisPassword,
			DB:       redisDB,
		})
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		if err := client.Ping(ctx).Err(); err != nil {
			return nil, fmt.Errorf("não consegui conectar no redis em %s: %w", redisAddr, err)
		}
		return storage.NewRedisStorage(client), nil
	case "memory":
		return storage.NewMemoryStorage(), nil
	default:
		return nil, fmt.Errorf("LIMITER_STORAGE desconhecido: %q (use redis ou memory)", kind)
	}
}
