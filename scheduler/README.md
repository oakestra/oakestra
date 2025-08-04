# Scheduler

The scheduler is the scheduling component of the Oakestra control plane.
It accepts deployment job requests in the form of SLAs and returns the appropriate
scheduling candidate (cluster or worker) on which the service should be scheduled.
If no appropriate ResourceList could be found a NegativeSchedulingResult is returned.

The scheduler was constructed with expandability in mind. A new scheduler can be implemented by defining a ResourceList and a calculate function.
The scheduler works agnostically to the underlying ResourceList and SchedulingAlgorithm.

## Taxonomy
- ResourceList: A collection of named resources. A new scheduler can be implemented by defining a new ResourceList with a new calculate function.
- Job: An object of type ResourceList. It defines the required resources for a service and is provided in the form of an SLA
- Placement Candidate: An object of type ResourceList. It defines the available resources of a cluster/worker and is provided by the ResourceAbstractor

## Architecture
![fig](fig/scheduler-arch.drawio.svg)

1. The API Module receives deployment job requests. 
2. These jobs are enqueued with asynq and stored in a redis db. 
3. In order to ascertain the placement candidates a request is sent to the ResourceAbstractor. 
4. The calculate function is applied to the jobs and placement candidates.
5. The calculate function is implemented in an instance of the scheduler interface. 
6. The most appropriate candidate is sent to the Manager so that the job can be scheduled.


## Interfacing with the Scheduler
The scheduler will expose an API endpoint at `[API_PORT]:/api/calculate/deploy`.
`API_PORT` should be defined in the docker compose file and the port must also be exposed.

The scheduler will send the response back to `[MANAGER_URL]:[MANAGER_PORT]/api/result/deploy`
Where `MANAGER_URL` and `MANAGER_PORT` should be defined as environment variables in the docker compose file

## Implementing new Scheduler behaviour

New scheduling behaviour can be rapidly introduced by implementing `ResourceList` and `Algorithm[T ResourceList]`. These interfaces are defined in `calculate/schedulers/interfaces`.

The `ResourceList` implementation should define the resources (name and type) that this scheduling algorithm will consider. The struct must be annotated with the json tags, so that the struct can be used to marshall the job and the data returned by the ResourceAbstractor API.

The `ResourceList` must, at least, provide: 
- The `GetId()` function, as jobs and placement candidates will always have an id 
- The `ResourceConstraints()` function, that returns a mapping for constraints to values. These could just be the provided `GenericConstraints` which should always be considered by the scheduling algorithm
- (Optionally) a custom Unmarshaller should be implemented `UnmarshalJSON(data []byte) error`

The `Algorithm` implementation is parameterized with a ResourceList implementation.

The `Algorithm` mist, at least, provide:
- The`ResourceList() []T ` function, that returns a slice of ResourceLists objects. These objects are empty as the slice is used to capture the response from the ResourceAbstractor API
- `JobData() T`, that returns an empty ResourceList object. This is used to capture the scheduling request payload
- `Calculate(job T, candidates []T) (T, error)`, the crux of the scheduler implementation. This function should evaluate the job and return the best candidate from the list
