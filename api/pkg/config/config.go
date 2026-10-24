package config

import (
	"encoding/json"
	"fmt"
	"os"
	"strings"

	"github.com/joho/godotenv"
)

type Config struct {
	HTTPAddr     string
	DatabaseURL  string
	LogLevel     string
	CORSOrigins  []string
	Repositories []RepositoryConfig
}

type RepositoryConfig struct {
	Provider         string `json:"provider"`
	FullName         string `json:"full_name"`
	WebhookSecretEnv string `json:"webhook_secret_env"`
	TokenEnv         string `json:"token_env"`
	BaseURLEnv       string `json:"base_url_env"`
}

func Load() (Config, error) {
	_ = godotenv.Load()
	cfg := Config{
		HTTPAddr:    envOr("HTTP_ADDR", ":8080"),
		DatabaseURL: os.Getenv("DATABASE_URL"),
		LogLevel:    envOr("LOG_LEVEL", "info"),
		CORSOrigins: splitCSV(envOr("CORS_ORIGINS", "http://localhost:3000,http://localhost:3001")),
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
