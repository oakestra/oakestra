#arm build
env GOOS=linux GOARCH=arm GOARM=7 go build -o bin/NodeEngine_arm ../NodeEngine.go

#amd build
env GOOS=linux GOARCH=amd64 go build -o bin/NodeEngine_amd64 ../NodeEngine.go

