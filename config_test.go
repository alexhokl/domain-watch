package main

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func env(m map[string]string) func(string) string {
	return func(k string) string { return m[k] }
}

func TestLoadConfig(t *testing.T) {
	cfg, err := loadConfig(env(map[string]string{
		"DOMAINS":      " Ottimo-Consulting.com, example.net ,,",
		"STATE_BUCKET": "my-bucket",
		"STATE_PREFIX": "/watch/",
		"WARN_DAYS":    "30, 7",
	}))

	require.NoError(t, err)
	assert.Equal(t, []string{"ottimo-consulting.com", "example.net"}, cfg.Domains)
	assert.Equal(t, "my-bucket", cfg.StateBucket)
	assert.Equal(t, "watch", cfg.StatePrefix)
	assert.Equal(t, []int{7, 30}, cfg.WarnDays)
}

func TestLoadConfigDefaults(t *testing.T) {
	cfg, err := loadConfig(env(map[string]string{"DOMAINS": "example.com", "STATE_DIR": "/tmp/x"}))

	require.NoError(t, err)
	assert.Equal(t, defaultStatePrefix, cfg.StatePrefix)
	assert.Equal(t, []int{7, 30, 60}, cfg.WarnDays)
}

func TestLoadConfigErrors(t *testing.T) {
	tests := []struct {
		name string
		env  map[string]string
	}{
		{"missing domains", map[string]string{"STATE_DIR": "/tmp/x"}},
		{"blank domains", map[string]string{"DOMAINS": " , ", "STATE_DIR": "/tmp/x"}},
		{"no state location", map[string]string{"DOMAINS": "example.com"}},
		{"both state locations", map[string]string{"DOMAINS": "example.com", "STATE_DIR": "/x", "STATE_BUCKET": "b"}},
		{"bad warn days", map[string]string{"DOMAINS": "example.com", "STATE_DIR": "/x", "WARN_DAYS": "soon"}},
		{"negative warn days", map[string]string{"DOMAINS": "example.com", "STATE_DIR": "/x", "WARN_DAYS": "-3"}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := loadConfig(env(tt.env))
			assert.Error(t, err)
		})
	}
}
