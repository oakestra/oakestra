from setuptools import find_packages, setup

setup(
    name="resource_abstractor_client",
    version="0.2.0",
    packages=find_packages(),
    include_package_data=True,
    # httpx/attrs/typing-extensions are what resource_abstractor_client/openapi/
    # (generated from go_resource_abstractor/openapi/openapi.yaml - see the module's
    # README) imports; typing-extensions backports typing.Self, added in Python 3.11,
    # for the 3.10 runtime this package targets (see the Dockerfiles).
    # Not listed: oakestra_utils, imported by job_operations.py for the Status enum -
    # satisfied only because consumers separately install
    # libraries/oakestra_utils_library alongside this package (see the Dockerfiles and
    # CI in root_orchestrator/system-manager-python and cluster_orchestrator/cluster-manager).
    install_requires=["httpx>=0.23.0,<1.0", "attrs>=22.2.0", "typing-extensions>=4.0.0"],
    python_requires=">=3.10",
)
