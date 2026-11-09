package middleware

import (
	"context"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/rafaelcorrea26/Desafio-RateLimiter/internal/infra/storage"
	"github.com/rafaelcorrea26/Desafio-RateLimiter/internal/limiter"
	"github.com/stretchr/testify/assert"
)

func newHandler(l *limiter.Limiter) http.Handler {
	ok := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Write([]byte("ok"))
	})
	return RateLimiter(l)(ok)
}

func doRequest(h http.Handler, ip, token string) *httptest.ResponseRecorder {
	req := httptest.NewRequest(http.MethodGet, "/", nil)
	req.RemoteAddr = ip + ":12345"
	if token != "" {
		req.Header.Set("API_KEY", token)
	}
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)
	return rec
}

func TestReturns429WithMessage(t *testing.T) {
	l := limiter.NewLimiter(storage.NewMemoryStorage(), limiter.Config{IPLimit: 2, BlockDuration: time.Minute})
	h := newHandler(l)

	assert.Equal(t, http.StatusOK, doRequest(h, "192.168.0.1", "").Code)
	assert.Equal(t, http.StatusOK, doRequest(h, "192.168.0.1", "").Code)

	rec := doRequest(h, "192.168.0.1", "")
	body, _ := io.ReadAll(rec.Body)
	assert.Equal(t, http.StatusTooManyRequests, rec.Code)
	assert.Equal(t, LimitExceededMessage, string(body))
	assert.Equal(t, "you have reached the maximum number of requests or actions allowed within a certain time frame", string(body))
}

func TestTokenHeaderTakesPrecedence(t *testing.T) {
	l := limiter.NewLimiter(storage.NewMemoryStorage(), limiter.Config{
		IPLimit:       10,
		TokenLimits:   map[string]int{"abc123": 100},
		BlockDuration: time.Minute,
	})
	h := newHandler(l)

	for i := 0; i < 100; i++ {
		rec := doRequest(h, "192.168.0.1", "abc123")
		if !assert.Equal(t, http.StatusOK, rec.Code, "requisição %d deveria passar", i+1) {
			return
		}
	}
	assert.Equal(t, http.StatusTooManyRequests, doRequest(h, "192.168.0.1", "abc123").Code)
}

// sobe um servidor de verdade e manda o header exatamente como "API_KEY", do
// jeito que o curl mandaria. O Go normaliza o nome pra "Api_key" do lado do
// servidor, e aqui dá pra garantir que o Header.Get continua achando.
func TestAPIKeyHeaderOverTheWire(t *testing.T) {
	l := limiter.NewLimiter(storage.NewMemoryStorage(), limiter.Config{
		IPLimit:       1,
		TokenLimits:   map[string]int{"abc123": 5},
		BlockDuration: time.Minute,
	})
	server := httptest.NewServer(newHandler(l))
	defer server.Close()

	statuses := []int{}
	for i := 0; i < 6; i++ {
		req, _ := http.NewRequest(http.MethodGet, server.URL, nil)
		req.Header["API_KEY"] = []string{"abc123"} // atribuindo direto pra não normalizar
		resp, err := http.DefaultClient.Do(req)
		if !assert.NoError(t, err) {
			return
		}
		resp.Body.Close()
		statuses = append(statuses, resp.StatusCode)
	}

	// limite do IP é 1, mas o token deixa passar 5
	assert.Equal(t, []int{200, 200, 200, 200, 200, 429}, statuses)
}

func TestBlockedClientStaysBlocked(t *testing.T) {
	l := limiter.NewLimiter(storage.NewMemoryStorage(), limiter.Config{IPLimit: 1, BlockDuration: 2 * time.Second})
	h := newHandler(l)

	assert.Equal(t, http.StatusOK, doRequest(h, "192.168.0.1", "").Code)
	assert.Equal(t, http.StatusTooManyRequests, doRequest(h, "192.168.0.1", "").Code)

	// passou da janela de 1s mas ainda está dentro do bloqueio
	time.Sleep(1100 * time.Millisecond)
	assert.Equal(t, http.StatusTooManyRequests, doRequest(h, "192.168.0.1", "").Code)

	// outro IP segue normal
	assert.Equal(t, http.StatusOK, doRequest(h, "192.168.0.2", "").Code)
}

type failingStorage struct{}

func (failingStorage) Increment(context.Context, string, time.Duration) (int64, error) {
	return 0, errors.New("redis fora do ar")
}
func (failingStorage) Block(context.Context, string, time.Duration) error { return nil }
func (failingStorage) IsBlocked(context.Context, string) (bool, error) {
	return false, errors.New("redis fora do ar")
}

func TestStorageErrorLetsRequestThrough(t *testing.T) {
	l := limiter.NewLimiter(failingStorage{}, limiter.Config{IPLimit: 1})
	h := newHandler(l)

	assert.Equal(t, http.StatusOK, doRequest(h, "192.168.0.1", "").Code)
	assert.Equal(t, http.StatusOK, doRequest(h, "192.168.0.1", "").Code)
}

// storage que não responde, só para quando o contexto vence
type slowStorage struct{}

func (slowStorage) Increment(context.Context, string, time.Duration) (int64, error) {
	return 1, nil
}
func (slowStorage) Block(context.Context, string, time.Duration) error { return nil }
func (slowStorage) IsBlocked(ctx context.Context, _ string) (bool, error) {
	select {
	case <-ctx.Done():
		return false, ctx.Err()
	case <-time.After(3 * time.Second):
		return false, errors.New("o contexto não venceu")
	}
}

func TestSlowStorageDoesNotHoldRequest(t *testing.T) {
	l := limiter.NewLimiter(slowStorage{}, limiter.Config{IPLimit: 1})
	h := newHandler(l)

	start := time.Now()
	rec := doRequest(h, "192.168.0.1", "")

	assert.Equal(t, http.StatusOK, rec.Code)
	assert.Less(t, time.Since(start), time.Second)
}
