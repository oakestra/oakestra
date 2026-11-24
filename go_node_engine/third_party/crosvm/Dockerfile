# hadolint global ignore=SC3040,DL4006,DL3003
FROM debian:trixie-20261027-slim AS build

ENV PATH="/root/.cargo/bin:${PATH}"

# Install dependencies.
RUN <<-'EOT' bash
    set -euo pipefail

    export DEBIAN_FRONTEND=noninteractive

    packages=(
        # common
        ca-certificates
        curl
        git

        # virglrenderer
        build-essential
        cmake
        pkgconf
        meson
        libdrm-dev
        python3-yaml

        # crosvm
        libwayland-dev
        libcap-dev
        libclang-dev
    )

    apt-get update
    apt-get install --yes --no-install-recommends "${packages[@]}"

    rm -rf /var/lib/apt/lists/*
EOT

# Install rustup with the exact toolchain and components requested by crosvm's rust-toolchain file.
RUN <<-'EOT' bash
    set -euo pipefail

    curl \
        --proto '=https' \
        --tlsv1.2 \
        --silent \
        --show-error \
        --fail \
        --location https://sh.rustup.rs \
      | sh -s -- --profile minimal --default-toolchain "1.88.0" -y

    rustup component add --toolchain "1.88.0" \
        rustfmt \
        clippy \
        llvm-tools-preview
    rustup set auto-install disable
EOT

# Build artifacts are installed into this prefix and exported by the final scratch stage.
RUN mkdir --parents \
    /opt/oakestra/bin \
    /opt/oakestra/include \
    /opt/oakestra/lib \
    /opt/oakestra/lib/pkgconfig \
    /opt/oakestra/libexec

# Build virglrenderer with only the AMDGPU DRM native-context.
RUN <<-'EOT' bash
    set -euo pipefail

    git clone \
        --depth 1 \
        --branch "drm-amdgpu-no-gallium" \
        https://gitlab.freedesktop.org/axiphi/virglrenderer.git \
        /scratch/virglrenderer

    cd /scratch/virglrenderer

    meson setup out \
        --prefix /opt/oakestra \
        --includedir include \
        --libdir lib \
        --libexecdir libexec \
        --pkg-config-path /opt/oakestra/lib/pkgconfig \
        -Dvrend=false \
        -Ddrm-renderers=amdgpu-experimental \
        -Dunstable-apis=true

    meson compile -C out -j "$(nproc)"
    meson install -C out
EOT

# Build crosvm with virtio-gpu display support and the virglrenderer backend.
# The rutabga version is changed to the locally cloned directory above.
RUN <<-'EOT' bash
    set -euo pipefail

    git clone \
        --recursive \
        --depth 1 \
        --branch "gpu-displayless-mode" \
        https://github.com/axiphi/crosvm.git \
        /scratch/crosvm

    cd /scratch/crosvm

    # rutabaga_gfx doesn't compile with with the virgl_renderer_unstable feature under version 0.1.80.
    sed -i 's/rutabaga_gfx = "=0.1.80"/rutabaga_gfx = "=0.1.85"/' Cargo.toml

    export LD_LIBRARY_PATH=/opt/oakestra/lib
    export PKG_CONFIG_PATH=/opt/oakestra/lib/pkgconfig
    export CROSVM_USE_SYSTEM_VIRGLRENDERER=1

    cargo build \
        --release \
        --package crosvm \
        --package crosvm_control \
        --no-default-features \
        --features 'balloon,qcow,net,gpu,virgl_renderer'

    install --mode 0755 target/release/crosvm /opt/oakestra/bin/crosvm
    install --mode 0644 target/release/build/crosvm_control-*/out/crosvm_control.h /opt/oakestra/include/crosvm_control.h
    install --mode 0644 target/release/libcrosvm_control.so /opt/oakestra/lib/libcrosvm_control.so
EOT


FROM scratch

# Export-only stage for use with `docker build --output`.
# The directories are created with separate COPY operations,
# because merging them into a single COPY does not preserve the desired output layout with this stage.
COPY --from=build /opt/oakestra/bin /bin
COPY --from=build /opt/oakestra/include /include
COPY --from=build /opt/oakestra/lib /lib
COPY --from=build /opt/oakestra/libexec /libexec

COPY --chown=0:0 --chmod=0755 crosvm-wrapper.sh /bin/crosvm-wrapper.sh
COPY --chown=0:0 --chmod=0644 crosvm_control.pc /lib/pkgconfig/crosvm_control.pc
