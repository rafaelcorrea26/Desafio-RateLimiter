package configs

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestParseTokenLimits(t *testing.T) {
	limits, err := parseTokenLimits("abc123:100, xyz789:50 ,")
	assert.NoError(t, err)
	assert.Equal(t, map[string]int{"abc123": 100, "xyz789": 50}, limits)

	limits, err = parseTokenLimits("")
	assert.NoError(t, err)
	assert.Empty(t, limits)

	_, err = parseTokenLimits("abc123")
	assert.Error(t, err)

	_, err = parseTokenLimits("abc123:muito")
	assert.Error(t, err)
}

func TestLoadConfigRejectsInvalidValues(t *testing.T) {
	cases := []struct{ name, value string }{
		{"BLOCK_DURATION", "0"},
		{"BLOCK_DURATION", "-5"},
		{"RATE_LIMIT_IP", "0"},
		{"RATE_LIMIT_IP", "-1"},
		{"RATE_LIMIT_TOKEN", "0"},
		{"RATE_LIMIT_TOKEN", "-1"},
		{"RATE_LIMIT_TOKENS", "abc123:0"},
		{"RATE_LIMIT_TOKENS", "abc123:-1"},
	}

	for _, c := range cases {
		t.Run(c.name+"="+c.value, func(t *testing.T) {
			t.Setenv(c.name, c.value)

			_, err := LoadConfig(t.TempDir())
			assert.Error(t, err)
		})
	}
}

func TestLoadConfigAcceptsValidValues(t *testing.T) {
	t.Setenv("BLOCK_DURATION", "1")
	t.Setenv("RATE_LIMIT_IP", "1")
	t.Setenv("RATE_LIMIT_TOKEN", "1")
	t.Setenv("RATE_LIMIT_TOKENS", "abc123:1")

	cfg, err := LoadConfig(t.TempDir())
	assert.NoError(t, err)
	assert.Equal(t, map[string]int{"abc123": 1}, cfg.TokenLimits)
}
