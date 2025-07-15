# To generate the gRPC code

Install gRPC for go following [this](https://grpc.io/docs/languages/go/quickstart/) guide. 

Run the following command while in the `go_node_engine` directory:

```
protoc --go_out=. --go_opt=paths=source_relative \
    --go-grpc_out=. --go-grpc_opt=paths=source_relative \
    requests/proto/migration.proto
```

More information on how to use the `protoc` command can be found in the [Protocol Buffers documentation](https://grpc.io/docs/languages/go/quickstart/).