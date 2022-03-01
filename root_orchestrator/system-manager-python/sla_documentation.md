# Documentation of EdgeIO-SLA (v0.3.2)
Example under *sla.json*

- **api_version** : Version of SLA API; v0.3.2 as of now
- **customerID** : ID of the customer
- **args** : *see "On expandability"*
- **applications** : List of applications run by the customer
  - **applicationID** : ID of the application described,
  - **application_name** : Name of the application
  - **application_namespace** : (Optional) : Namespace of the application
  - **application_desc** : Description of the application
  - **microservices** : List of microservices this application needs
    - **microserviceID** : ID of a microservice; automatically generated (UUID)
    - **microservice_name** : Name of the microservice; used for addressing
    - **microservice_namespace** : Optional - User may specify a name to be used in the address of the microservice (if it is still available)
    - **virtualization** : type of virtualization chosen for the application, may be one of ["container", "unikernel", "vm"]
    - **memory** : Needed memory in MB,
    - **vcpus** : Needed vCPUs, default 1
    - **vgpus** : Needed vGPUs, default 0
    - **vtpus** : Needed vTPUs, default 0
    - **bandwidth_in** : Minimum bandwidth-ingress needed for application in kbit/s, default 0
    - **bandwith_out** : Minimum bandwidth-egress needed for application in kbit/s, default 0
    - **storage** : Permanent Storage needed in MB, default 0
    - **code** : File containing the code; given as URL
    - **state** : File containing the state; given as URL; default empty
    - **port** : Port for exposure of the microservice chosen by the developer
    - **addresses** : Optional - ***[Taken from Giovanni's and Mehdi's design; more details in On addressess]***
      - **rr_ip** : Optional - IP chosen for round-robin addressation
      - **closest_ip** : Optional - The orchestrator may choose the closest IP to the given one
      - **instances** : Optional - Field of instances
        - **from**
        - **to**
        - **start**
    - **added_files** : List of added files necessary, can be configured by the developer
      - **url** : URL of a file, that the developer needs to have added to the microservice
    - **args** : *see "On expandability"*
    - **constraints** : List of constraints that need to be applied
      - **type** : Type of constraint; one of ["latency", "geo"]
        - ***For type "latency"***
        - **area** : Specifies the area in which the constraint is in effect; must be chosen from a list of predefined areas (Mainly urban areas)
        - **threshold** : Maximum latency in [ms],
        - **rigidness** : Rigidness of constraint; If [**rigidness**] * [recent_requests] > [successfull_recent_requests], the constraint is counted as failed: *Example*: rigidness of 1 implies the first failure to meet goals triggers an alarm. rigidness of 0.99 implies that 99% (or more) of recent requests must satisfy the constraint, otherwise an alarm gets triggered.
        - **convergence_time** : Time the orchestration framework has to find the optimal solution, before the [**rigidness**] of the constraint gets measured. In [s], default 300 (5 min)
        - ***For type "geo", see "About geo"***
        - **location** : Specifies the location close to which the service should be deployed (as a tuple of (longitude; latitude))
        - **threshold** : Maximum distance from location in [km],
        - **rigidness** : Rigidness of constraint; If [**rigidness**] * [recent requests] > [successfull recent requests], the constraint is counted as failed: *Example*: rigidness of 1 implies the first failure to meet goals triggers an alarm. rigidness of 0.99 implies that 99% (or more) of recent requests must satisfy the constraint, otherwise an alarm gets triggered.
        - **convergence_time** : Time the orchestration framework has to find the optimal solution, before the [**rigidness**] of the constraint gets measured. In [s], default 300 (5 min)
    - **connectivity** : List of connections this microservice needs to make to other microservices of the application and constraints that have to be satisfied; for more information read ***On conectivity***
      - **target_microservice_id** : ID of the microservice this microservice needs to communicate with.
      - **con_constraints** : List of connectivity constraints
        - **type** : Type of constraint; one of ["latency", "bandwidth"]
          - **threshold** : For "latency" maximum latency of the connection in [ms]; for "bandwidth" minimum bandwidth in [kbit/s]
          - **rigidness** : Rigidness of constraint; see definition in constraint.
          - **convergence_time** : Time the orchestration framework has to find the optimal solution, before the [**rigidness**] of the constraint gets measured. In [s], default 300 (5 min)
          
## On expandability

There are multiple points for this design to be expanded upon:

First, both the customer and its applications can be expanded upon with multiple further arguments, either in place of or as a dictionary in the field "args" (present both in the "customer"-object and the "application"-object, though obviously not referencing the exact same type)

### Constraints

Secondly, a main point to expand this SLA-scheme is by adding constraint-types: All constraints listed under the "constraints"-attribute must be fulfilled (according to their "rigidness") to not trigger the alarm. This way one might i.e. create coverage for bigger areas than just one predefined area: by adding one latency constraint for the area "munich-1" and another one for area "berlin-1", he can consequently define coverage of his application for multiple areas.

The **convergence_time** is the time the orchestration framework has, to find one (or more) suitable nodes to deploy the microservice to. During this initial time, the rigidness of the constraint is not enforced. A higher **convergence_time** might result in better and maybe even fewer nodes being chosen for the service, whereas a low one might result in the system being "overly careful" right away and deploying on more nodes just to make sure, before figuring out the best option.

## On connectivity

This attribute is designed to define the ways a developer might want to specify, how his different microservices communicate within an application.

Firstly, the developer specifies a list of microserviceIDs he wants to communicate with. For example if microsevice 4 (which we are configuring right now) needs to communicate with microservices 1 and 2, he lists them here.

Furthermore, the developer can now also specify constraints, that the placement of the microservices must satisfy. For example, while microservices 4 and 1 have no constraints whatsoever to fulfill, microservices 4 and 2 have to be able to communicate with at most 50 ms latency at least 90% of the time. This can be specified under the field **con_constraints** using the latency-type.

This example of connectivity is "fleshed out" in the demo ***sla.json***

**Note**: This way of defining connectivity always assumes bidirectional connections. In case there are contradictions in a connection, i.e. microservice 4 -> 1 is defined with maximum 30 ms; microservice 1 -> 4 is defined with maximum 50 ms (or not defined at all), the scheduler should assume connectivity and take the tighter constraint (30 ms).

## On addresses

These fields are added on the account of Giovanni, who introduced them together with Mehdi to allow developers to choose an IP address their microservice should be available at, in case it is unoccupied.
