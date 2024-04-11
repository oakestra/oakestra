# Quick Startup Scripts

## Start Oakestra 1DOC 

This script can be used to quickly setup a 1-DOC cluster

- (optional) setup a repository branch e.g., `export OAKESTRA_BRANCH=develop`, default branch is `main`.
- (optional) setup comma-separated list of custom override files for docker compose e.g., `export OVERRIDE_FILES=override-alpha-versions.yaml`
- Download setup and startup the 1-DOC cluster managers simply running:
```
curl -sfL https://raw.githubusercontent.com/oakestra/oakestra/develop/scripts/StartOakestraFull.sh | sh - 
```

## Start Oakestra Root Standalone

This script can be used to quickly setup a full Root Orchestrator

- (optional) setup a repository branch e.g., `export OAKESTRA_BRANCH=develop`, default branch is `main`.
- (optional) setup comma-separated list of custom override files for docker compose e.g., `export OVERRIDE_FILES=override-alpha-versions.yaml`
- Download setup and startup the root orchestrator simply running:
```
curl -sfL https://raw.githubusercontent.com/oakestra/oakestra/develop/scripts/StartOakestraRoot.sh | sh - 
```

## Start Oakestra Cluster

If you already have a 1-DOC setup or a standalone Root orchestrator you can use the following script to setup a new cluster on a machine that yet does not have a cluster orchestrator:

- (optional) setup a repository branch e.g., `export OAKESTRA_BRANCH=develop`, default branch is `main`.
- (optional) setup comma-separated list of custom override files for docker compose e.g., `export OVERRIDE_FILES=override-alpha-versions.yaml`
- (optional) setup a custom cluster location e.g., `export CLUSTER_LOCATION=<latitude>,<longitude>,<radius>`, default location is automatically inferred from the public IP address of the machine. 
- Choose a cluster name: 
`export CLUSTER_NAME=my_awesome_cluster`
- Set the URL or IP of the root orchestrator:
`export SYSTEM_MANAGER_URL=<url or ip>`
- Startup the cluster orchestrator with:
```
curl -sfL https://raw.githubusercontent.com/oakestra/oakestra/develop/scripts/StartOakestraCluster.sh | sh - 
```

## Install Worker and Node Engine
- (optional) select a custom version e.g.: `export OAKESTRA_VERSION=alpha` for the latest alpha version or `export OAKESTRA_VERSION=alpha-v0.4.203` for a specific version. Default value: latest stable. 
- Install the binaries using
```
curl -sfL https://raw.githubusercontent.com/oakestra/oakestra/develop/scripts/InstallOakestraWorker.sh | sh - 
```
- Run the binaries as usual