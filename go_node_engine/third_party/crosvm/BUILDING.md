# Building crosvm for Oakestra

This file describes how to build *crosvm* to use it in *Oakestra*.


## Python Setup

The `go_node_engine/third_party/crosvm` directory contains a `requirements.txt`
file with the dependencies needed to run *crosvm* development scripts.
It also contains a `.python-version` file with the recommended Python version to use.
If you are using *pyenv* it will automatically pickup this version.

It's recommended to create a virtual environment to run the *crosvm* development scripts:
1. `$ cd go_node_engine/third_party/crosvm`
2. `$ python -m venv .venv`
3. `$ source .venv/bin/activate`
4. `$ pip install -r requirements.txt`


## Compiling crosvm in a Development Container

Change into the directory of the *crosvm* repository: `$ cd go_node_engine/third_party/crosvm/crosvm`

> [!NOTE]  
> In the next step a development container will be started using either Podman or Docker.
> If your operating system is using SELinux (e.g. Fedora), you need to adjust the `go_node_engine/third_party/crosvm/crosvm/tools/dev_container` script:
> In the `workspace_mount_args` function, add the 'Z' flag to the volume mount, e.g. `f"--volume {quoted(CROSVM_ROOT)}:/workspace:rw,Z"`.

Run `$ ./tools/dev_container cargo build --release --target-dir /workspace/target -p crosvm_control` to start a *crosvm* development container and compile crosvm inside of it.
The build output will be located at `go_node_engine/third_party/crosvm/crosvm/target`.
You can now stop the development container again with `$ ./tools/dev_container --stop`.

Unfortunately, the *C* header file needed to compile against *crosvm* is now located in an intermediate build directory.
To copy it to the final build output directory run `$ cp ./target/release/build/crosvm_control-*/out/crosvm_control.h ./target/release`

