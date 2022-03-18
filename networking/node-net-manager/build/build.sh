#arm build
env GOOS=linux GOARCH=arm GOARM=7 go build -o bin/arm-7-NetManager ../NetManager.go

#amd build
env GOOS=linux GOARCH=amd64 go build -o bin/amd64-NetManager ../NetManager.go

