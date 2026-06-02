package config

import (
	"os"
	"strconv"
	"strings"
)

type Config struct {
	ListenAddr           string
	DatabaseDriver       string
	DatabaseDSN          string
	APIToken             string
	WorkerToken          string
	SecretKey            string
	RunMigrations        bool
	InternalExecutor     bool
	InternalWorkerID     string
	InternalPollInterval int
	InternalLeaseSeconds int
	GatewayCallbackURL   string
	GatewayCallbackToken string
	GatewayTimeoutSecs   int
}

func Load() Config {
	return Config{
		ListenAddr:           envString("AUTOMATION_LISTEN_ADDR", ":8091"),
		DatabaseDriver:       strings.ToLower(envString("AUTOMATION_DB_DRIVER", "sqlite")),
		DatabaseDSN:          envString("AUTOMATION_DB_DSN", "./data/token-account-automation.db"),
		APIToken:             envString("AUTOMATION_API_TOKEN", ""),
		WorkerToken:          envString("AUTOMATION_WORKER_TOKEN", ""),
		SecretKey:            envString("AUTOMATION_SECRET_KEY", ""),
		RunMigrations:        envBool("AUTOMATION_RUN_MIGRATIONS", true),
		InternalExecutor:     envBool("AUTOMATION_INTERNAL_EXECUTOR", true),
		InternalWorkerID:     envString("AUTOMATION_INTERNAL_WORKER_ID", "internal-api-1"),
		InternalPollInterval: envInt("AUTOMATION_INTERNAL_POLL_INTERVAL", 2),
		InternalLeaseSeconds: envInt("AUTOMATION_INTERNAL_LEASE_SECONDS", 60),
		GatewayCallbackURL:   envString("AUTOMATION_GATEWAY_CALLBACK_URL", ""),
		GatewayCallbackToken: envString("AUTOMATION_GATEWAY_CALLBACK_TOKEN", ""),
		GatewayTimeoutSecs:   envInt("AUTOMATION_GATEWAY_CALLBACK_TIMEOUT_SECONDS", 5),
	}
}

func envString(name string, fallback string) string {
	value := strings.TrimSpace(os.Getenv(name))
	if value == "" {
		return fallback
	}
	return value
}

func envBool(name string, fallback bool) bool {
	value := strings.TrimSpace(os.Getenv(name))
	if value == "" {
		return fallback
	}
	parsed, err := strconv.ParseBool(value)
	if err != nil {
		return fallback
	}
	return parsed
}

func envInt(name string, fallback int) int {
	value := strings.TrimSpace(os.Getenv(name))
	if value == "" {
		return fallback
	}
	parsed, err := strconv.Atoi(value)
	if err != nil {
		return fallback
	}
	return parsed
}
