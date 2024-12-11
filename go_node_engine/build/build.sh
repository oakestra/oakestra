#!/usr/bin/env bash

version=$(git describe --tags --abbrev=0)

# Before running this script, to ensure compatibility with the wasmtime-go library, read the README.md file
#arm build
export CGO_CFLAGS="-I../../wasmtime-go/c-api/include"
export CGO_LDFLAGS="-L../../wasmtime-go/target/aarch64-unknown-linux-gnu/release -lwasmtime"
export LD_LIBRARY_PATH="../../wasmtime-go/target/aarch64-unknown-linux-gnu/release:$LD_LIBRARY_PATH"
export DYLD_LIBRARY_PATH="../../wasmtime-go/target/aarch64-unknown-linux-gnu/release:$DYLD_LIBRARY_PATH"
export CC=aarch64-linux-gnu-gcc

env CGO_ENABLED=1 GOOS=linux GOARCH=arm64 go build -ldflags="-X 'go_node_engine/cmd.Version=$version'" -o NodeEngine_arm64 ../NodeEngine.go
env CGO_ENABLED=1 GOOS=linux GOARCH=arm64 go build -ldflags="-X 'go_node_engine/cmd.Version=$version'" -o nodeengined_arm64 ../pkg/nodeengined.go
unset CC GOOS GOARCH CGO_ENABLED CGO_CFLAGS CGO_LDFLAGS LD_LIBRARY_PATH

#amd build
export CGO_CFLAGS="-I../../wasmtime-go/c-api/include"
export CGO_LDFLAGS="-L../../wasmtime-go/target/x86_64-unknown-linux-gnu/release -lwasmtime"
export LD_LIBRARY_PATH="../../wasmtime-go/target/x86_64-unknown-linux-gnu/release:$LD_LIBRARY_PATH"
export DYLD_LIBRARY_PATH="../../wasmtime-go/target/x86_64-unknown-linux-gnu/release:$DYLD_LIBRARY_PATH"

env CGO_ENABLED=1 GOOS=linux GOARCH=amd64 go build -ldflags="-X 'go_node_engine/cmd.Version=$version'" -o NodeEngine_amd64 ../NodeEngine.go
env CGO_ENABLED=1 GOOS=linux GOARCH=amd64 go build -ldflags="-X 'go_node_engine/cmd.Version=$version'" -o nodeengined_amd64 ../pkg/nodeengined.go
unset CC GOOS GOARCH CGO_ENABLED CGO_CFLAGS CGO_LDFLAGS LD_LIBRARY_PATH
