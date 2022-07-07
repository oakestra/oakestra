# Examples

This folder contains some examples that can be used to play a bit with the deployment 

## How to:

Once you have deployed your fully functional cluster (Root Orch, (at least 1)Cluster Orchestrator, (at least 1) worker node) do the following:

1) go to root-orch-url:10000/api/docs
2) You'll se the openapi docs you can play with 
3) Use the login API to generate the access token
4) Use the access token to authorize the following requests. Simply paste is into the "authorize" box.
5) Create your application using the POST /api/application endpoint 
6) Check you app identifier from the GET /api/applications endpoint
7) Use the endpoint POST /api/service to create the service metadata. Fill up the json request using one of the example deployment descriptors in this folder. Remember to replace the application-id accordingly.
8) Get the service-id of the generated service using the GET /api/services endpoint
9) Deploy a new instance using the POST /api/service/{serviceid}/instance endpoint

Feel free to experiment the other endpoints. 

You can also use the official dashboard or CLI to deploy the services in a more Human friendly way :)