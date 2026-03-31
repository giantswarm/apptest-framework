# Writing Tests

- [Writing Tests](#writing-tests)
  - [API Documentation](#api-documentation)
  - [Adding New Test Suites](#adding-new-test-suites)
  - [Adding New Test Cases](#adding-new-test-cases)
  - [Upgrade Tests](#upgrade-tests)
  - [Testing App Bundles](#testing-app-bundles)
  - [Testing Default Apps](#testing-default-apps)
  - [Testing with HelmRelease CRs](#testing-with-helmrelease-crs)
  - [Testing with AWS API Access](#testing-with-aws-api-access)
  - [Related Resources](#related-resources)

## API Documentation

API documentation for this framework can be found at: [pkg.go.dev/github.com/giantswarm/apptest-framework](https://pkg.go.dev/github.com/giantswarm/apptest-framework).

This test framework also make use of [clustertest](https://github.com/giantswarm/clustertest) for a lot of the functionality when interacting with clusters. The API documentation for this library can be found at: [https://godoc.org/github.com/giantswarm/clustertest](https://godoc.org/github.com/giantswarm/clustertest).

## Test Config

This framework expects, and relies on, some config to be supplied for the test suites that indicate the App being tested and the environment to test in.
The definition of the config can be found in `./pkg/config/config.go` but an annotated example is provided below:

```yaml
# appName: The name of the app as it will be installed
appName: "hello-world"
# reponame: The name of the Apps repository on GitHub (this may be the same as `appName`)
repoName: "hello-world-app"
# appCatalog: The (production) catalog that the App gets published to. This will also be used to determine the dev catalog.
appCatalog: "giantswarm"

# providers: A list of CAPI providers to run the tests against when triggered from a PR.
# This defaults to just `capa` if not supplied.
providers:
- capa

# isMCTest: A boolean indicating whether this test should run on the management cluster rather than creating a workload cluster for the tests.
# This defaults to false
isMCTest: true

# aws: (Optional) AWS-specific configuration for tests that need to interact with AWS APIs.
# See "Testing with AWS API Access" section for more details.
aws:
  # iamRoleARN: The IAM Role ARN to assume for AWS API access
  iamRoleARN: "arn:aws:iam::123456789012:role/e2e-test-readonly"
  # region: Default AWS region for API calls
  region: "eu-west-1"
```

There are two locations that the `config.yaml` can be found:

1. Alongside the test suite itself, e.g. `./tests/e2e/suites/basic/config.yaml`. If found this takes priority.
2. In the e2e directory, e.g. `./tests/e2e/config.yaml`. This applies to all test suites that don't include a dedicated configuration file.

## Adding New Test Suites

If you need to test different configured functionality of your App (e.g. a different set of values provided when installing) you can create a new test suite for each of these variations. Each test suite should be run in isolation in its own test workload cluster so it doesn't interfere with other tests.

To add a new test suite, create a new directory under `./tests/e2e/suites/` with the name of your new test suite and follow the same layout as the `basic` test suite.

E.g.

```plain
📂 tests/e2e
├── 📂 suites
│  ├── 📂 basic
│  │  ├── 📄 basic_suite_test.go
│  │  ├── 📄 config.yaml
│  │  └── 📄 values.yaml
│  └── 📂 variation
│     ├── 📄 variation_suite_test.go
│     └── 📄 values.yaml
├── 📄 config.yaml
├── 📄 go.mod
└── 📄 go.sum
```

Be sure to update the test name use within the `*_suite_test.go` so that it correctly reports the test suite when run with Ginkgo. (E.g. update the `Run(t, "Basic Test")` line)

> [!TIP]
> Example: [ingress-nginx-app - tests/e2e/suites/auth-bundle](https://github.com/giantswarm/ingress-nginx-app/tree/d3269ccf2e5d3cc044f9a4ea7c291c84806be75c/tests/e2e/suites/auth-bundle)

## Adding New Test Cases

Once [bootstrapped](https://github.com/giantswarm/apptest-framework#installation) your repo will have a test suite called `basic` that you can start adding tests to.

There are 4 phases in which you can add tests:

1. `AfterClusterReady` - These are run first, as soon as the workload cluster is deemed to be ready, and should be used to check for any needed pre-requisites in the cluster. This is optional and only need to be provided if you require some logic to run as soon as the cluster is stable. Note: Does not run for tests of default apps.
1. `BeforeUpgrade` - These are only run if performing an upgrade tests and are run between installing the latest released version of your App and the version being tested. These are used to test that the App is in an expected state before performing the upgrade. Note: Does not run for tests of default apps.
1. `Tests` - This is where most of your tests will go and will be run after your App has been installed and marked as "Deployed" in the cluster. This is the minimum that needs to be provided.
1. `AfterSuite` - This is performed during the cleanup after the tests have completed. This function will be triggered before the test App is uninstalled and before the workload cluster is deleted. This is optional and allows for any extra cleanup that might be required.

To add new test cases you can either add them inline within the above functions or call out to other functions and modules without your codebase so you can better structure different tests together. Be sure to follow the Ginkgo docs on writing [Spec Subjects](https://onsi.github.io/ginkgo/#spec-subjects-it) and the Gomega docs on [making assertions](https://onsi.github.io/gomega/#making-assertions).

> [!TIP]
> Example specifying inline tests: [tests/e2e/suites/basic/basic_suite_test.go](https://github.com/giantswarm/apptest-framework/blob/534f57426d183921e042e09cf6694ac2756d3862/tests/e2e/suites/basic/basic_suite_test.go#L80-L100)

## Upgrade Tests

To perform an upgrade test you must first [create a new test suite](#adding-new-test-suites) that will handle the upgrade scenario.

In this new test case you then need to call `WithIsUpgrade(true)` on the suite.

E.g.

```go
suite.New().
  WithInstallNamespace(installNamespace).
  WithIsUpgrade(true).
  WithValuesFile("./values.yaml").
  BeforeUpgrade(func() {

    // Pre-upgrade checks

  }).
  Tests(func() {

    // Post-upgrade checks

  })
```

When running an upgrade test the framework will first install the latest released versions of the App (based on GitHub releases) into the workload cluster. The framework will then run any provided `BeforeUpgrade` logic before it then installs the dev version of the App (taken from the `E2E_APP_VERSION` env var). Following the installation of the dev version the framework will then move on to running the provided `Tests` logic.

> [!IMPORTANT]
> We currently don't have an example of this! 😱
>
> If you write an upgrade test suite for your App then please update this documentation with a link to it as an example! 💙

## Testing App Bundles

> [!WARNING]
> Due to inconsistencies and no standard to how we build bundles it is possible that some bundle apps aren't compatible with this test framework.
>
> If you discover a bundle that is not supported please raise an issue for it.

If the App you're wanting to test is deployed as part of a bundle then you need to indicate this to the test framework via the `InAppBundle` function.

E.g.

```go
suite.New().
  InAppBundle("auth-bundle").
  WithInstallNamespace("kube-system").
  ...etc...
```

When testing within a bundle, the test framework will use the latest released version of the bundle App and override the version of the App being tested within the bundle values.

If the bundle App is also a default app please make sure to also read the [Testing Default Apps](#testing-default-apps) section below.

> [!TIP]
> Example: [tests/e2e/suites/defaultbundleapp](https://github.com/giantswarm/apptest-framework/blob/534f57426d183921e042e09cf6694ac2756d3862/tests/e2e/suites/defaultbundleapp/defaultbundleapp_suite_test.go)

## Testing Default Apps

Nothing special needs to be done to a test suite to make it compatible with a default app, instead the framework will detect at runtime if the app being tested is a default App or not by checking it against the Release spec for that provider.

One thing to be aware of when testing default apps is it's not possible to perform any actions before the installation of the App as it is done as part of the cluster creation.

If testing an app within a bundle App that is a default App, the framework will detect that bundle App from the Release and patch it to make sure the App being tested is installed as a child App during the cluster installation phase.

> [!TIP]
> Example: [tests/e2e/suites/defaultapp](https://github.com/giantswarm/apptest-framework/blob/534f57426d183921e042e09cf6694ac2756d3862/tests/e2e/suites/defaultapp/defaultapp_suite_test.go)

## Testing Apps on Management Clusters

Aside from setting `isMCTest: true` in the config.yaml for your test suite (See [Test Config](#test-config) above) there is nothing else special needed when writing tests for management clusters.
The only other thing to be aware of is that the tests MUST be performed against an Ephemeral MC and will fail if attempted to run against any other (to protect that MC).

> [!TIP]
> Example: [tests/e2e/suites/mcAppTest](https://github.com/giantswarm/apptest-framework/blob/0d6ce8d985465957a3167f234448326149c12b3f/tests/e2e/suites/mcAppTest/mc_app_suite_test.go)

## Testing with HelmRelease CRs

By default, the framework installs Apps using Giant Swarm's `App` CR. If your chart is managed by Flux and you want to test it via a `HelmRelease` CR instead, you can enable HelmRelease mode.

The framework supports both source kinds used by Flux HelmReleases:
- **`OCIRepository`** (default) — used by most Giant Swarm apps deployed via Flux (e.g., `observability-operator`)
- **`HelmRepository`** — for charts sourced from traditional Helm repositories

For Giant Swarm apps on `gsoci.azurecr.io`, the framework defaults the source URL automatically — no `WithHelmSourceURL` needed in most cases.

### Configuration

The minimal setup for a standard GS app:

```go
// OCIRepository (default) — framework creates oci://gsoci.azurecr.io/charts/giantswarm/{appName}
suite.New().
  WithHelmRelease(true).
  WithHelmSourceName("observability-operator").
  WithHelmSourceNamespace("giantswarm").
  WithInstallNamespace("giantswarm").
  WithHelmTargetNamespace("monitoring").
  WithHelmStorageNamespace("monitoring").
  WithHelmReleaseName("observability-operator").
  WithValuesFile("./values.yaml").
  Tests(func() {
    It("has the expected resources", func() {
      // your test assertions
    })
  }).
  Run(t, "HelmRelease Test")
```

Use `WithHelmSourceURL` only if the chart lives outside `gsoci.azurecr.io/charts/giantswarm`, or `WithHelmChartName` when the chart name in the registry differs from the app install name.

### Available Builder Methods

| Method | Description |
| --- | --- |
| `WithHelmRelease(bool)` | Enables HelmRelease mode. When `true`, creates a Flux `HelmRelease` CR instead of an `App` CR. |
| `WithHelmSourceKind(SourceKind)` | Sets the source kind: `client.SourceKindOCIRepository` (default) or `client.SourceKindHelmRepository`. |
| `WithHelmSourceURL(string)` | URL of the source CR. Defaults to `oci://gsoci.azurecr.io/charts/giantswarm` (HelmRepository) or `oci://gsoci.azurecr.io/charts/giantswarm/{chartName}` (OCIRepository). Set this only for non-GS registries. |
| `WithHelmChartName(string)` | Name of the chart in the source registry. Defaults to `appName`. Set this when the chart name differs from the app install name. |
| `WithHelmSourceName(string)` | Name of the source CR to create/reference. Defaults to `appName`. |
| `WithHelmSourceNamespace(string)` | Namespace of the source CR. Defaults to the HelmRelease namespace. |
| `WithHelmTargetNamespace(string)` | Namespace where the Helm chart will be installed (`spec.targetNamespace`). |
| `WithHelmStorageNamespace(string)` | Namespace for Helm storage (`spec.storageNamespace`). |
| `WithHelmReleaseName(string)` | Helm release name (`spec.releaseName`). Defaults to the HelmRelease resource name. |
| `WithHelmTimeout(time.Duration)` | Timeout for Helm operations. Defaults to 10 minutes. |
| `WithHelmRetries(int)` | Number of retries for install/upgrade remediation. Defaults to 10. |
| `WithHelmServiceAccountName(string)` | Service account to impersonate when reconciling. Defaults to `appName`; auto-created if missing. |
| `WithHelmKubeConfigSecretName(string)` | Kubeconfig secret for remote cluster access. Defaults to `{clusterName}-kubeconfig` for workload cluster tests. |

### How It Works

When HelmRelease mode is enabled, the framework will:

1. Auto-configure defaults for workload cluster tests (namespace → cluster org namespace, kubeconfig secret → `{clusterName}-kubeconfig`).
2. Create the source CR (`HelmRepository` or `OCIRepository`), defaulting to the GS OCI registry.
3. Ensure required namespaces exist, creating them if needed.
4. Ensure the service account exists, creating it if needed.
5. Create a `Secret` containing chart values if a values file is provided.
6. Create the `HelmRelease` CR referencing the source.
7. Wait for the HelmRelease `Ready` condition to become `True`.
8. Run your test cases.
9. Delete the `HelmRelease`, values `Secret`, and source CR during cleanup.

### Upgrade Tests with HelmRelease

Upgrade tests work for both source kinds. The framework installs the latest released version first, then upgrades:

```go
suite.New().
  WithHelmRelease(true).
  WithHelmTargetNamespace("strimzi-system").
  WithIsUpgrade(true).
  BeforeUpgrade(func() {
    // checks before upgrading
  }).
  Tests(func() {
    // checks after upgrading
  }).
  Run(t, "HelmRelease Upgrade Test")
```

For `HelmRepository` sources the framework patches `spec.chart.spec.version` on the HelmRelease. For `OCIRepository` sources it patches `spec.ref.tag` on the OCIRepository.

### Client Helper Functions

The `pkg/client` package provides helper functions for working with HelmRelease CRs directly in your tests:

| Function | Description |
| --- | --- |
| `client.IsHelmReleaseReady(ctx, name, namespace)` | Checks if a HelmRelease has `Ready=True` |
| `client.IsHelmReleaseVersion(ctx, name, namespace, version)` | Checks the chart version on a HelmRelease |

### Accessing State

The HelmRelease is stored in the shared state and can be accessed within your tests:

```go
import "github.com/giantswarm/apptest-framework/v3/pkg/state"

hr := state.GetHelmRelease()
```

> [!IMPORTANT]
> Giant Swarm MCs enforce a `flux-multi-tenancy` Kyverno policy that requires:
> 1. `serviceAccountName` must be set on HelmReleases — use `WithHelmServiceAccountName()`
> 2. `targetNamespace` must match `metadata.namespace` unless `kubeConfig` is set — make sure `WithInstallNamespace()` and `WithHelmTargetNamespace()` use the same namespace, or omit `WithHelmTargetNamespace()` entirely

> [!NOTE]
> HelmRelease mode cannot be combined with App Bundle mode (`InAppBundle`). If you need to test a chart within a bundle, use the standard App CR mode.

## Testing with AWS API Access

Some tests may need to interact with AWS APIs to verify that resources were created correctly (e.g., Load Balancers, EBS volumes, Route53 records). This framework supports AWS authentication via IRSA (IAM Roles for Service Accounts).

### Configuration

To enable AWS API access, add the `aws` configuration block to your test suite's `config.yaml`:

```yaml
appName: my-aws-app
repoName: my-aws-app
appCatalog: giantswarm
providers:
- capa
aws:
  # The IAM Role ARN to assume for AWS API access
  iamRoleARN: "arn:aws:iam::123456789012:role/e2e-test-readonly"
  # Default AWS region for API calls
  region: "eu-west-1"
```

### IAM Role Prerequisites

The IAM Role must have:

1. **Trust policy** - Allows the OIDC provider of the cluster where the test pod runs:

```json
{
  "Version": "2012-10-17",
  "Statement": [
    {
      "Effect": "Allow",
      "Principal": {
        "Federated": "arn:aws:iam::ACCOUNT_ID:oidc-provider/OIDC_PROVIDER_URL"
      },
      "Action": "sts:AssumeRoleWithWebIdentity",
      "Condition": {
        "StringEquals": {
          "OIDC_PROVIDER_URL:aud": "sts.amazonaws.com"
        }
      }
    }
  ]
}
```

2. **Permissions** - The necessary permissions for the AWS APIs your tests need to call (e.g., `elasticloadbalancing:DescribeLoadBalancers`, `ec2:DescribeVolumes`).

### Using AWS APIs in Tests

The framework provides helper functions in the `pkg/aws` package to simplify creating AWS clients:

```go
import (
    "context"

    . "github.com/onsi/ginkgo/v2"
    . "github.com/onsi/gomega"

    awshelper "github.com/giantswarm/apptest-framework/v3/pkg/aws"
    "github.com/aws/aws-sdk-go-v2/service/elasticloadbalancingv2"
)

var _ = Describe("AWS Resource Tests", func() {
    It("should create a Load Balancer", func() {
        ctx := context.Background()

        // Create AWS config - credentials are automatically provided via IRSA
        cfg, err := awshelper.NewConfig(ctx, "eu-west-1")
        Expect(err).NotTo(HaveOccurred())

        // Create an ELB client
        elbClient := elasticloadbalancingv2.NewFromConfig(cfg)

        // Use the client to verify resources
        result, err := elbClient.DescribeLoadBalancers(ctx, &elasticloadbalancingv2.DescribeLoadBalancersInput{})
        Expect(err).NotTo(HaveOccurred())
        Expect(result.LoadBalancers).NotTo(BeEmpty())
    })
})
```

### Helper Functions

The `pkg/aws` package provides these helper functions:

| Function | Description |
| --- | --- |
| `NewConfig(ctx, region)` | Creates an AWS config using the default credential chain |
| `NewConfigWithRegion(ctx, region)` | Creates an AWS config, requiring a region to be specified |
| `MustNewConfig(ctx, region)` | Like `NewConfig` but panics on error (useful in test setup) |
| `IsIRSAConfigured()` | Returns true if IRSA environment variables are set |
| `GetIRSARoleARN()` | Returns the IAM Role ARN configured via IRSA |

### Running Locally

When running tests locally (not in CI), you can authenticate using any method supported by the AWS SDK's default credential chain:

- Environment variables (`AWS_ACCESS_KEY_ID`, `AWS_SECRET_ACCESS_KEY`)
- Shared credentials file (`~/.aws/credentials`)
- IAM role for EC2 instances
- SSO credentials

> [!NOTE]
> AWS API access is only available when running in CI with IRSA configured, or locally with valid AWS credentials. Tests that require AWS access should check `awshelper.IsIRSAConfigured()` or handle credential errors gracefully if AWS access is optional.

## Related Resources

- [Ginkgo docs](https://onsi.github.io/ginkgo/)
- [Gomega docs](https://onsi.github.io/gomega/)
- [`clustertest` documentation](https://pkg.go.dev/github.com/giantswarm/clustertest)
