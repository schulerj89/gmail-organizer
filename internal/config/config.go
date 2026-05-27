package config

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"
)

type Config struct {
	Host                   string
	Port                   int
	DataDir                string
	GoogleClientSecretFile string
	OpenAIKeyFile          string
	OpenAIModel            string
	OpenAIMaxOutputTokens  int
	OpenAIMaxRetries       int
	OpenAIRequestDelayMs   int
	OpenAIChunkSize        int
	OpenAITimeoutSeconds   int
	EnableOpenAI           bool
	FrontendDistDir        string
	OAuthRedirectURLValue  string
	MonitorInterval        int
	MonitorCacheLimit      int
	ScanCacheLimit         int
}

func Load() (Config, error) {
	root, err := os.Getwd()
	if err != nil {
		return Config{}, err
	}

	port := intFromEnv("GMAIL_ORGANIZER_PORT", 8787)
	cfg := Config{
		Host:                  stringFromEnv("GMAIL_ORGANIZER_HOST", "127.0.0.1"),
		Port:                  port,
		DataDir:               stringFromEnv("GMAIL_ORGANIZER_DATA_DIR", filepath.Join(root, "data")),
		OpenAIModel:           stringFromEnv("OPENAI_MODEL", "gpt-5-mini"),
		OpenAIMaxOutputTokens: intFromEnv("OPENAI_MAX_OUTPUT_TOKENS", 2000),
		OpenAIMaxRetries:      intFromEnv("OPENAI_MAX_RETRIES", 3),
		OpenAIRequestDelayMs:  intFromEnv("OPENAI_REQUEST_DELAY_MS", 1200),
		OpenAIChunkSize:       intFromEnv("OPENAI_CLASSIFY_CHUNK_SIZE", 25),
		OpenAITimeoutSeconds:  intFromEnv("OPENAI_TIMEOUT_SECONDS", 45),
		EnableOpenAI:          boolFromEnv("GMAIL_ORGANIZER_ENABLE_OPENAI", true),
		FrontendDistDir:       stringFromEnv("GMAIL_ORGANIZER_FRONTEND_DIST", filepath.Join(root, "web", "dist")),
		OAuthRedirectURLValue: stringFromEnv("GMAIL_ORGANIZER_OAUTH_REDIRECT_URL", ""),
		MonitorInterval:       intFromEnv("GMAIL_ORGANIZER_MONITOR_INTERVAL_SECONDS", 60),
		MonitorCacheLimit:     intFromEnv("GMAIL_ORGANIZER_MONITOR_CACHE_LIMIT", 500),
		ScanCacheLimit:        intFromEnv("GMAIL_ORGANIZER_SCAN_CACHE_LIMIT", 1000),
	}

	cfg.GoogleClientSecretFile = stringFromEnv("GOOGLE_CLIENT_SECRET_FILE", discoverFirst(root, "client_secret*.json"))
	cfg.OpenAIKeyFile = stringFromEnv("OPENAI_API_KEY_FILE", discoverFirst(root, "openai_key.txt"))

	if cfg.GoogleClientSecretFile == "" {
		return Config{}, errors.New("GOOGLE_CLIENT_SECRET_FILE is not set and no client_secret*.json was found in parent directories")
	}
	if cfg.OpenAIKeyFile == "" {
		return Config{}, errors.New("OPENAI_API_KEY_FILE is not set and no openai_key.txt was found in parent directories")
	}
	if cfg.Port <= 0 || cfg.Port > 65535 {
		return Config{}, fmt.Errorf("invalid port %d", cfg.Port)
	}
	if cfg.MonitorInterval < 15 {
		cfg.MonitorInterval = 15
	}
	if cfg.MonitorCacheLimit < 50 {
		cfg.MonitorCacheLimit = 50
	}
	if cfg.ScanCacheLimit < 100 {
		cfg.ScanCacheLimit = 100
	}
	if cfg.OpenAIMaxOutputTokens < 256 {
		cfg.OpenAIMaxOutputTokens = 256
	}
	if cfg.OpenAIMaxOutputTokens > 8000 {
		cfg.OpenAIMaxOutputTokens = 8000
	}
	if cfg.OpenAIMaxRetries < 0 {
		cfg.OpenAIMaxRetries = 0
	}
	if cfg.OpenAIMaxRetries > 6 {
		cfg.OpenAIMaxRetries = 6
	}
	if cfg.OpenAIRequestDelayMs < 0 {
		cfg.OpenAIRequestDelayMs = 0
	}
	if cfg.OpenAIRequestDelayMs > 30000 {
		cfg.OpenAIRequestDelayMs = 30000
	}
	if cfg.OpenAIChunkSize < 1 {
		cfg.OpenAIChunkSize = 1
	}
	if cfg.OpenAIChunkSize > 50 {
		cfg.OpenAIChunkSize = 50
	}
	if cfg.OpenAITimeoutSeconds < 10 {
		cfg.OpenAITimeoutSeconds = 10
	}
	if cfg.OpenAITimeoutSeconds > 120 {
		cfg.OpenAITimeoutSeconds = 120
	}
	return cfg, nil
}

func (c Config) ListenAddr() string {
	return fmt.Sprintf("%s:%d", c.Host, c.Port)
}

func (c Config) OAuthRedirectURL() string {
	if c.OAuthRedirectURLValue != "" {
		return c.OAuthRedirectURLValue
	}
	return fmt.Sprintf("http://127.0.0.1:%d/api/auth/google/callback", c.Port)
}

func discoverFirst(start string, pattern string) string {
	current := start
	for i := 0; i < 4; i++ {
		matches, _ := filepath.Glob(filepath.Join(current, pattern))
		if len(matches) > 0 {
			return matches[0]
		}
		parent := filepath.Dir(current)
		if parent == current {
			break
		}
		current = parent
	}
	return ""
}

func stringFromEnv(key, fallback string) string {
	value := strings.TrimSpace(os.Getenv(key))
	if value == "" {
		return fallback
	}
	return value
}

func intFromEnv(key string, fallback int) int {
	value := strings.TrimSpace(os.Getenv(key))
	if value == "" {
		return fallback
	}
	parsed, err := strconv.Atoi(value)
	if err != nil {
		return fallback
	}
	return parsed
}

func boolFromEnv(key string, fallback bool) bool {
	value := strings.TrimSpace(os.Getenv(key))
	if value == "" {
		return fallback
	}
	parsed, err := strconv.ParseBool(value)
	if err != nil {
		return fallback
	}
	return parsed
}
