package bundles

type bundleValues struct {
	Apps map[string]appValues `yaml:"apps"`
}

type appValues struct {
	Enabled   bool   `yaml:"enabled"`
	Catalog   string `yaml:"catalog"`
	Version   string `yaml:"version"`
	AppName   string `yaml:"appName"`
	ChartName string `yaml:"chartName"`
	Namespace string `yaml:"namespace"`
}
