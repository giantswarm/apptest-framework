package client

import (
	"context"
	"fmt"
	"time"

	"github.com/giantswarm/clustertest/v5/pkg/failurehandler"
	"github.com/giantswarm/clustertest/v5/pkg/logger"

	helmv2 "github.com/fluxcd/helm-controller/api/v2"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	cr "sigs.k8s.io/controller-runtime/pkg/client"

	"github.com/giantswarm/apptest-framework/v5/pkg/state"
)

// diagnosticsTimeout bounds the gathering itself, which runs while a suite is already failing.
const diagnosticsTimeout = 5 * time.Minute

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
	return FailureDiagnosticsIn("", format, args...)
}

// FailureDiagnosticsIn is FailureDiagnostics for a wait that polls a namespace of its own.
//
// clustertest's handlers all report on the cluster's org namespace, which is where a suite's
// resources live by default. A suite that sets its own install namespace puts the HelmRelease
// somewhere else, and dumping the org namespace for it describes resources that have nothing to
// do with the failure while saying nothing about the one that timed out.
func FailureDiagnosticsIn(namespace string, format string, args ...any) func() string {
	return func() string {
		framework := state.GetFramework()
		cluster := state.GetCluster()

		if framework != nil && cluster != nil {
			if namespace != "" && namespace != cluster.Organization.GetNamespace() {
				run(helmReleasesNotReadyIn(namespace))
			}

			run(failurehandler.HelmReleasesNotReady(framework, cluster))
			run(failurehandler.AppIssues(framework, cluster))
			run(failurehandler.PodsNotReady(framework, cluster))
		}

		return fmt.Sprintf(format, args...)
	}
}

// helmReleasesNotReadyIn dumps the HelmReleases of one namespace, the way clustertest's own
// handler does for the org namespace. clustertest has no namespace-aware variant of it.
func helmReleasesNotReadyIn(namespace string) failurehandler.FailureHandler {
	return failurehandler.Wrap(func() {
		ctx, cancel := context.WithTimeout(context.Background(), diagnosticsTimeout) // #nosec G118
		defer cancel()

		mcClient := state.GetFramework().MC()

		logger.Log("Gathering HelmRelease status information in namespace '%s' for debugging", namespace)

		helmReleases := &helmv2.HelmReleaseList{}
		if err := mcClient.List(ctx, helmReleases, cr.InNamespace(namespace)); err != nil {
			logger.Log("Failed to get HelmReleases - %v", err)
			return
		}

		for _, hr := range helmReleases.Items {
			if isHelmReleaseConditionReady(hr) {
				continue
			}

			logger.Log("HelmRelease '%s/%s' is not ready:", hr.Namespace, hr.Name)
			for _, condition := range hr.Status.Conditions {
				logger.Log("  Condition: Type=%s, Status=%s, Reason=%s, Message=%s",
					condition.Type, condition.Status, condition.Reason, condition.Message)
			}

			events := &corev1.EventList{}
			if err := mcClient.List(ctx, events, cr.InNamespace(hr.Namespace),
				cr.MatchingFields{"involvedObject.name": hr.Name}); err != nil {
				continue
			}

			logger.Log("  Recent events:")
			for _, event := range events.Items {
				if event.InvolvedObject.Kind == "HelmRelease" {
					logger.Log("    %s: %s", event.Reason, event.Message)
				}
			}
		}
	})
}

// isHelmReleaseConditionReady reports whether the HelmRelease carries a ready condition.
func isHelmReleaseConditionReady(hr helmv2.HelmRelease) bool {
	for _, condition := range hr.Status.Conditions {
		if condition.Type == "Ready" && condition.Status == metav1.ConditionTrue {
			return true
		}
	}
	return false
}

// run invokes a clustertest FailureHandler. They are typed as `interface{}` so that Gomega
// accepts them, so there is nothing better than a type assertion to call one.
func run(handler failurehandler.FailureHandler) {
	if fn, ok := handler.(func() string); ok {
		_ = fn()
	}
}
