FROM debian:12.10-slim AS build

ARG GPU_KERNEL_DRIVER=AMDGPU

# install dependencies
RUN <<-EOT bash
    set -euo pipefail

    export DEBIAN_FRONTEND=noninteractive
    export DEBCONF_NOWARNINGS=yes
    apt-get update
    apt-get upgrade --yes
    apt-get install --yes --no-install-recommends \
        ca-certificates \
        curl \
        git \
        cmake \
        meson \
        pkg-config \
        nasm \
        iasl \
        python3 \
        python-is-python3 \
        python3-yaml \
        build-essential \
        libepoxy-dev \
        libdrm-dev \
        libvulkan-dev \
        libwayland-dev \
        libcap-dev \
        libclang-dev \
        uuid-dev \
        mesa-common-dev
EOT

# install rustup for root user
RUN <<-EOT bash
    set -euo pipefail

    curl --proto '=https' --tlsv1.2 --silent --show-error --fail --location https://sh.rustup.rs \
        | sh -s -- --profile minimal --default-toolchain 1.81.0 -y
EOT

# create output directory for build artifacts
RUN mkdir --parents \
        /opt/oakestra \
        /opt/oakestra/bin \
        /opt/oakestra/include \
        /opt/oakestra/lib \
        /opt/oakestra/lib/pkgconfig \
        /opt/oakestra/libexec

# build minigbm (maybe a specific branch or commit should be used)
# NOTE: minigbm does not have a prefix option separate from destdir, so we manually override it at the end
RUN <<-EOT bash
    set -euo pipefail

    git clone --depth 1 https://chromium.googlesource.com/chromiumos/platform/minigbm.git /scratch/minigbm
    cd /scratch/minigbm
    sed --in-place 's/-Wxor-used-as-pow/-Wno-xor-used-as-pow/g' common.mk
    make \
        CPPFLAGS="-DDRV_${GPU_KERNEL_DRIVER}" DRV_${GPU_KERNEL_DRIVER}=1 \
        OUT=out \
        DESTDIR=/opt/oakestra \
        INCLUDEDIR=include \
        LIBDIR=lib \
        install
EOT

# build virglrenderer (maybe a specific branch or commit should be used, e.g. "1.1.0")
RUN <<-EOT bash
    set -euo pipefail

    git clone --depth 1 https://gitlab.freedesktop.org/virgl/virglrenderer.git /scratch/virglrenderer
    cd /scratch/virglrenderer
    meson setup \
        --pkg-config-path /opt/oakestra/lib/pkgconfig \
        --prefix /opt/oakestra \
        --includedir include \
        --libdir lib \
        --libexecdir libexec \
        -D venus=true -D minigbm_allocation=true -D unstable-apis=true \
        out
    meson compile -C out
    meson install -C out
EOT

# the version of the rutabaga_gfx crate that crosvm depends on (v0.1.71) has a bug that makes it not compile:
# https://github.com/magma-gpu/rutabaga_gfx/issues/15
#
# the step below patches bug locally and the crosvm build then uses it instead of the upstream version
RUN <<-EOT bash
    set -euo pipefail

    git clone --depth 1 --recursive --branch v0.1.71 https://github.com/magma-gpu/rutabaga_gfx.git /scratch/rutabaga_gfx
    cd /scratch/rutabaga_gfx
    sed -i '/use mesa3d_util::MESA_HANDLE_TYPE_MEM_SHM;/a use mesa3d_util::MESA_HANDLE_TYPE_SIGNAL_SYNC_FD;' \
        src/virgl_renderer.rs
EOT

# build crosvm (maybe a specific commit should be used)
# NOTE: libcrosvm_control.so has its SONAME set to exactly "libcrosvm_control.so",
#       so there's no need to create versioned symlinks
RUN <<-EOT bash
    set -euo pipefail

    git clone --depth 1 --recursive https://chromium.googlesource.com/crosvm/crosvm.git /scratch/crosvm
    cd /scratch/crosvm
    . /root/.cargo/env

	sed -i '/minijail = { path = "third_party\/minijail\/rust\/minijail" }/a rutabaga_gfx = { path = "/scratch/rutabaga_gfx" }' \
        Cargo.toml

    export LD_LIBRARY_PATH=/opt/oakestra/lib
    export PKG_CONFIG_PATH=/opt/oakestra/lib/pkgconfig
    export CROSVM_USE_SYSTEM_MINIGBM=1
    export CROSVM_USE_SYSTEM_VIRGLRENDERER=1
    cargo build --release --package crosvm --package crosvm_control --features 'virgl_renderer'

    install --mode 0755 target/release/crosvm /opt/oakestra/bin
    install --mode 0644 target/release/build/crosvm_control-*/out/crosvm_control.h /opt/oakestra/include
    install --mode 0644 target/release/libcrosvm_control.so /opt/oakestra/lib
EOT

FROM scratch

# copy build artifacts to the otherwise empty root directory, so it can be used with the --output flag of the docker build command
# NOTE: copying the subdirectories of /opt/oakestra in one COPY instruction does not work correctly for some reason
COPY --from=build /opt/oakestra/bin /bin
COPY --from=build /opt/oakestra/include /include
COPY --from=build /opt/oakestra/lib /lib
COPY --from=build /opt/oakestra/libexec /libexec
COPY --chown=0:0 --chmod=0755 crosvm-wrapper.sh /bin/crosvm-wrapper.sh
COPY --chown=0:0 --chmod=0644 crosvm_control.pc /lib/pkgconfig/crosvm_control.pc
