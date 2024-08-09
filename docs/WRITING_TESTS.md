# Writing Tests

- [Writing Tests](#writing-tests)
  - [API Documentation](#api-documentation)
  - [Adding New Test Suites](#adding-new-test-suites)
  - [Adding New Test Cases](#adding-new-test-cases)
  - [Upgrade Tests](#upgrade-tests)
  - [Testing App Bundles](#testing-app-bundles)
  - [Testing Default Apps](#testing-default-apps)
  - [Related Resources](#related-resources)

## API Documentation

API documentation for this framework can be found at: [pkg.go.dev/github.com/giantswarm/apptest-framework](https://pkg.go.dev/github.com/giantswarm/apptest-framework).

This test framework also make use of [clustertest](https://github.com/giantswarm/clustertest) for a lot of the functionality when interacting with clusters. The API documentation for this library can be found at: [https://godoc.org/github.com/giantswarm/clustertest](https://godoc.org/github.com/giantswarm/clustertest).

## Adding New Test Suites

If you need to test different configured functionality of your App (e.g. a different set of values provided when installing) you can create a new test suite for each of these variations. Each test suite should be run in isolation in its own test workload cluster so it doesn't interfere with other tests.

To add a new test suite, create a new directory under `./tests/e2e/suites/` with the name of your new test suite and follow the same layout as the `basic` test suite.

E.g.

```plain
📂 tests/e2e
├── 📂 suites
│  ├── 📂 basic
│  │  ├── 📄 basic_suite_test.go
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

1. `AfterClusterReady` - These are run first, as soon as the workload cluster is deemed to be ready, and should be used to check for any needed pre-requisites in the cluster. This is optional and only need to be provided if you require some logic to run as soon as the cluster is stable.
1. `BeforeUpgrade` - These are only run if performing an upgrade tests and are run between installing the latest released version of your App and the version being tested. These are used to test that the App is in an expected state before performing the upgrade.
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
suite.New(appConfig).
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
suite.New(config.MustLoad("../../config.yaml")).
  InAppBundle("auth-bundle").
  WithInstallNamespace("kube-system").
  ...etc...
```

When testing within a bundle, the test framework will use the latest released version of the bundle App and override the version of the App being tested within the bundle values.

If the bundle App is also a default app please make sure to also read the [Testing Default Apps](#testing-default-apps) section below.

> [!TIP]
> Example: [tests/e2e/suites/defaultbundleapp](https://github.com/giantswarm/apptest-framework/blob/534f57426d183921e042e09cf6694ac2756d3862/tests/e2e/suites/defaultbundleapp/defaultbundleapp_suite_test.go)

## Testing Default Apps

> [!NOTE]
> Testing of default apps is only supported with providers that have been updated to make use of Releases.

Nothing special needs to be done to a test suite to make it compatible with a default app, instead the framework will detect at runtime if the app being tested is a default App or not by checking it against the Release spec for that provider.

One thing to be aware of when testing default apps is it's not possible to perform any actions before the installation of the App as it is done as part of the cluster creation.

If testing an app within a bundle App that is a default App, the framework will detect that bundle App from the Release and patch it to make sure the App being tested is installed as a child App during the cluster installation phase.

> [!TIP]
> Example: [tests/e2e/suites/defaultapp](https://github.com/giantswarm/apptest-framework/blob/534f57426d183921e042e09cf6694ac2756d3862/tests/e2e/suites/defaultapp/defaultapp_suite_test.go)

## Related Resources

- [Ginkgo docs](https://onsi.github.io/ginkgo/)
- [Gomega docs](https://onsi.github.io/gomega/)
- [`clustertest` documentation](https://pkg.go.dev/github.com/giantswarm/clustertest)
