package client

import (
	"context"
	"time"

	"github.com/giantswarm/apptest-framework/pkg/state"
	"github.com/giantswarm/clustertest/pkg/application"
	"github.com/giantswarm/clustertest/pkg/logger"
	"github.com/giantswarm/clustertest/pkg/wait"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

// InstallApp installs the given App then waits for it to be marked as installed.
// Timeout can be controlled via the provided context
func InstallApp(ctx context.Context, app *application.Application) {
	GinkgoHelper()

	logger.Log("Installing App %s (version: %s)", app.AppName, app.Version)

	err := state.GetFramework().MC().DeployApp(state.GetContext(), *app)
	Expect(err).NotTo(HaveOccurred())

	Eventually(wait.IsAppVersion(state.GetContext(), state.GetFramework().MC(), app.InstallName, app.GetNamespace(), app.Version)).
		WithContext(ctx).
		WithPolling(5 * time.Second).
		Should(BeTrue())

	Eventually(wait.IsAppDeployed(state.GetContext(), state.GetFramework().MC(), app.InstallName, app.GetNamespace())).
		WithContext(ctx).
		WithPolling(5 * time.Second).
		Should(BeTrue())
}
