package state

import (
	"context"
	"sync"

	"github.com/giantswarm/clustertest/v3"
	"github.com/giantswarm/clustertest/v3/pkg/application"
)

var lock = &sync.Mutex{}

type state struct {
	framework         *clustertest.Framework
	cluster           *application.Cluster
	application       *application.Application
	bundleApplication *application.Application
	ctx               context.Context
}

var singleInstance *state

func get() *state {
	if singleInstance == nil {
		lock.Lock()
		defer lock.Unlock()
		if singleInstance == nil {
			singleInstance = &state{}
		}
	}

	return singleInstance
}

func SetContext(ctx context.Context) {
	s := get()
	s.ctx = ctx
}

func GetContext() context.Context {
	return get().ctx
}

func SetFramework(framework *clustertest.Framework) {
	s := get()
	s.framework = framework
}

func GetFramework() *clustertest.Framework {
	return get().framework
}

func SetCluster(framework *application.Cluster) {
	s := get()
	s.cluster = framework
}

func GetCluster() *application.Cluster {
	return get().cluster
}

func SetApplication(app *application.Application) {
	s := get()
	s.application = app
}

func GetApplication() *application.Application {
	return get().application
}

func SetBundleApplication(app *application.Application) {
	s := get()
	s.bundleApplication = app
}

func GetBundleApplication() *application.Application {
	return get().bundleApplication
}
