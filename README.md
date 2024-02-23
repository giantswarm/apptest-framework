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

## Documentation

Documentation can be found at: [pkg.go.dev/github.com/giantswarm/apptest-framework](https://pkg.go.dev/github.com/giantswarm/apptest-framework).

## Resources

* [Ginkgo docs](https://onsi.github.io/ginkgo/)
* [`clustertest` documentation](https://pkg.go.dev/github.com/giantswarm/clustertest)
* [CI Tekton Pipeline](https://github.com/giantswarm/tekton-resources/blob/main/tekton-resources/pipelines/app-test-suites.yaml)
