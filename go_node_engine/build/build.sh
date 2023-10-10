#arm build
env GOOS=linux GOARCH=arm GOARM=7 go build -ldflags="-extldflags=-Wl,-z,lazy" -o bin/NodeEngine_arm-7 ../NodeEngine.go

#amd build
env GOOS=linux GOARCH=amd64 go build -ldflags="-extldflags=-Wl,-z,lazy" -o bin/NodeEngine_amd64 ../NodeEngine.go

