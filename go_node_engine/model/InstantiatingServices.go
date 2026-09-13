package model

import (
	"strconv"
	"sync"
)

// Services pulling an image have no containerd/runtime entry yet, so a
// runtime's own resource monitoring can't pick them up. Track them here so
// each runtime can report its own in-flight instantiations alongside its
// regular resource updates, keeping the cluster heartbeat alive during a
// long image pull.
var instantiatingServices sync.Map

func instantiatingServiceKey(sname string, instance int) string {
	return sname + "/" + strconv.Itoa(instance)
}

// TrackInstantiating records a service as being instantiated (e.g. its image
// is being pulled) so it shows up in InstantiatingResources until the
// corresponding UntrackInstantiating call.
func TrackInstantiating(service Service) {
	instantiatingServices.Store(instantiatingServiceKey(service.Sname, service.Instance), service)
}

// UntrackInstantiating removes a service from the instantiating registry,
// e.g. once its deployment has completed (successfully or not).
func UntrackInstantiating(sname string, instance int) {
	instantiatingServices.Delete(instantiatingServiceKey(sname, instance))
}

// InstantiatingResources returns the Resources entries for all services
// currently instantiating under the given runtime, so a runtime's
// ResourceMonitoring loop can report them alongside its regular updates.
func InstantiatingResources(runtime RuntimeType) []Resources {
	resources := make([]Resources, 0)
	instantiatingServices.Range(func(_, v any) bool {
		s := v.(Service)
		if RuntimeType(s.Runtime) == runtime {
			resources = append(resources, Resources{
				Sname:    s.Sname,
				Instance: s.Instance,
				Runtime:  s.Runtime,
				Status:   SERVICE_INSTANTIATION,
			})
		}
		return true
	})
	return resources
}
