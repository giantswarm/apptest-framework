# apptest-framework

<a href="https://godoc.org/github.com/giantswarm/apptest-framework"><img src="https://godoc.org/github.com/giantswarm/apptest-framework?status.svg"></a>

A test framework for helping with E2E testing of Giant Swarm managed Apps within Giant Swarm clusters.

## Features

- Handles test suite setup (using Ginkgo)
- Handles workload cluster creation and deletion
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

## Config.yaml

This framework relies on a `./tests/e2e/config.yaml` file to be present in the repo. This config contains the following properties that can be set based on what is needed by the App.

| property | type | description |
| --- | --- | --- |
| `appName` | string | The name of the App as it appears in the catalog |
| `repoName` | string | The name of the repository |
| `appCatalog` | string | The (non-test) catalog that the App is published into |
| `providers` | string array | A list of CAPI providers to test against when triggering from a PR (default if unset `capa`) |

Example:
```yaml
appName: ingress-nginx
repoName: ingress-nginx-app
appCatalog: giantswarm
providers:
- capa
- capv
```

## Running Tests Against a PR

Providing the repo has been setup to make use of `apptest-framework` following the above instructions you can then trigger the configured tests on any open PR by adding the following as a comment:

```
/run app-test-suites
```

This will run all test suites in the App repo against all providers configured in the config.yaml.

If you want to trigger the test suites against only a single provider (rather than everything configured in your config.yaml) then you can use the following (with the appropriate provider specified):

```
/run app-test-suites-single PROVIDER=capa
```

If you want to run specific test suites rather than all you can specify them as follows:

```
/run app-test-suites-single PROVIDER=capa TARGET_SUITES=basic,defaultapp
```

The `TARGET_SUITES` argument supports a comma separated list of test suite names that match the directory name under `./test/e2e/suites/`.

## Running Tests Locally

> [!NOTE]
> Make sure you have [Ginkgo installed](https://onsi.github.io/ginkgo/#installing-ginkgo)

Before you can run tests locally you must set the following required environment variables:

- `E2E_KUBECONFIG` must be set to the path to the kubeconfig of the test management cluster (e.g. `./kube/e2e.yaml`) - for the requirements of this kubeconfig please see [cluster-standup-teardown](https://github.com/giantswarm/cluster-standup-teardown) for more details.
- `E2E_KUBECONFIG_CONTEXT` must be set to the context to use for the management cluster in the kubeconfig (e.g. `capa`)
- `E2E_APP_VERSION` must be set to version of the app to test against (e.g. `3.5.1`). Note, this version must have already been published to the catalog.

Optionally, the following can be set to re-use an existing workload cluster:

- `E2E_WC_NAME` - the name of the workload cluster on the MC
- `E2E_WC_NAMESPACE` - the namespace the workload cluser is found in
- `E2E_WC_KEEP` - set to a truthy value to skip deleting the workload cluster at the end of the tests

Once those are set, you can trigger the E2E tests in you App repo with the following:

```sh
cd ./tests/e2e
ginkgo --timeout 4h -v -r ./suites/basic/
```

This will run the `basic` test suite. If you have others you wish to run, replace the directory with the test suite you want to trigger.

### Running local `apptest-framework` changes

If you need to run with a local copy of `apptest-framework` (such as when testing out changes to the framework) you can do so by adding the following to your Apps test go.mod (with the path correctly set to point to your checked out code):

```
replace github.com/giantswarm/apptest-framework => /path/to/my/apptest-framework
```

## Writing Tests

See [docs/WRITING_TESTS.md](docs/WRITING_TESTS.md).

## Resources

- [Ginkgo docs](https://onsi.github.io/ginkgo/)
- [`clustertest` documentation](https://pkg.go.dev/github.com/giantswarm/clustertest)
- [`cluster-standup-teardown`](https://github.com/giantswarm/cluster-standup-teardown)
- [CI Tekton Pipeline](https://github.com/giantswarm/tekton-resources/blob/main/tekton-resources/pipelines/app-test-suites.yaml)
