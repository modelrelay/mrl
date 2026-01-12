package main

import (
	"os"
	"path/filepath"
	"strings"

	"gopkg.in/yaml.v3"
)

type cliConfig struct {
	CurrentProfile string                `yaml:"current_profile,omitempty"`
	Profiles       map[string]cliProfile `yaml:"profiles,omitempty"`
}

type cliProfile struct {
	APIKey    string `yaml:"api_key,omitempty"`
	Token     string `yaml:"token,omitempty"`
	BaseURL   string `yaml:"base_url,omitempty"`
	ProjectID string `yaml:"project_id,omitempty"`
	Output    string `yaml:"output,omitempty"`
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
	return filepath.Join(dir, "modelrelay", "config.yaml"), nil
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
	if err := yaml.Unmarshal(data, &cfg); err != nil {
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
	data, err := yaml.Marshal(cfg)
	if err != nil {
		return err
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return err
	}
	return os.WriteFile(path, data, 0o600)
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
