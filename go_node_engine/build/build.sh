#arm build
env GOOS=linux GOARCH=arm64 go build -o NodeEngine_arm-7 ../NodeEngine.go

#amd build
env GOOS=linux GOARCH=amd64 go build -o NodeEngine_amd64 ../NodeEngine.go