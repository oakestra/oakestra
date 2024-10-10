#!/usr/bin/env bash

version=$(git describe --tags --abbrev=0)

#arm build
env GOOS=linux GOARCH=arm64 go build -ldflags="-X 'go_node_engine/cmd.Version=$version'" -o NodeEngine_arm64 ../NodeEngine.go
env GOOS=linux GOARCH=arm64 go build -ldflags="-X 'go_node_engine/cmd.Version=$version'" -o nodeengined_arm64 ../pkg/nodeengined.go

#amd build
env GOOS=linux GOARCH=amd64 go build -ldflags="-X 'go_node_engine/cmd.Version=$version'" -o NodeEngine_amd64 ../NodeEngine.go
env GOOS=linux GOARCH=amd64 go build -ldflags="-X 'go_node_engine/cmd.Version=$version'" -o nodeengined_amd64 ../pkg/nodeengined.go