package config

import (
	"os"
	"path/filepath"

	"sigs.k8s.io/yaml"
)

// AWSConfig provides AWS-specific configuration for tests that need to interact with AWS APIs
type AWSConfig struct {
	// IAMRoleARN is the ARN of the IAM role to assume via IRSA (IAM Roles for Service Accounts).
	// The test pod will use this role for AWS API authentication.
	// The role must have a trust policy that allows the OIDC provider of the cluster
	// where the test pod runs.
	IAMRoleARN string `json:"iamRoleARN,omitempty"`

	// Region is the default AWS region to use for API calls.
	// If not set, tests should specify the region when creating AWS clients.
	Region string `json:"region,omitempty"`
}

// TestConfig provides a standard configuration for Apps
type TestConfig struct {
	AppName    string   `json:"appName"`
	RepoName   string   `json:"repoName"`
	AppCatalog string   `json:"appCatalog"`
	Providers  []string `json:"providers"`
	IsMCTest   bool     `json:"isMCTest"`

	// AWS contains AWS-specific configuration for tests that need to interact with AWS APIs.
	// This enables IRSA-based authentication for the test pod.
	AWS *AWSConfig `json:"aws,omitempty"`
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

// HasAWSConfig returns true if AWS configuration is present with an IAM Role ARN
func (c *TestConfig) HasAWSConfig() bool {
	return c.AWS != nil && c.AWS.IAMRoleARN != ""
}

// GetAWSIAMRoleARN returns the configured IAM Role ARN, or empty string if not set
func (c *TestConfig) GetAWSIAMRoleARN() string {
	if c.AWS != nil {
		return c.AWS.IAMRoleARN
	}
	return ""
}

// GetAWSRegion returns the configured AWS region, or the provided default if not configured
func (c *TestConfig) GetAWSRegion(defaultRegion string) string {
	if c.AWS != nil && c.AWS.Region != "" {
		return c.AWS.Region
	}
	return defaultRegion
}
