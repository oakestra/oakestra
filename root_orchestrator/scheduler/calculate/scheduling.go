package calculate

import (
	"errors"
	"scheduler/calculate/schedulers/interfaces"
	"scheduler/logger"
	"scheduler/requests/manager"
	"scheduler/requests/resource"
)

func PerformSchedulingRequest[T interfaces.ResourceList](job T, algorithm interfaces.Algorithm[T]) error {
	data := algorithm.ResourceList()

	err := resource.AvailableResources(&data, job.ResourceConstraints())
	if err != nil {
		return err
	}
	logger.DebugLogger().Printf("Available Resources: %v", data)

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
