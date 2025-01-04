.PHONY: all proto

all: proto

proto:
	. bin/activate && \
	python3 -m grpc_tools.protoc --python_out=. --grpc_python_out=. --proto_path=. proto/*.proto