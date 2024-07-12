version=$(git describe --tags --abbrev=0)

#arm build
env GOOS=linux GOARCH=arm GOARM=7 go build -ldflags="-X 'NetManager/cmd.Version=$version'" -o bin/arm-7-NetManager ../NetManager.go

#amd build
env GOOS=linux GOARCH=amd64 go build -ldflags="-X 'NetManager/cmd.Version=$version'" -o bin/amd64-NetManager ../NetManager.go

