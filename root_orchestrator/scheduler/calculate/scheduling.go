package calculate

import (
	"scheduler/calculate/schedulers/interfaces"
	"scheduler/requests/manager"
	"scheduler/requests/resource"
)

func PerformSchedulingRequest[T interfaces.ResourceList](job T, algorithm interfaces.Algorithm[T]) error {
	data := algorithm.ResourceList()

	err := resource.AvailableResources(data, job.ResourceConstraints())
	if err != nil {
		return err
	}

	placementCandidate := algorithm.Calculate(job, data)

	err = manager.Deploy(job.GetId(), placementCandidate.GetId())

	return err
}
