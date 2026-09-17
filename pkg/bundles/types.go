package bundles

// ChildOverride describes the child app a bundle should install, independently of how
// the child itself is deployed. The App CR path and the Flux path need the same values
// layer, so it is expressed in the terms the bundle chart reads rather than in terms of
// an App CR.
type ChildOverride struct {
	// AppName is the child's app name, which also drives the key it is written under.
	AppName string
	// ChartName is the name the child's chart is published under, which is not always
	// AppName (`cluster-autoscaler-app` for the app named `cluster-autoscaler`).
	// Defaults to AppName when empty.
	ChartName string
	// Catalog is the catalog to install the child from. Ignored by bundle charts that
	// render their children as HelmReleases, and left to the chart's default when empty.
	Catalog string
	// Version is the child's chart version.
	Version string
	// Namespace is the namespace to install the child into. Left to the chart's default
	// when empty.
	Namespace string
}

type bundleValues struct {
	Apps map[string]appValues `yaml:"apps"`
}

// appValues is the per-child values layer a bundle chart reads. Every string field is
// omitempty: an empty value here is "not overridden", and marshalling it would override
// the chart's own default with an empty string. A blank chartName in particular renders
// a `charts/giantswarm/` OCI URL that cannot pull.
type appValues struct {
	Enabled   bool   `yaml:"enabled"`
	Catalog   string `yaml:"catalog,omitempty"`
	Version   string `yaml:"version,omitempty"`
	AppName   string `yaml:"appName,omitempty"`
	ChartName string `yaml:"chartName,omitempty"`
	Namespace string `yaml:"namespace,omitempty"`
}
