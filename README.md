# apptest-framework

<a href="https://godoc.org/github.com/giantswarm/apptest-framework"><img src="https://godoc.org/github.com/giantswarm/apptest-framework?status.svg"></a>

A test framework for helping with E2E testing of Giant Swarm managed Apps within Giant Swarm clusters.

## Installation

```shell
go get github.com/giantswarm/apptest-framework
```

## Features

- Handles test suite setup (using Ginkgo)
- Provides shared state across test cases
- Provides hooks for pre-install, pre-upgrade and post-install steps

## Required Environment Variables

- `E2E_KUBECONFIG` must be set to the kubeconfig of the test management cluster
- `E2E_KUBECONFIG_CONTEXT` must be set to the context to use in the kubeconfig
- `E2E_WC_NAME` must be set to the name of the test workload cluster
- `E2E_WC_NAMESPACE` must be set to namespace the test workload cluster is in within the management cluster
- `E2E_APP_VERSION` must be set to version of the app to test against

## Documentation

Documentation can be found at: [pkg.go.dev/github.com/giantswarm/apptest-framework](https://pkg.go.dev/github.com/giantswarm/apptest-framework).

## Resources

- [Ginkgo docs](https://onsi.github.io/ginkgo/)
- [`clustertest` documentation](https://pkg.go.dev/github.com/giantswarm/clustertest)
- [CI Tekton Pipeline](https://github.com/giantswarm/tekton-resources/blob/main/tekton-resources/pipelines/app-test-suites.yaml)
