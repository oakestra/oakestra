# Resource Abstractor
The resource abstractor service is a microservice that interacts with the cluster database and abstracts the available information. The service exposes an API that can be used to query the currently available resources.


## Ingoing Endpoints

- `/resources`: Endpoint for retrieving information on resources. Only support querying. 
- `/jobs`: Endpoint to query the jobs in the system. Also supports updating a job document, such as updating the job status.

For more details checkout out the [openapi.json](./api/v1/openapi.json)


## Start Resource Abstractor

Run the service by running `./start-up.sh`

A virtualenv will be started and the component will start up.


## Built With

Python 3.8
- flask
- flask_pymongo
- flask-smorest
- flask-swagger-ui
- marshmallow

The resource abstractor could be written in another language, just by exposing the same endpoints and database API.