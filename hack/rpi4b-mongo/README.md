# MongoDB Image for the Raspberry Pi 4 Model B
The [Dockerfile](Dockerfile) in this directory provides a custom MongoDB image built specifically for the Raspberry Pi 4 Model B (ARMv8-A).
The standard MongoDB ARM container targets ARMv8.2-A or newer, which the RPI-4b does not support. 
By cross-compiling MongoDB with the appropriate CPU flags and using the system allocator,
this build produces a fully compatible image that runs on RPI-4b hardware.

> [!WARNING]  
> This image is experimental; please raise issues for any bugs you encounter.

## Usage 
You can use this image as a drop-in replacement for the official MongoDB container in
Oakestra’s root and cluster orchestrators when deploying to RPI-4b devices.

Currently, no pre-built images are published and you will need to build one yourself.
When pre-built versions become available, it will be documented here.

## Building
The build process in the Dockerfile is designed for cross-compilation: It will always target ARM,
but you should be able to build it independent of your host architecture.

When building the image, you need to specify the following build arguments:
- `MONGO_VERSION`: the version of MongoDB to build; only full version triplets are supported (e.g. __*8.0.4*__)
- `NINJA_JOBS`: the number of jobs to use during compilation (e.g. __*16*__)

An example of a full build command to be run from inside this directory:
```shell
docker build \
  --build-arg MONGO_VERSION=8.0.4 \
  --build-arg NINJA_JOBS=16 \
  --tag oakestra/rpi4b-mongo:8.0.4 \
  .
```
