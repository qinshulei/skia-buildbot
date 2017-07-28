package autoscaler

import (
	"path/filepath"
	"sort"
	"sync"

	"google.golang.org/api/compute/v0.alpha"

	"go.skia.org/infra/go/auth"
	"go.skia.org/infra/go/gce"
	"go.skia.org/infra/go/util"
)

// Interface useful for mocking.
type IAutoscaler interface {
	GetRunningInstances() ([]string, error)
	StopAllInstances() error
	StartAllInstances() error
}

// Autoscaler is a struct used for autoscaling instances in GCE.
type Autoscaler struct {
	g         *gce.GCloud
	workdir   string
	instances []*gce.Instance
}

// NewAutoscaler returns an Autoscaler instance.
func NewAutoscaler(zone, workdir string, minInstanceNum, maxInstanceNum int, getInstance func(int) *gce.Instance) (*Autoscaler, error) {
	// Get the absolute workdir.
	wdAbs, err := filepath.Abs(workdir)
	if err != nil {
		return nil, err
	}

	// Create the GCloud object.
	httpClient, err := auth.NewClient(false, "", compute.CloudPlatformScope, compute.ComputeScope, compute.DevstorageFullControlScope)
	if err != nil {
		return nil, err
	}
	g, err := gce.NewGCloudWithClient(zone, wdAbs, httpClient)
	if err != nil {
		return nil, err
	}
	// Create slice of instances.
	instances := []*gce.Instance{}
	for num := minInstanceNum; num <= maxInstanceNum; num++ {
		instances = append(instances, getInstance(num))
	}
	return &Autoscaler{
		g:         g,
		workdir:   workdir,
		instances: instances,
	}, nil
}

// GetRunningInstances returns a slice of all running instance names.
func (a *Autoscaler) GetRunningInstances() ([]string, error) {
	runningInstances := []string{}
	// Mutex to control access to above slice.
	var m sync.Mutex
	// Perform the requested operation.
	group := util.NewNamedErrGroup()
	for _, instance := range a.instances {
		name := instance.Name // https://golang.org/doc/faq#closures_and_goroutines
		group.Go(name, func() error {
			if a.g.IsInstanceRunning(name) {
				m.Lock()
				runningInstances = append(runningInstances, name)
				m.Unlock()
			}
			return nil
		})
	}
	if err := group.Wait(); err != nil {
		return nil, err
	}
	sort.Strings(runningInstances)
	return runningInstances, nil
}

// StopAllInstances stops all instances.
func (a *Autoscaler) StopAllInstances() error {
	// Perform the requested operation.
	group := util.NewNamedErrGroup()
	for _, instance := range a.instances {
		instance := instance // https://golang.org/doc/faq#closures_and_goroutines
		group.Go(instance.Name, func() error {
			return a.g.Stop(instance)
		})
	}
	if err := group.Wait(); err != nil {
		return err
	}
	return nil
}

// StartAllInstances starts all instances.
func (a *Autoscaler) StartAllInstances() error {
	// Perform the requested operation.
	group := util.NewNamedErrGroup()
	for _, instance := range a.instances {
		instance := instance // https://golang.org/doc/faq#closures_and_goroutines
		group.Go(instance.Name, func() error {
			return a.g.Start(instance)
		})
	}
	if err := group.Wait(); err != nil {
		return err
	}
	return nil
}
