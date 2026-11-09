package configs

import (
	"fmt"
	"strconv"
	"strings"
	"time"

	"github.com/spf13/viper"
)

type conf struct {
	WebServerPort string `mapstructure:"WEB_SERVER_PORT"`

	// limites em requisições por segundo
	RateLimitIP    int `mapstructure:"RATE_LIMIT_IP"`
	RateLimitToken int `mapstructure:"RATE_LIMIT_TOKEN"`
	// limites específicos por token, no formato "token1:100,token2:50"
	RateLimitTokens string `mapstructure:"RATE_LIMIT_TOKENS"`
	// quanto tempo o IP/token fica bloqueado depois de estourar o limite, em segundos
	BlockDuration int `mapstructure:"BLOCK_DURATION"`

	// qual estratégia de persistência usar: redis ou memory
	Storage string `mapstructure:"LIMITER_STORAGE"`

	RedisHost     string `mapstructure:"REDIS_HOST"`
	RedisPort     string `mapstructure:"REDIS_PORT"`
	RedisPassword string `mapstructure:"REDIS_PASSWORD"`
	RedisDB       int    `mapstructure:"REDIS_DB"`

	TokenLimits map[string]int
}

func (c *conf) BlockTime() time.Duration {
	return time.Duration(c.BlockDuration) * time.Second
}

func (c *conf) RedisAddr() string {
	return c.RedisHost + ":" + c.RedisPort
}

func LoadConfig(path string) (*conf, error) {
	var cfg *conf

	// sem os defaults o AutomaticEnv não funciona com o Unmarshal quando o .env
	// não existe (caso do container, que recebe tudo por variável de ambiente)
	viper.SetDefault("WEB_SERVER_PORT", "8080")
	viper.SetDefault("RATE_LIMIT_IP", 10)
	viper.SetDefault("RATE_LIMIT_TOKEN", 100)
	viper.SetDefault("RATE_LIMIT_TOKENS", "")
	viper.SetDefault("BLOCK_DURATION", 300)
	viper.SetDefault("LIMITER_STORAGE", "redis")
	viper.SetDefault("REDIS_HOST", "localhost")
	viper.SetDefault("REDIS_PORT", "6379")
	viper.SetDefault("REDIS_PASSWORD", "")
	viper.SetDefault("REDIS_DB", 0)

	viper.SetConfigType("env")
	viper.AddConfigPath(path)
	viper.SetConfigFile(path + "/.env")
	viper.AutomaticEnv()

	// o .env é opcional, se não tiver segue com as variáveis de ambiente
	if err := viper.ReadInConfig(); err != nil {
		fmt.Println("arquivo .env não encontrado, usando variáveis de ambiente")
	}

	if err := viper.Unmarshal(&cfg); err != nil {
		return nil, err
	}

	if err := cfg.validate(); err != nil {
		return nil, err
	}

	limits, err := parseTokenLimits(cfg.RateLimitTokens)
	if err != nil {
		return nil, err
	}
	cfg.TokenLimits = limits

	return cfg, nil
}

// BLOCK_DURATION precisa ser positivo porque no Redis um SET com TTL 0 não expira,
// e o bloqueio ficaria para sempre
func (c *conf) validate() error {
	if c.RateLimitIP <= 0 {
		return fmt.Errorf("RATE_LIMIT_IP precisa ser maior que zero, veio %d", c.RateLimitIP)
	}
	if c.RateLimitToken <= 0 {
		return fmt.Errorf("RATE_LIMIT_TOKEN precisa ser maior que zero, veio %d", c.RateLimitToken)
	}
	if c.BlockDuration <= 0 {
		return fmt.Errorf("BLOCK_DURATION precisa ser maior que zero, veio %d", c.BlockDuration)
	}
	return nil
}

func parseTokenLimits(raw string) (map[string]int, error) {
	limits := map[string]int{}
	for _, item := range strings.Split(raw, ",") {
		item = strings.TrimSpace(item)
		if item == "" {
			continue
		}
		token, value, ok := strings.Cut(item, ":")
		if !ok {
			return nil, fmt.Errorf("RATE_LIMIT_TOKENS inválido em %q, o formato é token:limite", item)
		}
		limit, err := strconv.Atoi(strings.TrimSpace(value))
		if err != nil {
			return nil, fmt.Errorf("limite inválido para o token %q: %w", token, err)
		}
		if limit <= 0 {
			return nil, fmt.Errorf("limite do token %q precisa ser maior que zero, veio %d", token, limit)
		}
		limits[strings.TrimSpace(token)] = limit
	}
	return limits, nil
}
