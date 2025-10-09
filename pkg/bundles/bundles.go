package bundles

import (
	"fmt"
	"strings"

	"github.com/giantswarm/cluster-standup-teardown/v2/pkg/values"
	"github.com/giantswarm/clustertest/v2/pkg/application"
	"golang.org/x/text/cases"
	"golang.org/x/text/language"
	yaml "sigs.k8s.io/yaml/goyaml.v2"
)

// OverrideChildApp takes two apps, a bundle app and a child app, and attempts to correctly set the values of the bundle app
// to have it install the desired version of the child app.
func OverrideChildApp(bundleApp *application.Application, childApp *application.Application) (*application.Application, error) {
	appName := childApp.AppName

	if isCamelCaseName(bundleApp.AppName) {
		// Convert the app name to be camel case rather than hyphened
		appName = strings.ReplaceAll(appName, "-", " ")
		appName = cases.Title(language.English).String(appName)
		appName = strings.ReplaceAll(appName, " ", "")
		appName = strings.ToLower(appName[:1]) + appName[1:]
	} else if !isHyphenName(bundleApp.AppName) {
		return nil, fmt.Errorf("provided bundle is unsupported, child version override format is unknown")
	}

	overrideValues := bundleValues{
		Apps: map[string]appValues{
			appName: {
				Enabled:   true,
				Catalog:   childApp.Catalog,
				Version:   childApp.Version,
				AppName:   childApp.AppName,
				ChartName: childApp.AppName,
				Namespace: childApp.InstallNamespace,
			},
		},
	}

	valuesLayer, err := yaml.Marshal(overrideValues)
	if err != nil {
		return nil, err
	}

	finalValues, err := values.Merge(bundleApp.Values, string(valuesLayer))
	if err != nil {
		return nil, err
	}

	return bundleApp.WithValues(finalValues, &application.TemplateValues{})
}

func isCamelCaseName(appName string) bool {
	return strings.EqualFold(appName, "security-bundle") ||
		strings.EqualFold(appName, "observability-bundle") ||
		strings.EqualFold(appName, "gateway-api-bundle")
}

func isHyphenName(appName string) bool {
	return strings.EqualFold(appName, "service-mesh-bundle") ||
		strings.EqualFold(appName, "auth-bundle")
}
