package main

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"time"
)

// Config holds all tunable parameters for the application.
// All values are loaded from data/config.json at startup.
type Config struct {
	ScoreThreshold float64 `json:"score_threshold"` // Minimum score to accept a strategy
	FailThreshold  int     `json:"fail_threshold"`  // Failures before triggering recovery
	CheckInterval  int     `json:"check_interval"`  // Seconds between watchdog checks
	InitDelay      int     `json:"init_delay"`      // Delay before first watchdog check
	TestTimeout    int     `json:"test_timeout"`    // Seconds per strategy test
	TestRuns       int     `json:"test_runs"`       // Test repetitions for reliability
}

// defaultConfig contains the default configuration values used when config.json is absent
var defaultConfig = Config{
	ScoreThreshold: 0.6,
	FailThreshold:  3,
	CheckInterval:  60,
	InitDelay:      5,
	TestTimeout:    8,
	TestRuns:       2,
}

// Cfg is the active configuration, loaded once at startup and modified via HTTP API
var Cfg = defaultConfig

// configPath returns absolute path to data/config.json
func configPath() string {
	cwd, err := os.Getwd()
	if err != nil {
		cwd = "."
	}
	return filepath.Join(cwd, "data", "config.json")
}

// LoadConfig reads data/config.json and merges into Cfg.
// If file doesn't exist, creates it with defaults.
func LoadConfig() error {
	path := configPath()

	data, err := os.ReadFile(path)
	if os.IsNotExist(err) {
		// Create default config
		if err := os.MkdirAll(filepath.Dir(path), 0755); err != nil {
			return fmt.Errorf("create config dir: %w", err)
		}

		defaultData, err := json.MarshalIndent(defaultConfig, "", "  ")
		if err != nil {
			return fmt.Errorf("marshal default config: %w", err)
		}

		if err := os.WriteFile(path, defaultData, 0644); err != nil {
			return fmt.Errorf("write default config: %w", err)
		}
		Cfg = defaultConfig
		logInfo("[config] created default config at %s", path)
		return nil
	}
	if err != nil {
		return fmt.Errorf("read config: %w", err)
	}

	loaded := defaultConfig // start from defaults so missing fields stay as default
	if err := json.Unmarshal(data, &loaded); err != nil {
		return fmt.Errorf("parse config: %w", err)
	}

	Cfg = loaded
	logInfo("[config] loaded from %s", path)
	return nil
}

// CheckIntervalDuration returns the check interval as a time.Duration
func (c Config) CheckIntervalDuration() time.Duration { return secs(c.CheckInterval) }

// InitDelayDuration returns the init delay as a time.Duration
func (c Config) InitDelayDuration() time.Duration { return secs(c.InitDelay) }

// TestTimeoutDuration returns the test timeout as a time.Duration
func (c Config) TestTimeoutDuration() time.Duration { return secs(c.TestTimeout) }

// secs converts an integer to a time.Duration in seconds
func secs(n int) time.Duration { return time.Duration(n) * time.Second }

// SaveConfig writes the given config to data/config.json atomically.
func SaveConfig(cfg Config) error {
	path := configPath()
	tmpPath := path + ".tmp"

	data, err := json.MarshalIndent(cfg, "", "  ")
	if err != nil {
		return fmt.Errorf("marshal config: %w", err)
	}

	if err := os.WriteFile(tmpPath, data, 0644); err != nil {
		return fmt.Errorf("write temp config: %w", err)
	}

	if err := os.Rename(tmpPath, path); err != nil {
		os.Remove(tmpPath)
		return fmt.Errorf("rename temp config: %w", err)
	}

	return nil
}
