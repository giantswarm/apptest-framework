# apptest-framework

<a href="https://godoc.org/github.com/giantswarm/apptest-framework"><img src="https://godoc.org/github.com/giantswarm/apptest-framework?status.svg"></a>

A test framework for helping with E2E testing of Giant Swarm managed Apps within Giant Swarm clusters.

## Features

- Handles test suite setup (using Ginkgo)
- Provides shared state across test cases
- Provides hooks for pre-install, pre-upgrade and post-install steps

## Installation

> [!NOTE]
> The following information is specific to Giant Swarm repos

To ease the setup and bootstrapping of apptests there are two `devctl` commands that setup everything for you.

First, your App repo needs to have the webhooks configured to send to our CI/CD cluster so we can trigger runs from PRs:

```shell
REPO_NAME="example-app" # Change this with your repo name
SHARED_SECRET=""        # This needs to be provided. See the `--help` output for the `ci-webhooks` command for more details

devctl repo setup ci-webhooks --webhook-secret ${SHARED_SECRET} giantswarm/${REPO_NAME}
```

Second, you can scaffold the needed files in the correct place within your repo. Be sure to set the catalog and the app name your app uses within that catalog.

From the root of your App repo:

```shell
devctl gen apptest --repo-name ${REPO_NAME} --app-name ${APP_NAME} --catalog ${CATALOG}
```

Once done your repo should now contain the following:

```plain
📂 tests/e2e
├── 📂 suites
│  └── 📂 basic
│     ├── 📄 basic_suite_test.go
│     └── 📄 values.yaml
├── 📄 config.yaml
├── 📄 go.mod
└── 📄 go.sum
```

This contains a single test suite called `basic` that without any changes will install your App into the cluster and test for it being marked as "Deployed" successfully.

You can now add your test cases and additional test suites if needed.

## Required Environment Variables

- `E2E_KUBECONFIG` must be set to the kubeconfig of the test management cluster
- `E2E_KUBECONFIG_CONTEXT` must be set to the context to use in the kubeconfig
- `E2E_WC_NAME` must be set to the name of the test workload cluster
- `E2E_WC_NAMESPACE` must be set to namespace the test workload cluster is in within the management cluster
- `E2E_APP_VERSION` must be set to version of the app to test against

## API Documentation

API documentation can be found at: [pkg.go.dev/github.com/giantswarm/apptest-framework](https://pkg.go.dev/github.com/giantswarm/apptest-framework).

## Adding Tests

### Test Cases

Once bootstrapped your repo will have a test suite called `basic` that you can start adding tests to.

There are 3 phases in which you can add tests:

- `BeforeInstall` - These are run first, before anything else is done, and should be used to check for any needed pre-requisites in the cluster.
- `BeforeUpgrade` - These are only run if performing an upgrade tests and are run between installing the latest released version of your App and the version being tested. These are used to test that the App is in an expected state before performing the upgrade.
- `Tests` - This is where most of your tests will go and will be run after your App has been installed and marked as "Deployed" in the cluster.

To add new test cases you can either add them inline within the above functions or call out to other functions and modules without your codebase so you can better structure different tests together. Be sure to follow the Ginkgo docs on writing [Spec Subjects](https://onsi.github.io/ginkgo/#spec-subjects-it).

### Test Suites

If you need to test different configured functionality of your App (e.g. a different set of values provided when installing) you can create a new test suite for each of these variations. Each test suite should be run in isolation in its own test workload cluster so it doesn't interfere with other tests.

To add a new test suite, create a new directory under `./tests/e2e/suites/` with the name of your new test suite and follow the same layout as the `basic` test suite.

## Resources

- [Ginkgo docs](https://onsi.github.io/ginkgo/)
- [`clustertest` documentation](https://pkg.go.dev/github.com/giantswarm/clustertest)
- [CI Tekton Pipeline](https://github.com/giantswarm/tekton-resources/blob/main/tekton-resources/pipelines/app-test-suites.yaml)
