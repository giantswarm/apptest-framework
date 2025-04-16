package client

import (
	"context"
	"time"

	"github.com/giantswarm/clustertest/pkg/application"
	"github.com/giantswarm/clustertest/pkg/logger"
	"github.com/giantswarm/clustertest/pkg/wait"

	"github.com/giantswarm/apptest-framework/pkg/state"

	. "github.com/onsi/ginkgo/v2" //nolint:staticcheck
	. "github.com/onsi/gomega"    //nolint:staticcheck
)

// InstallApp installs the given App then waits for it to be marked as installed.
// Timeout can be controlled via the provided context
func InstallApp(ctx context.Context, app *application.Application) {
	GinkgoHelper()

	builtApp, _, err := app.Build()
	Expect(err).NotTo(HaveOccurred())
	version := builtApp.Spec.Version

	logger.Log("Installing App %s (version: %s)", app.AppName, version)

	err = state.GetFramework().MC().DeployApp(state.GetContext(), *app)
	Expect(err).NotTo(HaveOccurred())

	Eventually(wait.IsAppVersion(state.GetContext(), state.GetFramework().MC(), app.InstallName, app.GetNamespace(), version)).
		WithContext(ctx).
		WithPolling(5 * time.Second).
		Should(BeTrue())

	Eventually(wait.IsAppDeployed(state.GetContext(), state.GetFramework().MC(), app.InstallName, app.GetNamespace())).
		WithContext(ctx).
		WithPolling(5 * time.Second).
		Should(BeTrue())
}
