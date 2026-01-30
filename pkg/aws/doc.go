// Package aws provides helper functions for creating AWS clients in apptest-framework tests.
//
// This package simplifies AWS API access for tests that need to verify AWS resources
// (e.g., Load Balancers, EBS volumes, etc.) created by applications under test.
//
// # Authentication
//
// When running in CI with IRSA (IAM Roles for Service Accounts) configured, the AWS SDK
// automatically discovers credentials from the projected service account token.
// No explicit credential configuration is required in test code.
//
// # Configuration
//
// To enable IRSA authentication, add AWS configuration to your test suite's config.yaml:
//
//	appName: my-aws-app
//	repoName: my-aws-app
//	appCatalog: giantswarm
//	providers:
//	- capa
//	aws:
//	  iamRoleARN: "arn:aws:iam::123456789012:role/e2e-test-role"
//	  region: "eu-west-1"
//
// # Usage Example
//
//	import (
//	    "context"
//	    awshelper "github.com/giantswarm/apptest-framework/v3/pkg/aws"
//	    "github.com/aws/aws-sdk-go-v2/service/ec2"
//	)
//
//	It("should verify ELB was created", func() {
//	    ctx := context.Background()
//	    cfg, err := awshelper.NewConfig(ctx, "eu-west-1")
//	    Expect(err).NotTo(HaveOccurred())
//
//	    ec2Client := ec2.NewFromConfig(cfg)
//	    // Use ec2Client to verify resources...
//	})
package aws
