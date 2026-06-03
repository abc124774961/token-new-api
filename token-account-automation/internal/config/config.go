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
	DesktopToken         string
	SecretKey            string
	RunMigrations        bool
	InternalExecutor     bool
	InternalWorkerID     string
	InternalPollInterval int
	InternalLeaseSeconds int
	BrowserLoginExecutor string
	GatewayCallbackURL   string
	GatewayCallbackToken string
	GatewayTimeoutSecs   int
}

func Load() Config {
	loadEnvFile()
	return Config{
		ListenAddr:           envString("AUTOMATION_LISTEN_ADDR", ":8091"),
		DatabaseDriver:       strings.ToLower(envString("AUTOMATION_DB_DRIVER", "sqlite")),
		DatabaseDSN:          envString("AUTOMATION_DB_DSN", "./data/token-account-automation.db"),
		APIToken:             envString("AUTOMATION_API_TOKEN", ""),
		WorkerToken:          envString("AUTOMATION_WORKER_TOKEN", ""),
		DesktopToken:         envString("AUTOMATION_DESKTOP_TOKEN", ""),
		SecretKey:            envString("AUTOMATION_SECRET_KEY", ""),
		RunMigrations:        envBool("AUTOMATION_RUN_MIGRATIONS", true),
		InternalExecutor:     envBool("AUTOMATION_INTERNAL_EXECUTOR", true),
		InternalWorkerID:     envString("AUTOMATION_INTERNAL_WORKER_ID", "internal-api-1"),
		InternalPollInterval: envInt("AUTOMATION_INTERNAL_POLL_INTERVAL", 2),
		InternalLeaseSeconds: envInt("AUTOMATION_INTERNAL_LEASE_SECONDS", 60),
		BrowserLoginExecutor: normalizeExecutor(envString("AUTOMATION_BROWSER_LOGIN_EXECUTOR", "browser_playwright")),
		GatewayCallbackURL:   envString("AUTOMATION_GATEWAY_CALLBACK_URL", ""),
		GatewayCallbackToken: envString("AUTOMATION_GATEWAY_CALLBACK_TOKEN", ""),
		GatewayTimeoutSecs:   envInt("AUTOMATION_GATEWAY_CALLBACK_TIMEOUT_SECONDS", 5),
	}
}

func loadEnvFile() {
	path := strings.TrimSpace(os.Getenv("AUTOMATION_ENV_FILE"))
	if path == "" {
		path = ".env"
	}
	data, err := os.ReadFile(path)
	if err != nil {
		return
	}
	for _, line := range strings.Split(string(data), "\n") {
		key, value, ok := parseEnvLine(line)
		if !ok {
			continue
		}
		if existing, exists := os.LookupEnv(key); exists && strings.TrimSpace(existing) != "" {
			continue
		}
		_ = os.Setenv(key, os.ExpandEnv(value))
	}
}

func parseEnvLine(line string) (string, string, bool) {
	line = strings.TrimSpace(line)
	if line == "" || strings.HasPrefix(line, "#") {
		return "", "", false
	}
	line = strings.TrimPrefix(line, "export ")
	key, value, found := strings.Cut(line, "=")
	if !found {
		return "", "", false
	}
	key = strings.TrimSpace(key)
	if key == "" || strings.ContainsAny(key, " \t") {
		return "", "", false
	}
	value = strings.TrimSpace(value)
	if len(value) >= 2 {
		first, last := value[0], value[len(value)-1]
		if first == '"' && last == '"' {
			if unquoted, err := strconv.Unquote(value); err == nil {
				value = unquoted
			}
		} else if first == '\'' && last == '\'' {
			value = strings.TrimSuffix(strings.TrimPrefix(value, "'"), "'")
		}
	}
	return key, value, true
}

func normalizeExecutor(value string) string {
	value = strings.ToLower(strings.TrimSpace(value))
	switch value {
	case "desktop_session", "browser_playwright":
		return value
	default:
		return "browser_playwright"
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
