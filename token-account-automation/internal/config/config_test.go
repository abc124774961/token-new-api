package config

import (
	"os"
	"path/filepath"
	"testing"
)

func TestLoadReadsAutomationEnvFile(t *testing.T) {
	cleanupEnv(t, []string{
		"TOKEN_ACCOUNT_AUTOMATION_API_TOKEN",
		"AUTOMATION_API_TOKEN",
		"AUTOMATION_WORKER_TOKEN",
		"AUTOMATION_DESKTOP_TOKEN",
		"AUTOMATION_LISTEN_ADDR",
		"AUTOMATION_BROWSER_LOGIN_EXECUTOR",
	})
	dir := t.TempDir()
	envPath := filepath.Join(dir, ".env.dev")
	err := os.WriteFile(envPath, []byte(`
TOKEN_ACCOUNT_AUTOMATION_API_TOKEN=api-from-file
AUTOMATION_API_TOKEN=${TOKEN_ACCOUNT_AUTOMATION_API_TOKEN}
AUTOMATION_WORKER_TOKEN='worker-from-file'
AUTOMATION_DESKTOP_TOKEN="desktop-from-file"
AUTOMATION_LISTEN_ADDR=:18091
AUTOMATION_BROWSER_LOGIN_EXECUTOR=desktop_session
`), 0o600)
	if err != nil {
		t.Fatalf("write env file: %v", err)
	}
	t.Setenv("AUTOMATION_ENV_FILE", envPath)
	t.Setenv("AUTOMATION_SECRET_KEY", "secret-from-process")

	cfg := Load()

	if cfg.ListenAddr != ":18091" {
		t.Fatalf("unexpected listen addr: %s", cfg.ListenAddr)
	}
	if cfg.APIToken != "api-from-file" || cfg.WorkerToken != "worker-from-file" || cfg.DesktopToken != "desktop-from-file" {
		t.Fatalf("unexpected tokens: api=%q worker=%q desktop=%q", cfg.APIToken, cfg.WorkerToken, cfg.DesktopToken)
	}
	if cfg.SecretKey != "secret-from-process" {
		t.Fatalf("process env should win, got %q", cfg.SecretKey)
	}
	if cfg.BrowserLoginExecutor != "desktop_session" {
		t.Fatalf("unexpected executor: %s", cfg.BrowserLoginExecutor)
	}
}

func cleanupEnv(t *testing.T, keys []string) {
	t.Helper()
	old := make(map[string]string, len(keys))
	present := make(map[string]bool, len(keys))
	for _, key := range keys {
		value, ok := os.LookupEnv(key)
		old[key] = value
		present[key] = ok
		_ = os.Unsetenv(key)
	}
	t.Cleanup(func() {
		for _, key := range keys {
			if present[key] {
				_ = os.Setenv(key, old[key])
			} else {
				_ = os.Unsetenv(key)
			}
		}
	})
}
