package config

import (
	"flag"
	"fmt"
	"os"
	"path/filepath"
	"time"
)

// Config holds runtime configuration for the opencode-status binary.
type Config struct {
	DBPath         string        // SQLite database file path
	ModelsDevURL   string        // models.dev api.json endpoint
	OpenRouterURL  string        // OpenRouter /api/v1/models endpoint
	PollInterval   time.Duration // poll cadence (default 5m)
	RetentionDays  int           // days of history to keep (default 30)
	WebAddr        string        // HTTP listen address for /api endpoints (empty = no HTTP)
	WebDisable     bool          // disable HTTP server entirely
	TUIDisable     bool          // disable TUI (run as daemon + HTTP only)
	OpenRouterKey  string        // optional API key (improves live probe reliability)
	ShowPaid       bool          // also show paid models in the list
	LogJSON        bool          // log JSON to stdout instead of human
}

// FromArgs parses CLI flags + env vars into a Config.
func FromArgs(args []string) (*Config, error) {
	fs := flag.NewFlagSet("opencode-status", flag.ContinueOnError)

	c := &Config{
		DBPath:        getEnv("OPENCODE_STATUS_DB", "/var/lib/opencode-status/history.db"),
		ModelsDevURL:  getEnv("OPENCODE_STATUS_MODELS_DEV", "https://models.dev/api.json"),
		OpenRouterURL: getEnv("OPENCODE_STATUS_OPENROUTER", "https://openrouter.ai/api/v1/models"),
		PollInterval:  5 * time.Minute,
		RetentionDays: 30,
		WebAddr:       getEnv("OPENCODE_STATUS_WEB", ":8080"),
	}

	fs.StringVar(&c.DBPath, "db", c.DBPath, "SQLite database file path")
	fs.StringVar(&c.ModelsDevURL, "models-dev", c.ModelsDevURL, "models.dev api.json URL")
	fs.StringVar(&c.OpenRouterURL, "openrouter", c.OpenRouterURL, "OpenRouter /api/v1/models URL")
	fs.DurationVar(&c.PollInterval, "interval", c.PollInterval, "poll interval (e.g. 5m, 1h)")
	fs.IntVar(&c.RetentionDays, "retention-days", c.RetentionDays, "days of history to retain")
	fs.StringVar(&c.WebAddr, "web", c.WebAddr, "HTTP listen address (empty disables)")
	fs.BoolVar(&c.WebDisable, "no-web", false, "disable HTTP server")
	fs.BoolVar(&c.TUIDisable, "no-tui", false, "disable TUI (daemon only)")
	fs.StringVar(&c.OpenRouterKey, "openrouter-key", os.Getenv("OPENROUTER_API_KEY"), "OpenRouter API key (optional, improves probe reliability)")
	fs.BoolVar(&c.ShowPaid, "show-paid", false, "include paid models in the list")
	fs.BoolVar(&c.LogJSON, "log-json", false, "log JSON to stdout")

	if err := fs.Parse(args); err != nil {
		return nil, err
	}

	// Expand DB path.
	if dbp := os.Getenv("OPENCODE_STATUS_DB"); dbp != "" {
		c.DBPath = dbp
	}
	if !filepath.IsAbs(c.DBPath) && c.DBPath != ":memory:" {
		abs, err := filepath.Abs(c.DBPath)
		if err != nil {
			return nil, fmt.Errorf("resolve db path: %w", err)
		}
		c.DBPath = abs
	}

	return c, nil
}

func getEnv(k, def string) string {
	if v := os.Getenv(k); v != "" {
		return v
	}
	return def
}
