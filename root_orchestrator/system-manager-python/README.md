# System Manager

The System Manager is one of the components of the Root Orchestrator. It has an overview of all participating clusters and communicates with the Root Scheduler to decide a task placement.

## Purpose and Tasks of System Manager

- It maintains the database: add cluster, remove cluster, count up/down workers within clusters.
- Users can ask the System Manager to deploy a service within the system. The System Manager gives feedback about the status of a job.
- Users can also ask the System Manager to move/migrate a service to another cluster or to another worker node as well as terminate a job, or replicate (scale up, scale down) a job

### Ingoing Endpoints

- provides `/register` endpoint for clusters to register into the system
- provides endpoints to count up / count down the number of participating worker nodes within a cluster
- provides endpoints to move, replicate, terminate a service

### Outgoing Endpoints

- asks Scheduler to calculate placement for a job
- propagates scheduler decision to the selected cluster

## Start the System Manager

Please start the system-manager with `./start-up.sh`.

A virtualenv will be started and the component will start up.

For how to use the endpoints, look in `gitlab.lrz.de/cm/2020-mehdi-masters-thesis/src/README.md`. 

## Testing

The suite needs no running containers. Run it from this directory:

```bash
pip install -r requirements.txt -r requirements-test.txt
pip install ../../libraries/resource_abstractor_client ../../libraries/oakestra_utils_library
python -m grpc_tools.protoc -I. --python_out=. --grpc_python_out=. proto/clusterRegistration.proto
pytest
```

- `tests/unit/` tests single modules in isolation.
- `tests/component/` boots the real system manager (Flask app plus the gRPC registration server) and drives it only through its REST and gRPC interfaces. All external dependencies are replaced at the network boundary (see `tests/component/conftest.py`):
  - MongoDB is `mongomock`.
  - The resource-abstractor is a stateful in-memory fake (`tests/component/fakes/resource_abstractor.py`) that mirrors the real API. If you change the resource-abstractor contract, update the fake in the same PR.
  - The root scheduler, network plugin and cluster managers are recording stubs. Tests assert on the requests they received through the `outbound` fixture.
  - Tokens come from the real `jwt-generator` app, loaded in-process.

  Any HTTP request to an address no fake answers fails the test, so the suite never touches the network.
- Known production bugs are pinned as `xfail(strict=True)` tests. When you fix one, its test starts passing and fails the run until you remove the marker.
- `tests/component/test_api_surface.py` compares the public routes, and whether they require a token, against `tests/component/snapshots/api_surface.json`. After an intentional API change, regenerate it with `UPDATE_SNAPSHOTS=1 pytest tests/component/test_api_surface.py` and commit the diff.

## Built With

Python3.10
  - bson
  - Flask
  - Flask_PyMongo
  - Flask-SocketIO
  - eventlet
  - PyYAML
  - requests
  - Cerberus

The System Manager could be written in another programming language as well. Just the endpoints, protocols, and database API should be supported by the language.

## Some further Thoughts

- Very interesting (=weird) behaviour: Pycharm 2020.1 reports `socketio.exceptions.BadNamespaceError: /init is not a connected namespace.` when a socketIO client wants to emit a Websocket message to an already connected SocketIO server. But in Pycharm2020.3 it works fine. Not examined further, but, probably, different versions of Python packages are installed on both.
- Not sure if it is a Pycharm Issue, because when inserting time.sleep() instructions before/after socketIO operations, it seems to work.
