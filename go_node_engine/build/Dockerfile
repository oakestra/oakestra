FROM docker.io/library/golang:1.24.3-bookworm

ARG VERSION

COPY ./ /scratch/oakestra/
COPY --from=localhost/oakestra-dev/third-party:latest / /opt/oakestra/

RUN apt-get update \
    && apt-get --yes upgrade \
    && apt-get --yes install --no-install-recommends \
        libvulkan-dev \
        libdrm-dev \
        libepoxy-dev \
        libcap-dev \
        libclang-dev \
        libwayland-dev

CMD "/bin/bash"

#RUN cd /scratch/oakestra/build \
#    && GOOS=linux GOARCH=amd64 go build \
#        -ldflags="-X 'go_node_engine/cmd.Version=${VERSION}'" \
#        -o NodeEngine_amd64 \
#        ../NodeEngine.go \
#    && GOOS=linux GOARCH=amd64 go build \
#        -ldflags="-X 'go_node_engine/cmd.Version=${VERSION}'" \
#        -o nodeengined_amd64 \
#        ../internal/daemon/nodeengined.go
