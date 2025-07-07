package config

import (
	"os"
	"path/filepath"

	"sigs.k8s.io/yaml"
)

// TestConfig provides a standard configuration for Apps
type TestConfig struct {
	AppName    string   `json:"appName"`
	RepoName   string   `json:"repoName"`
	AppCatalog string   `json:"appCatalog"`
	Providers  []string `json:"providers"`
	IsMCTest   bool     `json:"isMCTest"`
}

// MustLoad opens the given yaml file and parses it into a TestConfig instance
// Any errors while opening are silently ignored.
func MustLoad() TestConfig {
	config := TestConfig{}

	ex, _ := os.Executable()
	exDir := filepath.Dir(ex)
	configPath, _ := filepath.Abs(filepath.Join(exDir, "config.yaml"))
	if _, err := os.Stat(configPath); err != nil {
		configPath, _ = filepath.Abs(filepath.Join(exDir, "../", "../", "config.yaml"))
	}
	yamlFile, _ := os.ReadFile(configPath) // #nosec G304
	_ = yaml.Unmarshal(yamlFile, &config)

	return config
}
