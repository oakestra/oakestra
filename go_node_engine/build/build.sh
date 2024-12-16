#!/usr/bin/env bash
version=$(git describe --tags --abbrev=0)

# Removing existing binaries for wasmtime if any
rm -rf /tmp/wasmtime-v25
rm -rf /tmp/wasmtime-v27

echo "Downloading wasmtime library for amd64"

# Download wasmtime-go library to /usr/local/lib/wasmtime-go for dynamic linking (depending on the architecture)
wget -nv https://github.com/bytecodealliance/wasmtime/releases/download/v25.0.0/wasmtime-v25.0.0-x86_64-linux-c-api.tar.xz -O /tmp/wasmtime-v25.0.0-x86_64-linux-c-api.tar.xz
tar -xf /tmp/wasmtime-v25.0.0-x86_64-linux-c-api.tar.xz -C /tmp

export LD_LIBRARY_PATH=/tmp/wasmtime-v25.0.0-x86_64-linux-c-api:$LD_LIBRARY_PATH
export CGO_CFLAGS="-I/tmp/wasmtime-v25.0.0-x86_64-linux-c-api/include"
export CGO_LDFLAGS="-L/tmp/wasmtime-v25.0.0-x86_64-linux-c-api/lib"
export CC=gcc

echo "Building for amd64"
env CGO_ENABLED=1 GOOS=linux GOARCH=amd64 go build -ldflags="-X 'go_node_engine/cmd.Version=$version'" -o NodeEngine_amd64 ../NodeEngine.go
env CGO_ENABLED=1 GOOS=linux GOARCH=amd64 go build -ldflags="-X 'go_node_engine/cmd.Version=$version'" -o nodeengined_amd64 ../pkg/nodeengined.go
echo "Building for amd64 done"

echo "Downloading wasmtime library for arm64"
wget -nv https://github.com/bytecodealliance/wasmtime/releases/download/v25.0.0/wasmtime-v25.0.0-aarch64-linux-c-api.tar.xz -O /tmp/wasmtime-v25.0.0-aarch64-linux-c-api.tar.xz
tar -xf /tmp/wasmtime-v25.0.0-aarch64-linux-c-api.tar.xz -C /tmp

export LD_LIBRARY_PATH=/tmp/wasmtime-v25.0.0-aarch64-linux-c-api:$LD_LIBRARY_PATH
export CGO_CFLAGS="-I/tmp/wasmtime-v25.0.0-aarch64-linux-c-api/include"
export CGO_LDFLAGS="-L/tmp/wasmtime-v25.0.0-aarch64-linux-c-api/lib"
export CC=aarch64-linux-gnu-gcc

echo "Building for arm64"
env CGO_ENABLED=1 GOOS=linux GOARCH=arm64 go build -ldflags="-X 'go_node_engine/cmd.Version=$version'" -o NodeEngine_arm64 ../NodeEngine.go
env CGO_ENABLED=1 GOOS=linux GOARCH=arm64 go build -ldflags="-X 'go_node_engine/cmd.Version=$version'" -o nodeengined_arm64 ../pkg/nodeengined.go

echo "Building for arm64 done"

unset LD_LIBRARY_PATH
unset CGO_CFLAGS
unset CGO_LDFLAGS
unset CC