package calculate

import (
	"errors"
	"reflect"
	"scheduler/calculate/schedulers/placement"
	"scheduler/logger"
	"scheduler/requests/manager"
	"scheduler/requests/resource"
	"strings"
	"sync"
)

var interestedResourcesCache sync.Map

// getInterestedResources extracts the JSON field names from a Candidate type
// via reflection, recursing into anonymous embedded structs. Results are
// cached by type so reflection runs only once per concrete candidate type.
func getInterestedResources[C placement.Candidate](candidate C) []string {
	t := reflect.TypeOf(candidate)
	if t.Kind() == reflect.Ptr {
		t = t.Elem()
	}
	if t.Kind() != reflect.Struct {
		return nil
	}

	if cached, ok := interestedResourcesCache.Load(t); ok {
		return cached.([]string)
	}

	fields := collectJSONFields(t)

	logger.DebugLogger().Printf("Interested resources: %v", fields)
	interestedResourcesCache.Store(t, fields)
	return fields
}

// collectJSONFields walks the struct type t and returns the names from all
// json struct tags, recursing into anonymous embedded structs so that
// promoted fields are included alongside fields declared directly on t.
func collectJSONFields(t reflect.Type) []string {
	var fields []string
	for i := range t.NumField() {
		f := t.Field(i)
		// Recurse into anonymous (embedded) struct fields that carry no json
		// tag of their own — their promoted fields need to be inspected directly.
		if f.Anonymous && f.Tag.Get("json") == "" {
			ft := f.Type
			if ft.Kind() == reflect.Ptr {
				ft = ft.Elem()
			}
			if ft.Kind() == reflect.Struct {
				fields = append(fields, collectJSONFields(ft)...)
				continue
			}
		}
		tag := f.Tag.Get("json")
		if tag == "" {
			continue
		}
		name, _, _ := strings.Cut(tag, ",")
		if name == "-" || name == "" {
			continue
		}
		fields = append(fields, name)
	}
	return fields
}

func PerformSchedulingRequest[J placement.Job, C placement.Candidate](job J, algorithm placement.Algorithm[J, C]) error {
	var candidates []C
	var zeroCandidate C
	err := resource.AvailableResources(&candidates, job.ResourceConstraints(), getInterestedResources(zeroCandidate))
	if err != nil {
		return err
	}
	logger.DebugLogger().Printf("Available Resources: %v", candidates)

	chosen, err := algorithm.Calculate(job, candidates)
	if err != nil {
		var schedulingError placement.SchedulingError
		if errors.As(err, &schedulingError) {
			logger.ErrorLogger().Printf("Scheduling failed: Sending status %v to manager", err)
			err = manager.Deploy(job.ID(), schedulingError.Error(), false)
		}
		return err
	}

	logger.InfoLogger().Printf("Scheduled job %s to candidate %s", job.ID(), chosen.ID())
	return manager.Deploy(job.ID(), chosen.ID(), true)
}
