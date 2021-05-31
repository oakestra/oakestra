#arm build
env GOOS=linux GOARCH=arm GOARM=7 go build -o bin/arm-7-TwoNetnsDev1 testEnvironment/TwoNetnsDev1.go
env GOOS=linux GOARCH=arm GOARM=7 go build -o bin/arm-7-TwoNetnsDev2 testEnvironment/TwoNetnsDev2.go
env GOOS=linux GOARCH=arm GOARM=7 go build -o bin/arm-7-TestDockerDev1 testEnvironment/TestDockerDev1.go
env GOOS=linux GOARCH=arm GOARM=7 go build -o bin/arm-7-NetManager NetManager.go

#amd build
env GOOS=linux GOARCH=amd64 go build -o bin/amd64-TwoNetnsDev1 testEnvironment/TwoNetnsDev1.go
env GOOS=linux GOARCH=amd64 go build -o bin/amd64-TwoNetnsDev2 testEnvironment/TwoNetnsDev2.go
env GOOS=linux GOARCH=amd64 go build -o bin/amd64-TestDockerDev1 testEnvironment/TestDockerDev1.go
env GOOS=linux GOARCH=amd64 go build -o bin/amd64-NetManager NetManager.go
