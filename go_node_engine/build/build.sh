#!/usr/bin/env bash

version=$(git describe --tags --abbrev=0)

#arm build
env GOOS=linux GOARCH=arm64 go build -ldflags="-X 'go_node_engine/cmd.Version=$version'" -o NodeEngine_arm-7 ../NodeEngine.go

#amd build
env GOOS=linux GOARCH=amd64 go build -ldflags="-X 'go_node_engine/cmd.Version=$version'" -o NodeEngine_amd64 ../NodeEngine.go