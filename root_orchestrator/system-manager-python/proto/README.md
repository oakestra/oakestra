## How to generate the proto files

Run from the service root (the directory containing `proto/`):

```
python3 -m grpc_tools.protoc -I. --python_out=. --pyi_out=. --grpc_python_out=. proto/clusterRegistration.proto
```