package client

import (
	"fmt"

	"github.com/giantswarm/clustertest/v5/pkg/failurehandler"

	"github.com/giantswarm/apptest-framework/v5/pkg/state"
)

// FailureDiagnostics returns a Gomega failure description that dumps the state of everything
// involved in getting an app running before the assertion reports the given message.
//
// A timed-out wait otherwise says only that something never became ready, while the reason
// (`values-schema-violation`, an image that cannot be pulled, a chart that fails to render)
// sits in the resource's conditions, its events or a pod's logs. It matters more than it looks
// on the Flux path: a default app's HelmRelease is created with `remediation.retries: -1`, so a
// failing upgrade retries forever and nothing but a bounded wait will ever end the suite.
//
// Every kind is dumped regardless of how the app was installed, because a suite asserts on
// whichever of the two the owner happened to render.
func FailureDiagnostics(format string, args ...any) func() string {
	return func() string {
		framework := state.GetFramework()
		cluster := state.GetCluster()

		if framework != nil && cluster != nil {
			run(failurehandler.HelmReleasesNotReady(framework, cluster))
			run(failurehandler.AppIssues(framework, cluster))
			run(failurehandler.PodsNotReady(framework, cluster))
		}

		return fmt.Sprintf(format, args...)
	}
}

// run invokes a clustertest FailureHandler. They are typed as `interface{}` so that Gomega
// accepts them, so there is nothing better than a type assertion to call one.
func run(handler failurehandler.FailureHandler) {
	if fn, ok := handler.(func() string); ok {
		_ = fn()
	}
}
