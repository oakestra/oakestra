# *crosvm* third-party dependency


*crosvm* is a virtual machine monitor that specializes in paravirtualization (no foreign architecture emulation).
It implements the most cutting-edge standards when it comes to GPU virtualization with *virtio*.

Oakestra uses it to provide a virtualization runtime with more capabilities (but potentially more overhead) than containers.

## Building

To build *crosvm* for Oakestra, a [*Dockerfile*](./Dockerfile) is provided that builds *crosvm* and some of its dependencies,
namely [*minigbm*](https://chromium.googlesource.com/chromiumos/platform/minigbm/) and [*virglrenderer*](https://gitlab.freedesktop.org/virgl/virglrenderer)
for GPU virtualization and [*EDK II*](https://github.com/tianocore/edk2) for UEFI support.

The built image contains only build artifacts of the mentioned projects and represents no runnable container.
It is intended to be used with the `--output` argument of the `docker build` command to move the build artifacts
to the `/opt/oakestra` directory of the host system:
1) `$ cd <PROJECT_ROOT>/go_node_engine/third_party/crosvm`
2) `$ mkdir out`
3) `$ docker build --output out`
4) `$ sudo mkdir --parents /opt/oakestra`
5) `$ sudo chown 0755 /opt/oakestra`
6) `$ sudo mv out/* /opt/oakestra`
7) `$ sudo chown --recursive root:root /opt/oakestra`
8) `$ rm --recursive out`

Even with these custom-built artifacts present in `/opt/oakestra`, to actually run crosvm, some additional dynamic libraries
have to be present on the system: 
- *libvulkan*
- *libdrm*
- *libepoxy*
- *libcap*

If you want to recompile *crosvm* on your host (e.g. to debug it in an IDE), you also need these libraries/tools:
- *libclang*
- *wayland-scanner*

> [!NOTE]  
> These lists might be incomplete, if you find crosvm not starting on your system, please open an issue.

On *Ubuntu*/*Debian*, these are the *APT* packages corresponding to both lists combined:
- *libvulkan-dev*
- *libdrm-dev*
- *libepoxy-dev*
- *libcap-dev*
- *libclang-dev*
- *libwayland-dev*

For *Fedora*, these are the equivalent *DNF* packages:
- *vulkan-loader-devel*
- *libdrm-devel*
- *libepoxy-devel*
- *libcap-devel*
- *clang-devel*
- *wayland-devel*
