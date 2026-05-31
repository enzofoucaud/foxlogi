// Package config loads bot configuration from a .env file and the environment,
// using Viper so configuration can grow (files, flags, env) without touching
// call sites.
package config

import (
	"errors"
	"fmt"
	"os"
	"time"

	"github.com/spf13/viper"
)

// Config holds runtime configuration for the bot.
type Config struct {
	Token         string        // Discord bot token (required)
	DBPath        string        // SQLite database file path
	SoonThreshold time.Duration // crafts within this window are flagged "soon" in listings
}

// Load reads configuration with the following precedence (highest first):
// real environment variables, then a local .env file, then built-in defaults.
// It validates that the required Discord token is present.
func Load() (Config, error) {
	v := viper.New()

	v.SetDefault("CRAFT_DB_PATH", "foxlogi.db")
	v.SetDefault("SOON_THRESHOLD", "10m")

	// Load .env if it exists; a missing file is not an error. Real environment
	// variables (AutomaticEnv) take precedence over the file's values.
	v.SetConfigFile(".env")
	v.SetConfigType("env")
	if err := v.ReadInConfig(); err != nil && !errors.Is(err, os.ErrNotExist) {
		return Config{}, fmt.Errorf("read .env: %w", err)
	}
	v.AutomaticEnv()

	cfg := Config{
		Token:  v.GetString("DISCORD_TOKEN"),
		DBPath: v.GetString("CRAFT_DB_PATH"),
	}
	if cfg.Token == "" {
		return Config{}, fmt.Errorf("DISCORD_TOKEN is required")
	}

	soonStr := v.GetString("SOON_THRESHOLD")
	soon, err := time.ParseDuration(soonStr)
	if err != nil {
		return Config{}, fmt.Errorf("invalid SOON_THRESHOLD %q: %w", soonStr, err)
	}
	cfg.SoonThreshold = soon

	return cfg, nil
}
