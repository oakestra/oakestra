# the mongo version to build, e.g. "8.0.4"
ARG MONGO_VERSION


# use debian image for compilation of mongo
FROM --platform=$BUILDPLATFORM debian:12.9-slim AS builder

# the number of jobs to use during compilation of mongo, the higher it is, the more memory and compute is needed
ARG NINJA_JOBS
# redeclare mongo version so it can be used in the builder image
ARG MONGO_VERSION

# set default shell for RUN commands to bash, so that bash-specific features can be used
SHELL ["/bin/bash", "-c"]

# add arm64 architecture and update all packages
RUN dpkg --add-architecture arm64 && apt-get -y update && apt-get -y upgrade
# install packages needed for compilation
RUN apt-get -y install git lld ccache icecc crossbuild-essential-arm64 libssl-dev:arm64 libcurl4-openssl-dev:arm64 \
      python3-dev python-dev-is-python3 python3-venv python3-poetry
# allow the arm64/aarch64 cross toolchain to use lld without needing to specify the absolute path everytime
RUN ln -s /usr/bin/lld /usr/bin/aarch64-linux-gnu-ld.lld

# do a minimal clone of the mongo repository at the specified version and make it the working directory
RUN git clone --depth 1 --branch r${MONGO_VERSION} https://github.com/mongodb/mongo.git /mongodb
WORKDIR /mongodb

# create python venv and install dependencies
RUN python3 -m venv python3-venv --prompt mongo \
  && source python3-venv/bin/activate \
  && poetry install --no-root --sync

# compile mongo server components:
# - the truncate command is to disable a custom toolchain path that mongo tries to use by default
# - this is what makes these binaries work on the RPI-4b:
#   - 'CCFLAGS="-march=armv8-a+crc -moutline-atomics -mtune=cortex-a72"': compilation flags for the RPI-4b CPU
#   - '--allocator=system': the default allocator of mongo (tcmalloc) does not work on the RPI-4b (https://github.com/google/tcmalloc/issues/82)
RUN source python3-venv/bin/activate \
  && truncate -s 0 etc/scons/mongodbtoolchain_stable_gcc.vars \
  && python buildscripts/scons.py AR=/usr/bin/aarch64-linux-gnu-ar CC=/usr/bin/aarch64-linux-gnu-gcc CXX=/usr/bin/aarch64-linux-gnu-g++ CCFLAGS="-march=armv8-a+crc -moutline-atomics -mtune=cortex-a72" --build-profile=release --variables-files=etc/scons/developer_versions.vars --allocator=system --disable-warnings-as-errors install-servers \
  && ninja -f release.ninja -j ${NINJA_JOBS} install-servers \
  && aarch64-linux-gnu-strip build/install/bin/{mongod,mongos}


# create final image based on specified mongo version
FROM --platform=linux/arm64/v8 mongo:$MONGO_VERSION

# replace mongo executables with the custom built ones
COPY --from=builder /mongodb/build/install/bin/mongod /usr/bin/mongod
COPY --from=builder /mongodb/build/install/bin/mongos /usr/bin/mongos

LABEL org.opencontainers.image.source="https://github.com/oakestra/oakestra"
