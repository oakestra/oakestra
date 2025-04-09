# Notes on crosvm dependency

## Dependencies

In addition to the custom built *virglrender* and *minigbm* libraries, crosvm needs the following libraries installed to compile:
- *libvulkan*
- *libdrm*
- *libepoxy*
- *libcap*
- *libclang*
- *wayland-scanner*

On Fedora, this means the following packages need to be installed:
- *vulkan-loader-devel*
- *libdrm-devel*
- *libepoxy-devel*
- *libcap-devel*
- *clang-devel*
- *wayland-devel*
