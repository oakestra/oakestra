FROM debian:12.10-slim AS build

ARG GPU_KERNEL_DRIVER=AMDGPU

# set default shell for RUN commands to bash, so that bash-specific features can be used
SHELL ["/bin/bash", "-c"]

# install dependencies
RUN apt-get update && apt-get --yes upgrade && apt-get install --yes --no-install-recommends \
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

# install rustup for root user
RUN curl --proto '=https' --tlsv1.2 --silent --show-error --fail --location https://sh.rustup.rs \
        | sh -s -- --profile minimal --default-toolchain 1.81.0 -y

# create output directory for build artifacts
RUN mkdir --parents /opt/crosvm /opt/crosvm/bin /opt/crosvm/include /opt/crosvm/lib /opt/crosvm/lib/pkgconfig /opt/crosvm/libexec /opt/crosvm/share /opt/crosvm/share/edk2

# build minigbm (maybe a specific branch or commit should be used)
# NOTE: minigbm does not have a prefix option separate from destdir, so we manually override it at the end
RUN git clone --depth 1 https://chromium.googlesource.com/chromiumos/platform/minigbm.git /scratch/minigbm \
    && cd /scratch/minigbm \
    && sed --in-place 's/-Wxor-used-as-pow/-Wno-xor-used-as-pow/g' common.mk \
    && make CPPFLAGS="-DDRV_${GPU_KERNEL_DRIVER}" DRV_${GPU_KERNEL_DRIVER}=1 OUT=out DESTDIR=/opt/crosvm INCLUDEDIR=include LIBDIR=lib install

# build virglrenderer (maybe a specific branch or commit should be used, e.g. "1.1.0")
RUN git clone --depth 1 https://gitlab.freedesktop.org/virgl/virglrenderer.git /scratch/virglrenderer \
    && cd /scratch/virglrenderer \
    && meson setup --pkg-config-path /opt/crosvm/lib/pkgconfig --prefix /opt/crosvm --includedir include --libdir lib --libexecdir libexec -D venus=true -D minigbm_allocation=true -D unstable-apis=true out \
    && meson compile -C out \
    && meson install -C out

# build crosvm (maybe a specific commit should be used)
# NOTE: libcrosvm_control.so has its SONAME set to exactly "libcrosvm_control.so",
#       so there's no need to create versioned symlinks
RUN git clone --depth 1 --recursive https://chromium.googlesource.com/crosvm/crosvm.git /scratch/crosvm \
    && cd /scratch/crosvm \
    && source /root/.cargo/env \
    && LD_LIBRARY_PATH=/opt/crosvm/lib PKG_CONFIG_PATH=/opt/crosvm/lib/pkgconfig CROSVM_USE_SYSTEM_MINIGBM=1 CROSVM_USE_SYSTEM_VIRGLRENDERER=1 cargo build --release --package crosvm --package crosvm_control --features 'virgl_renderer' \
    && install --mode 0755 target/release/crosvm /opt/crosvm/bin \
    && install --mode 0644 target/release/build/crosvm_control-*/out/crosvm_control.h /opt/crosvm/include \
    && install --mode 0644 target/release/libcrosvm_control.so /opt/crosvm/lib

# download crosvm specific edk2 patches (maybe a specific commit should be used)
RUN mkdir --parents /scratch/edk2-crosvm \
    && curl --proto '=https' --tlsv1.2 --silent --show-error --fail --location https://chromium.googlesource.com/chromiumos/overlays/chromiumos-overlay/+archive/refs/heads/main/sys-firmware/edk2-ovmf-crosvm.tar.gz \
        | tar --extract --gzip --file - --directory /scratch/edk2-crosvm

# clone edk2 at the tag that the crosvm patches are based on and build it
RUN export EDK_TAG=$(grep --perl-regexp --only-matching 'edk2-.+(?=\.tar\.gz)' /scratch/edk2-crosvm/Manifest) \
    && git clone --recursive --depth 1 --branch ${EDK_TAG} https://github.com/tianocore/edk2.git /scratch/edk2 \
    && cd /scratch/edk2 \
    && git apply /scratch/edk2-crosvm/files/* \
    && make --directory BaseTools \
    && source edksetup.sh \
    && build --platform OvmfPkg/Crosvm/CrosvmX64.dsc --tagname GCC5 --arch X64 --buildtarget RELEASE --define SECURE_BOOT_ENABLE --define FD_SIZE_4MB \
    && install --mode 0644 Build/CrosvmX64/RELEASE_GCC5/FV/CROSVM_{CODE,VARS}.fd /opt/crosvm/share/edk2


FROM scratch

# copy build artifacts to the otherwise empty root directory, so it can be used with the --output flag of the docker build command
# NOTE: copying the subdirectories of /opt/crosvm in one COPY instruction does not work correctly for some reason
COPY --chown=0:0 --chmod=0755 crosvm-wrapper.sh /bin/crosvm-wrapper.sh
COPY --from=build /opt/crosvm/bin /bin
COPY --from=build /opt/crosvm/include /include
COPY --from=build /opt/crosvm/lib /lib
COPY --from=build /opt/crosvm/libexec /libexec
COPY --from=build /opt/crosvm/share /share
