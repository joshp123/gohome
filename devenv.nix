{ pkgs, ... }:
{
  packages = with pkgs; [
    go
    golangci-lint
    buf
    protobuf
    protoc-gen-go
    protoc-gen-go-grpc
  ];
}
