# Marketplace manager

## Ingoing Endpoints

- `/marketplace/addons`: Endpoint for managing addons.

For more details checkout out the [openapi.json](./api/v1/openapi.json)


## Start Marketplace manager.

Run the service by running 

```
$ docker build -t marketplace_manager .
$ docker run marketplace_manager -p 11102:11102
```

## DB

Marketplace manager is dependent on mongodb via these env variables:
- ADDON_MARKETPLACE_MONGO_URL
- ADDON_MARKETPLACE_MONGO_PORT

The default for both are 'localhost' & '1007' respectively. These are defined in the dockerfile, so either adjust the dockerfile or run the image with different envs.


## Example:

The following addon contains two dummy services/containers where one can run successfuly but the other fails.

```json
{
  "name": "testing-addon",
  "services": [
    {
      "command": "sh -c 'while true; do echo \"Hello, World (testing!!) !\"; sleep 10; done'",
      "image": "alpine",
      "service_name": "my-alpine"
    },
    {
      "command": "false",
      "image": "busybox",
      "service_name": "my-busybox-fail"
    }
  ]
}
```
