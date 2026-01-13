package main

import (
	"os"
	"path/filepath"
	"strings"

	"github.com/BurntSushi/toml"
)

type cliConfig struct {
	CurrentProfile string                `toml:"current_profile,omitempty"`
	Profiles       map[string]cliProfile `toml:"profiles,omitempty"`
}

type cliProfile struct {
	APIKey    string `toml:"api_key,omitempty"`
	BaseURL   string `toml:"base_url,omitempty"`
	ProjectID string `toml:"project_id,omitempty"`
	Output    string `toml:"output,omitempty"`
}

func loadCLIConfig() (cliConfig, error) {
	path, err := defaultConfigPath()
	if err != nil {
		return cliConfig{}, nil
	}
	return readCLIConfig(path)
}

func defaultConfigPath() (string, error) {
	dir, err := os.UserConfigDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(dir, "mrl", "config.toml"), nil
}

func readCLIConfig(path string) (cliConfig, error) {
	if strings.TrimSpace(path) == "" {
		return cliConfig{}, nil
	}
	data, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return cliConfig{}, nil
		}
		return cliConfig{}, err
	}
	if len(data) == 0 {
		return cliConfig{}, nil
	}
	var cfg cliConfig
	if _, err := toml.Decode(string(data), &cfg); err != nil {
		return cliConfig{}, err
	}
	if cfg.Profiles == nil {
		cfg.Profiles = map[string]cliProfile{}
	}
	return cfg, nil
}

func writeCLIConfig(cfg cliConfig) error {
	path, err := defaultConfigPath()
	if err != nil {
		return err
	}
	if cfg.Profiles == nil {
		cfg.Profiles = map[string]cliProfile{}
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return err
	}
	f, err := os.OpenFile(path, os.O_WRONLY|os.O_CREATE|os.O_TRUNC, 0o600)
	if err != nil {
		return err
	}
	defer f.Close()
	return toml.NewEncoder(f).Encode(cfg)
}

func resolveProfileName(flagValue string, cfg cliConfig) string {
	clean := strings.TrimSpace(flagValue)
	if clean != "" {
		return clean
	}
	clean = strings.TrimSpace(cfg.CurrentProfile)
	if clean != "" {
		return clean
	}
	return "default"
}

func profileFor(cfg cliConfig, name string) cliProfile {
	if cfg.Profiles == nil {
		return cliProfile{}
	}
	return cfg.Profiles[name]
}
