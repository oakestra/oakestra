FROM debian:12.10-slim AS build

ARG GPU_KERNEL_DRIVER=AMDGPU

# install dependencies
RUN apt-get update && apt-get --yes upgrade && apt-get install --yes --no-install-recommends \
        ca-certificates \
        curl \
        build-essential \
        git \
        pkg-config \
        cmake \
        meson \
        python3-yaml \
        libepoxy-dev \
        libdrm-dev \
        libvulkan-dev \
        mesa-common-dev \
        libwayland-dev \
        libcap-dev \
        libclang-dev

# install rustup for root
RUN curl --proto '=https' --tlsv1.2 -sSf https://sh.rustup.rs \
    | sh -s -- --profile minimal --default-toolchain 1.81.0 -y

# build minigbm (maybe a specific branch or commit should be used)
RUN git clone --depth 1 https://chromium.googlesource.com/chromiumos/platform/minigbm.git /scratch/minigbm \
    && cd /scratch/minigbm \
    && sed -i 's/-Wxor-used-as-pow/-Wno-xor-used-as-pow/g' common.mk \
    && make CPPFLAGS="-DDRV_${GPU_KERNEL_DRIVER}" DRV_${GPU_KERNEL_DRIVER}=1 DESTDIR=/usr INCLUDEDIR=include LIBDIR=lib/x86_64-linux-gnu install

# build virglrenderer (maybe a specific branch or commit should be used, e.g. "1.1.0")
RUN git clone --depth 1 https://gitlab.freedesktop.org/virgl/virglrenderer.git /scratch/virglrenderer \
    && cd /scratch/virglrenderer \
    && meson setup out --prefix /usr --includedir include --libdir lib/x86_64-linux-gnu -D venus=true -D minigbm_allocation=true -D unstable-apis=true \
    && meson compile -C out \
    && meson install -C out

# build crosvm (maybe a specific commit should be used)
RUN . /root/.cargo/env \
    && git clone --depth 1 --recursive https://chromium.googlesource.com/crosvm/crosvm.git /scratch/crosvm \
    && cd /scratch/crosvm \
    && CROSVM_USE_SYSTEM_MINIGBM=1 CROSVM_USE_SYSTEM_VIRGLRENDERER=1 cargo build --release --package crosvm --package crosvm_control --features 'virgl_renderer' \
    && install --mode 0755 target/release/crosvm /usr/bin/crosvm

# prepare output directories
RUN mkdir -p /scratch/bin/ /scratch/include/ /scratch/lib/ /scratch/lib/pkgconfig/ /scratch/libexec/ \
    && cp /usr/bin/crosvm /scratch/bin/ \
    && cp -r /usr/include/virgl/ /scratch/include/virgl/ \
    && cp /usr/lib/x86_64-linux-gnu/libvirglrenderer.so.1.9.1 /scratch/lib/ \
    && ln -s libvirglrenderer.so.1.9.1 /scratch/lib/libvirglrenderer.so.1 \
    && ln -s libvirglrenderer.so.1 /scratch/lib/libvirglrenderer.so \
    && cp /usr/lib/x86_64-linux-gnu/pkgconfig/virglrenderer.pc /scratch/lib/pkgconfig/ \
    && cp /usr/libexec/virgl_render_server /scratch/libexec/ \
    && cp /usr/include/gbm.h /scratch/include/ \
    && cp /usr/lib/x86_64-linux-gnu/libminigbm.so.1.0.0 /scratch/lib/ \
    && ln -s libminigbm.so.1.0.0 /scratch/lib/libgbm.so.1.0.0 \
    && ln -s libminigbm.so.1.0.0 /scratch/lib/libgbm.so.1 \
    && ln -s libminigbm.so.1.0.0 /scratch/lib/libgbm.so \
    && cp /usr/lib/x86_64-linux-gnu/pkgconfig/gbm.pc /scratch/lib/pkgconfig/


FROM scratch

COPY --chown=0:0 --chmod=0755 crosvm-wrapper.sh /bin/crosvm-wrapper.sh
COPY --from=build /scratch/bin/ /bin/
COPY --from=build /scratch/include/ /include/
COPY --from=build /scratch/lib/ /lib/
COPY --from=build /scratch/libexec/ /libexec/

CMD ["/bin/crosvm-wrapper.sh"]
