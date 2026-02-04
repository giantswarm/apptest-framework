package aws

import (
	"context"
	"fmt"
	"os"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/config"
)

// NewConfig creates an AWS config using the default credential chain.
//
// When running with IRSA (IAM Roles for Service Accounts), credentials are
// automatically provided via the projected service account token. The AWS SDK
// uses the AWS_ROLE_ARN and AWS_WEB_IDENTITY_TOKEN_FILE environment variables
// that are set by the Kubernetes pod identity webhook.
//
// The region parameter specifies the AWS region for API calls. If empty,
// the SDK will attempt to determine the region from environment variables
// or instance metadata.
func NewConfig(ctx context.Context, region string) (aws.Config, error) {
	opts := []func(*config.LoadOptions) error{}

	if region != "" {
		opts = append(opts, config.WithRegion(region))
	}

	cfg, err := config.LoadDefaultConfig(ctx, opts...)
	if err != nil {
		return aws.Config{}, fmt.Errorf("failed to load AWS config: %w", err)
	}

	return cfg, nil
}

// NewConfigWithRegion creates an AWS config with the specified region.
// This is a convenience wrapper around NewConfig that requires a region to be specified.
func NewConfigWithRegion(ctx context.Context, region string) (aws.Config, error) {
	if region == "" {
		return aws.Config{}, fmt.Errorf("region is required")
	}
	return NewConfig(ctx, region)
}

// IsIRSAConfigured returns true if the IRSA environment variables are set,
// indicating that the pod is configured to use IAM Roles for Service Accounts.
func IsIRSAConfigured() bool {
	roleARN := os.Getenv("AWS_ROLE_ARN")
	tokenFile := os.Getenv("AWS_WEB_IDENTITY_TOKEN_FILE")
	return roleARN != "" && tokenFile != ""
}

// GetIRSARoleARN returns the IAM Role ARN configured via IRSA environment variables.
// Returns an empty string if IRSA is not configured.
func GetIRSARoleARN() string {
	return os.Getenv("AWS_ROLE_ARN")
}

// MustNewConfig creates an AWS config and panics if an error occurs.
// This is useful in test setup where failure should immediately stop the test.
func MustNewConfig(ctx context.Context, region string) aws.Config {
	cfg, err := NewConfig(ctx, region)
	if err != nil {
		panic(fmt.Sprintf("failed to create AWS config: %v", err))
	}
	return cfg
}
