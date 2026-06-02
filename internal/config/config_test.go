package config

import (
	"os"
	"path/filepath"
	"testing"
	"time"
)

// writeDotenv creates a .env in a temp dir and switches into it for the test.
func writeDotenv(t *testing.T, content string) {
	t.Helper()
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, ".env"), []byte(content), 0o600); err != nil {
		t.Fatalf("write .env: %v", err)
	}
	old, err := os.Getwd()
	if err != nil {
		t.Fatalf("getwd: %v", err)
	}
	if err := os.Chdir(dir); err != nil {
		t.Fatalf("chdir: %v", err)
	}
	t.Cleanup(func() { os.Chdir(old) })
}

func TestLoadReadsDotenv(t *testing.T) {
	os.Unsetenv("DISCORD_TOKEN")
	os.Unsetenv("SOON_THRESHOLD")
	os.Unsetenv("CRAFT_DB_PATH")
	writeDotenv(t, "DISCORD_TOKEN=tok-from-file\nSOON_THRESHOLD=2m\n")

	cfg, err := Load()
	if err != nil {
		t.Fatalf("load: %v", err)
	}
	if cfg.Token != "tok-from-file" {
		t.Errorf("token: want tok-from-file, got %q", cfg.Token)
	}
	if cfg.SoonThreshold != 2*time.Minute {
		t.Errorf("soon threshold: want 2m, got %v", cfg.SoonThreshold)
	}
	if cfg.DBPath != "foxlogi.db" {
		t.Errorf("dbpath default: got %q", cfg.DBPath)
	}
}

func TestLogLevelDefaultAndOverride(t *testing.T) {
	os.Unsetenv("LOG_LEVEL")
	writeDotenv(t, "DISCORD_TOKEN=tok\n")
	cfg, err := Load()
	if err != nil {
		t.Fatalf("load: %v", err)
	}
	if cfg.LogLevel != "info" {
		t.Errorf("default log level: want info, got %q", cfg.LogLevel)
	}

	t.Setenv("LOG_LEVEL", "DEBUG")
	cfg, err = Load()
	if err != nil {
		t.Fatalf("load: %v", err)
	}
	if cfg.LogLevel != "debug" {
		t.Errorf("log level should be lowercased: got %q", cfg.LogLevel)
	}
}

func TestEnvOverridesDotenv(t *testing.T) {
	writeDotenv(t, "DISCORD_TOKEN=tok-from-file\n")
	t.Setenv("DISCORD_TOKEN", "tok-from-env")

	cfg, err := Load()
	if err != nil {
		t.Fatalf("load: %v", err)
	}
	if cfg.Token != "tok-from-env" {
		t.Errorf("real env should override .env: got %q", cfg.Token)
	}
}

func TestMissingTokenIsError(t *testing.T) {
	os.Unsetenv("DISCORD_TOKEN")
	writeDotenv(t, "SOON_THRESHOLD=5m\n")

	if _, err := Load(); err == nil {
		t.Fatal("expected an error when DISCORD_TOKEN is absent")
	}
}
