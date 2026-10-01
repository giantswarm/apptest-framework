package client

import (
	"context"
	"fmt"
	"time"

	helmv2 "github.com/fluxcd/helm-controller/api/v2"
	"github.com/giantswarm/apiextensions-application/api/v1alpha1"
	"github.com/giantswarm/clustertest/v5/pkg/logger"
	cr "sigs.k8s.io/controller-runtime/pkg/client"

	"github.com/giantswarm/apptest-framework/v5/pkg/state"
)

const (
	// helmReleaseOriginNameLabel and helmReleaseOriginNamespaceLabel are put by helm-controller
	// on every object a HelmRelease installs, naming the HelmRelease it came from.
	helmReleaseOriginNameLabel      = "helm.toolkit.fluxcd.io/name"
	helmReleaseOriginNamespaceLabel = "helm.toolkit.fluxcd.io/namespace"
)

// helmReleaseOriginLabels selects the objects installed by the named HelmRelease.
func helmReleaseOriginLabels(name, namespace string) cr.MatchingLabels {
	return cr.MatchingLabels{
		helmReleaseOriginNameLabel:      name,
		helmReleaseOriginNamespaceLabel: namespace,
	}
}

// listHelmReleaseChildren returns the HelmReleases and App CRs the named HelmRelease installed
// into its own namespace, as `Kind/name`.
func listHelmReleaseChildren(ctx context.Context, name, namespace string) ([]string, error) {
	mcClient := state.GetFramework().MC()
	selector := helmReleaseOriginLabels(name, namespace)

	children := []string{}

	hrList := &helmv2.HelmReleaseList{}
	if err := mcClient.List(ctx, hrList, cr.InNamespace(namespace), selector); err != nil {
		return nil, fmt.Errorf("listing HelmReleases installed by %s/%s: %w", namespace, name, err)
	}
	for _, hr := range hrList.Items {
		children = append(children, "HelmRelease/"+hr.Name)
	}

	appList := &v1alpha1.AppList{}
	if err := mcClient.List(ctx, appList, cr.InNamespace(namespace), selector); err != nil {
		return nil, fmt.Errorf("listing App CRs installed by %s/%s: %w", namespace, name, err)
	}
	for _, app := range appList.Items {
		children = append(children, "App/"+app.Name)
	}

	return children, nil
}

// WaitForHelmReleaseChildrenDeleted blocks until every HelmRelease and App CR the named
// HelmRelease installed into its own namespace is gone, or the context expires.
//
// This matters for a HelmRelease installed in-cluster on the MC that itself renders apps for a
// workload cluster, such as an app bundle. Uninstalling it deletes those children, but each one
// holds a finalizer until its own uninstall has reached the workload cluster through the
// cluster's kubeconfig secret. Tearing the cluster down before then leaves the finalizers
// unable to run and the organization namespace stuck terminating.
func WaitForHelmReleaseChildrenDeleted(ctx context.Context, name, namespace string) error {
	for {
		children, err := listHelmReleaseChildren(ctx, name, namespace)
		if err != nil {
			return err
		}
		if len(children) == 0 {
			return nil
		}

		logger.Log("Waiting for %d resource(s) installed by HelmRelease %s/%s to be removed: %v", len(children), namespace, name, children)

		select {
		case <-ctx.Done():
			return fmt.Errorf("waiting for resources installed by HelmRelease %s/%s to be deleted, %v remaining: %w", namespace, name, children, ctx.Err())
		case <-time.After(5 * time.Second):
		}
	}
}
