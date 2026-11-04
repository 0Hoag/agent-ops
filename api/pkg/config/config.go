package config

import (
	"encoding/json"
	"fmt"
	"os"
	"strings"
	"time"

	"github.com/joho/godotenv"
)

type Config struct {
	HTTPAddr              string
	DatabaseURL           string
	LogLevel              string
	CORSOrigins           []string
	Repositories          []RepositoryConfig
	AgentRunToken         string
	AgentRunModelPrices   map[string]json.RawMessage
	ExchangeRateVNDPerUSD float64
	JWTSecret             string
	JWTExpiration         time.Duration
	EncryptionKey         []byte
	WorkerQueueSize       int
	WorkerConcurrency     int
	WorkerScriptRoots     []string
	WorkerWorktreeRoots   []string
	WorkerScripts         map[string]string
}

type RepositoryConfig struct {
	Provider         string `json:"provider"`
	FullName         string `json:"full_name"`
	OrganizationID   string `json:"organization_id"`
	WebhookSecretEnv string `json:"webhook_secret_env"`
	TokenEnv         string `json:"token_env"`
	BaseURLEnv       string `json:"base_url_env"`
}

func Load() (Config, error) {
	_ = godotenv.Load()
	cfg := Config{
		HTTPAddr:        envOr("HTTP_ADDR", ":8080"),
		DatabaseURL:     os.Getenv("DATABASE_URL"),
		LogLevel:        envOr("LOG_LEVEL", "info"),
		CORSOrigins:     splitCSV(envOr("CORS_ORIGINS", "http://localhost:3000,http://localhost:3001")),
		AgentRunToken:   os.Getenv("AGENT_RUN_TOKEN"),
		JWTSecret:       os.Getenv("JWT_SECRET"),
		JWTExpiration:   24 * time.Hour,
		EncryptionKey:   []byte(os.Getenv("REPOSITORY_ENCRYPTION_KEY")),
		WorkerQueueSize: 16, WorkerConcurrency: 1,
		WorkerScriptRoots:   splitCSV(envOr("WORKER_SCRIPT_ROOTS", ".")),
		WorkerWorktreeRoots: splitCSV(envOr("WORKER_WORKTREE_ROOTS", ".")),
		WorkerScripts:       map[string]string{"codex": "scripts/run-worker.sh", "opencode": "scripts/run-worker-opencode.sh"},
	}
	if raw := os.Getenv("WORKER_SCRIPTS_JSON"); raw != "" {
		if err := json.Unmarshal([]byte(raw), &cfg.WorkerScripts); err != nil {
			return Config{}, fmt.Errorf("WORKER_SCRIPTS_JSON is invalid")
		}
	}
	if raw := os.Getenv("EXCHANGE_RATE_VND_PER_USD"); raw != "" {
		if _, err := fmt.Sscan(raw, &cfg.ExchangeRateVNDPerUSD); err != nil || cfg.ExchangeRateVNDPerUSD <= 0 {
			return Config{}, fmt.Errorf("EXCHANGE_RATE_VND_PER_USD is invalid")
		}
	}
	if cfg.JWTSecret == "" {
		return Config{}, fmt.Errorf("JWT_SECRET is required")
	}
	if raw := os.Getenv("JWT_EXPIRATION"); raw != "" {
		duration, err := time.ParseDuration(raw)
		if err != nil || duration <= 0 {
			return Config{}, fmt.Errorf("JWT_EXPIRATION is invalid")
		}
		cfg.JWTExpiration = duration
	}
	if raw := os.Getenv("AGENT_RUN_MODEL_PRICES"); raw != "" {
		if err := json.Unmarshal([]byte(raw), &cfg.AgentRunModelPrices); err != nil {
			return Config{}, fmt.Errorf("AGENT_RUN_MODEL_PRICES is invalid")
		}
	}
	if raw := os.Getenv("SCM_REPOSITORIES_JSON"); raw != "" {
		if err := json.Unmarshal([]byte(raw), &cfg.Repositories); err != nil {
			return Config{}, fmt.Errorf("SCM_REPOSITORIES_JSON is invalid")
		}
	}
	if cfg.DatabaseURL == "" {
		return Config{}, fmt.Errorf("DATABASE_URL is required")
	}
	return cfg, nil
}

func splitCSV(value string) []string {
	var values []string
	for _, item := range strings.Split(value, ",") {
		if item = strings.TrimSpace(item); item != "" {
			values = append(values, item)
		}
	}
	return values
}

func envOr(key, fallback string) string {
	if value := os.Getenv(key); value != "" {
		return value
	}
	return fallback
}
