#!/bin/bash
# Generate Go code from Protocol Buffer definitions

set -e

# Ensure go binaries are in PATH
export PATH="$PATH:$(go env GOPATH)/bin"

PROTO_DIR="api/proto"
OUT_DIR="api/gen"

echo "Installing protoc plugins if needed..."
go install google.golang.org/protobuf/cmd/protoc-gen-go@latest
go install google.golang.org/grpc/cmd/protoc-gen-go-grpc@latest

echo "Generating wallet service protos..."
protoc \
  --proto_path=${PROTO_DIR}/wallet \
  --go_out=${OUT_DIR}/wallet \
  --go_opt=paths=source_relative \
  --go-grpc_out=${OUT_DIR}/wallet \
  --go-grpc_opt=paths=source_relative \
  ${PROTO_DIR}/wallet/wallet.proto

echo "Generating transaction service protos..."
protoc \
  --proto_path=${PROTO_DIR}/transaction \
  --go_out=${OUT_DIR}/transaction \
  --go_opt=paths=source_relative \
  --go-grpc_out=${OUT_DIR}/transaction \
  --go-grpc_opt=paths=source_relative \
  ${PROTO_DIR}/transaction/transaction.proto

echo "Proto generation complete!"
