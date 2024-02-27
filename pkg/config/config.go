package config

import (
	"os"
	"path/filepath"

	"sigs.k8s.io/yaml"
)

// TestConfig provides a standard configuration for Apps
type TestConfig struct {
	AppName    string `json:"appName"`
	RepoName   string `json:"repoName"`
	AppCatalog string `json:"appCatalog"`
}

// MustLoad opens the given yaml file and parses it into a TestConfig instance
// Any errors while opening are silently ignored.
func MustLoad(configPath string) TestConfig {
	configPath, _ = filepath.Abs(configPath)
	config := TestConfig{}
	yamlFile, _ := os.ReadFile(configPath)
	yaml.Unmarshal(yamlFile, &config)
	return config
}
