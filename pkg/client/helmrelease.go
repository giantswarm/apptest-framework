package client

import (
	"context"
	"fmt"
	"time"

	helmv2 "github.com/fluxcd/helm-controller/api/v2"
	"github.com/giantswarm/clustertest/v4/pkg/logger"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	cr "sigs.k8s.io/controller-runtime/pkg/client"

	"github.com/giantswarm/apptest-framework/v4/pkg/state"

	. "github.com/onsi/ginkgo/v2" //nolint:staticcheck
	. "github.com/onsi/gomega"    //nolint:staticcheck
)

// SourceKind represents the kind of source reference used by a HelmRelease.
type SourceKind string

const (
	// SourceKindHelmRepository uses spec.chart with a HelmRepository sourceRef.
	SourceKindHelmRepository SourceKind = "HelmRepository"
	// SourceKindOCIRepository uses spec.chartRef with an OCIRepository reference.
	SourceKindOCIRepository SourceKind = "OCIRepository"
)

// HelmReleaseConfig holds the configuration needed to create a HelmRelease CR.
type HelmReleaseConfig struct {
	// Name is the name of the HelmRelease resource.
	Name string
	// Namespace is the namespace where the HelmRelease CR will be created.
	Namespace string
	// TargetNamespace is the namespace where the Helm chart will be installed.
	TargetNamespace string
	// StorageNamespace is the namespace used for Helm storage.
	// If empty, defaults to the HelmRelease namespace.
	StorageNamespace string
	// ReleaseName is the Helm release name. If empty, defaults to the HelmRelease name.
	ReleaseName string
	// ChartName is the name of the chart in the source.
	ChartName string
	// ChartVersion is the version of the chart to install.
	ChartVersion string
	// SourceKind specifies the kind of source reference.
	// Defaults to SourceKindOCIRepository if not set.
	SourceKind SourceKind
	// SourceName is the name of the source reference (HelmRepository or OCIRepository).
	SourceName string
	// SourceNamespace is the namespace of the source reference.
	// If empty, defaults to the HelmRelease namespace.
	SourceNamespace string
	// Values is the raw values YAML to pass to the chart.
	Values string
	// Interval is the reconciliation interval. Defaults to 5m.
	Interval time.Duration
	// Timeout is the time to wait for Helm operations. Defaults to 5m.
	Timeout time.Duration
	// Retries is the number of retries for install/upgrade remediation. Defaults to 10.
	Retries *int
	// ServiceAccountName is the Kubernetes service account to impersonate when reconciling.
	// Required by clusters with the flux-multi-tenancy Kyverno policy.
	ServiceAccountName string
}

// InstallHelmRelease creates a HelmRelease CR and waits for it to become ready.
// It also ensures that the target and storage namespaces exist before creating the HelmRelease.
// Timeout can be controlled via the provided context.
func InstallHelmRelease(ctx context.Context, cfg HelmReleaseConfig) {
	GinkgoHelper()

	if cfg.Interval == 0 {
		cfg.Interval = 5 * time.Minute
	}

	// Default SourceName to ChartName if not set
	if cfg.SourceName == "" {
		cfg.SourceName = cfg.ChartName
	}

	// Ensure required namespaces exist
	ensureNamespace(ctx, cfg.Namespace)
	if cfg.TargetNamespace != "" {
		ensureNamespace(ctx, cfg.TargetNamespace)
	}
	if cfg.StorageNamespace != "" {
		ensureNamespace(ctx, cfg.StorageNamespace)
	}

	// Ensure the service account exists
	if cfg.ServiceAccountName != "" {
		ensureServiceAccount(ctx, cfg.ServiceAccountName, cfg.Namespace)
	}

	hr := buildHelmRelease(cfg)
	logger.Log("Installing HelmRelease %s/%s (chart: %s, version: %s, source: %s/%s)",
		hr.Namespace, hr.Name, cfg.ChartName, cfg.ChartVersion, cfg.SourceKind, cfg.SourceName)

	err := state.GetFramework().MC().CreateOrUpdate(state.GetContext(), hr)
	Expect(err).NotTo(HaveOccurred())

	state.SetHelmRelease(hr)

	Eventually(func() (bool, error) {
		return IsHelmReleaseReady(state.GetContext(), hr.Name, hr.Namespace)
	}).
		WithContext(ctx).
		WithPolling(5 * time.Second).
		Should(BeTrue())
}

// IsHelmReleaseReady checks if a HelmRelease has the Ready condition set to True.
func IsHelmReleaseReady(ctx context.Context, name, namespace string) (bool, error) {
	hr := &helmv2.HelmRelease{}
	err := state.GetFramework().MC().Get(ctx, types.NamespacedName{Name: name, Namespace: namespace}, hr)
	if err != nil {
		if errors.IsNotFound(err) {
			return false, nil
		}
		return false, err
	}

	for _, condition := range hr.Status.Conditions {
		if condition.Type == "Ready" {
			return condition.Status == metav1.ConditionTrue, nil
		}
	}

	return false, nil
}

// IsHelmReleaseVersion checks if a HelmRelease has the expected chart version in its status history.
func IsHelmReleaseVersion(ctx context.Context, name, namespace, version string) (bool, error) {
	hr := &helmv2.HelmRelease{}
	err := state.GetFramework().MC().Get(ctx, types.NamespacedName{Name: name, Namespace: namespace}, hr)
	if err != nil {
		if errors.IsNotFound(err) {
			return false, nil
		}
		return false, err
	}

	// Check spec.chart if using HelmRepository source
	if hr.Spec.Chart != nil {
		return hr.Spec.Chart.Spec.Version == version, nil
	}

	// For OCIRepository sources, check the last attempted revision in status
	if hr.Status.LastAttemptedRevision != "" {
		return hr.Status.LastAttemptedRevision == version, nil
	}

	return false, nil
}

// DeleteHelmRelease deletes a HelmRelease CR and its associated values Secret if present.
func DeleteHelmRelease(ctx context.Context, name, namespace string) error {
	hr := &helmv2.HelmRelease{
		ObjectMeta: metav1.ObjectMeta{
			Name:      name,
			Namespace: namespace,
		},
	}
	err := state.GetFramework().MC().Delete(ctx, hr)
	if err != nil && !errors.IsNotFound(err) {
		return fmt.Errorf("deleting HelmRelease %s/%s: %w", namespace, name, err)
	}

	// Clean up the values secret if it was created
	secretName := fmt.Sprintf("%s-values", name)
	secret := &corev1.Secret{
		ObjectMeta: metav1.ObjectMeta{
			Name:      secretName,
			Namespace: namespace,
		},
	}
	err = state.GetFramework().MC().Delete(ctx, secret)
	if err != nil && !errors.IsNotFound(err) {
		return fmt.Errorf("deleting values Secret %s/%s: %w", namespace, secretName, err)
	}

	return nil
}

func buildHelmRelease(cfg HelmReleaseConfig) *helmv2.HelmRelease {
	sourceName := cfg.SourceName
	if sourceName == "" {
		sourceName = cfg.ChartName
	}

	sourceNamespace := cfg.SourceNamespace
	if sourceNamespace == "" {
		sourceNamespace = cfg.Namespace
	}

	sourceKind := cfg.SourceKind
	if sourceKind == "" {
		sourceKind = SourceKindOCIRepository
	}

	retries := 10
	if cfg.Retries != nil {
		retries = *cfg.Retries
	}

	hr := &helmv2.HelmRelease{
		TypeMeta: metav1.TypeMeta{
			APIVersion: "helm.toolkit.fluxcd.io/v2",
			Kind:       "HelmRelease",
		},
		ObjectMeta: metav1.ObjectMeta{
			Name:      cfg.Name,
			Namespace: cfg.Namespace,
		},
		Spec: helmv2.HelmReleaseSpec{
			Interval: metav1.Duration{Duration: cfg.Interval},
			Install: &helmv2.Install{
				Remediation: &helmv2.InstallRemediation{
					Retries: retries,
				},
			},
			Upgrade: &helmv2.Upgrade{
				Remediation: &helmv2.UpgradeRemediation{
					Retries:              retries,
					Strategy:             &[]helmv2.RemediationStrategy{helmv2.RollbackRemediationStrategy}[0],
					RemediateLastFailure: boolPtr(true),
				},
			},
		},
	}

	if cfg.Timeout > 0 {
		hr.Spec.Timeout = &metav1.Duration{Duration: cfg.Timeout}
	}

	switch sourceKind {
	case SourceKindOCIRepository:
		hr.Spec.ChartRef = &helmv2.CrossNamespaceSourceReference{
			Kind:      string(SourceKindOCIRepository),
			Name:      sourceName,
			Namespace: sourceNamespace,
		}
	case SourceKindHelmRepository:
		hr.Spec.Chart = &helmv2.HelmChartTemplate{
			Spec: helmv2.HelmChartTemplateSpec{
				Chart:   cfg.ChartName,
				Version: cfg.ChartVersion,
				SourceRef: helmv2.CrossNamespaceObjectReference{
					Kind:      string(SourceKindHelmRepository),
					Name:      sourceName,
					Namespace: sourceNamespace,
				},
			},
		}
	}

	if cfg.TargetNamespace != "" {
		hr.Spec.TargetNamespace = cfg.TargetNamespace
	}

	if cfg.StorageNamespace != "" {
		hr.Spec.StorageNamespace = cfg.StorageNamespace
	}

	if cfg.ReleaseName != "" {
		hr.Spec.ReleaseName = cfg.ReleaseName
	}

	if cfg.ServiceAccountName != "" {
		hr.Spec.ServiceAccountName = cfg.ServiceAccountName
	}

	if cfg.Values != "" {
		hr.Spec.ValuesFrom = append(hr.Spec.ValuesFrom, helmv2.ValuesReference{
			Kind: "Secret",
			Name: fmt.Sprintf("%s-values", cfg.Name),
		})
	}

	return hr
}

// CreateValuesSecret creates a Secret containing chart values for a HelmRelease.
// It ensures the target namespace exists before creating the Secret.
func CreateValuesSecret(ctx context.Context, name, namespace, values string) {
	GinkgoHelper()

	ensureNamespace(ctx, namespace)

	secret := &corev1.Secret{
		TypeMeta: metav1.TypeMeta{
			APIVersion: "v1",
			Kind:       "Secret",
		},
		ObjectMeta: metav1.ObjectMeta{
			Name:      fmt.Sprintf("%s-values", name),
			Namespace: namespace,
		},
		StringData: map[string]string{
			"values.yaml": values,
		},
	}

	err := state.GetFramework().MC().CreateOrUpdate(ctx, secret)
	Expect(err).NotTo(HaveOccurred())
}

// UpdateHelmReleaseVersion updates the chart version of an existing HelmRelease.
// For HelmRepository sources, it updates spec.chart.spec.version.
// For OCIRepository sources, the version is controlled by the OCIRepository itself,
// so this is a no-op and the caller should update the OCIRepository instead.
func UpdateHelmReleaseVersion(ctx context.Context, name, namespace, version string) {
	GinkgoHelper()

	hr := &helmv2.HelmRelease{}
	err := state.GetFramework().MC().Get(ctx, types.NamespacedName{Name: name, Namespace: namespace}, hr)
	Expect(err).NotTo(HaveOccurred())

	if hr.Spec.Chart != nil {
		hr.Spec.Chart.Spec.Version = version
		err = state.GetFramework().MC().Update(ctx, hr, &cr.UpdateOptions{})
		Expect(err).NotTo(HaveOccurred())
	}
}

// ensureServiceAccount creates a service account if it doesn't already exist.
func ensureServiceAccount(ctx context.Context, name, namespace string) {
	sa := &corev1.ServiceAccount{
		ObjectMeta: metav1.ObjectMeta{
			Name:      name,
			Namespace: namespace,
		},
	}
	err := state.GetFramework().MC().Get(ctx, types.NamespacedName{Name: name, Namespace: namespace}, sa)
	if errors.IsNotFound(err) {
		logger.Log("Creating ServiceAccount %s/%s", namespace, name)
		sa.TypeMeta = metav1.TypeMeta{
			APIVersion: "v1",
			Kind:       "ServiceAccount",
		}
		err = state.GetFramework().MC().Create(ctx, sa)
		if err != nil && !errors.IsAlreadyExists(err) {
			Expect(err).NotTo(HaveOccurred())
		}
	}
}

// ensureNamespace creates a namespace if it doesn't already exist.
func ensureNamespace(ctx context.Context, name string) {
	ns := &corev1.Namespace{
		TypeMeta: metav1.TypeMeta{
			APIVersion: "v1",
			Kind:       "Namespace",
		},
		ObjectMeta: metav1.ObjectMeta{
			Name: name,
		},
	}
	err := state.GetFramework().MC().Get(ctx, types.NamespacedName{Name: name}, ns)
	if errors.IsNotFound(err) {
		logger.Log("Creating namespace %s", name)
		err = state.GetFramework().MC().Create(ctx, ns)
		if err != nil && !errors.IsAlreadyExists(err) {
			Expect(err).NotTo(HaveOccurred())
		}
	}
}

func boolPtr(b bool) *bool {
	return &b
}
