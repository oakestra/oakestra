package calculate

import (
	"errors"
	"reflect"
	"scheduler/calculate/schedulers/interfaces"
	"scheduler/logger"
	"scheduler/requests/manager"
	"scheduler/requests/resource"
	"strings"
	"sync"
)

var interestedResourcesCache sync.Map

// getInterestedResources uses the ResourceList to create a list of interested resources using their JSON annotation
func getInterestedResources[T interfaces.ResourceList](jobData T) []string {
	t := reflect.TypeOf(jobData)
	if t.Kind() == reflect.Ptr {
		t = t.Elem()
	}

	if t.Kind() != reflect.Struct {
		return nil
	}

	// Check cache first
	if cached, ok := interestedResourcesCache.Load(t); ok {
		return cached.([]string)
	}

	var interestedResources []string
	for i := 0; i < t.NumField(); i++ {
		f := t.Field(i)
		tag := f.Tag.Get("json")
		if tag == "" {
			continue
		}
		name := strings.Split(tag, ",")[0]
		if name == "-" || name == "" {
			continue
		}
		interestedResources = append(interestedResources, name)
	}

	logger.DebugLogger().Printf("Interested resources: %v", interestedResources)
	interestedResourcesCache.Store(t, interestedResources)
	return interestedResources
}

func PerformSchedulingRequest[T interfaces.ResourceList](job T, algorithm interfaces.Algorithm[T]) error {
	data := algorithm.ResourceList()

	err := resource.AvailableResources(&data, job.ResourceConstraints(), getInterestedResources(algorithm.JobData()))
	if err != nil {
		return err
	}
	logger.DebugLogger().Printf("Scheduling job: %+v", job)
	logger.DebugLogger().Printf("Available Resources: %+v", data)

	placementCandidate, err := algorithm.Calculate(job, data)
	if err != nil {
		var schedulingError interfaces.SchedulingError
		if errors.As(err, &schedulingError) {
			logger.ErrorLogger().Printf("Scheduling failed: Sending status %v to manager", err)
			err = manager.Deploy(job.GetId(), schedulingError.Error(), false)
		}
		return err
	}

	err = manager.Deploy(job.GetId(), placementCandidate.GetId(), true)

	return err
}
