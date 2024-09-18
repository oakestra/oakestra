version=$(git describe --tags --abbrev=0)

#arm build
env GOOS=linux GOARCH=arm64 go build -ldflags="-X 'NetManager/cmd.Version=$version'" -o bin/arm64-NetManager ../NetManager.go

#amd build
env GOOS=linux GOARCH=amd64 go build -ldflags="-X 'NetManager/cmd.Version=$version'" -o bin/amd64-NetManager ../NetManager.go

