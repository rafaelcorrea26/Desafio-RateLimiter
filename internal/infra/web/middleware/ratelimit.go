package middleware

import (
	"context"
	"log"
	"net"
	"net/http"
	"time"

	"github.com/rafaelcorrea26/Desafio-RateLimiter/internal/limiter"
)

const LimitExceededMessage = "you have reached the maximum number of requests or actions allowed within a certain time frame"

// sem esse prazo, com o Redis fora do ar cada requisição ficava mais de um minuto
// esperando as tentativas de reconexão
const storageTimeout = 500 * time.Millisecond

func RateLimiter(l *limiter.Limiter) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			token := r.Header.Get("API_KEY")

			ctx, cancel := context.WithTimeout(r.Context(), storageTimeout)
			defer cancel()

			allowed, err := l.Allow(ctx, clientIP(r), token)
			if err != nil {
				// se o storage cair a API continua respondendo, só fica sem limite
				// até ele voltar. Preferi isso a derrubar tudo junto com o Redis.
				log.Printf("rate limiter: erro consultando storage: %v", err)
				next.ServeHTTP(w, r)
				return
			}

			if !allowed {
				w.Header().Set("Content-Type", "text/plain; charset=utf-8")
				w.WriteHeader(http.StatusTooManyRequests)
				w.Write([]byte(LimitExceededMessage))
				return
			}

			next.ServeHTTP(w, r)
		})
	}
}

// usa o RemoteAddr de propósito. Ler X-Forwarded-For aqui deixaria qualquer um
// fugir do limite só mandando um IP diferente no header. Se a aplicação ficar
// atrás de um proxy, o certo é colocar o middleware.RealIP do chi antes.
func clientIP(r *http.Request) string {
	host, _, err := net.SplitHostPort(r.RemoteAddr)
	if err != nil {
		return r.RemoteAddr
	}
	return host
}
