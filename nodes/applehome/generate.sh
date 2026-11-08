#!/usr/bin/env bash
# Regenerates the Swift protobuf and gRPC code and the reflection descriptor set
# from the repo's protos. Run from anywhere; needs Nix and Xcode.
set -euo pipefail

node_dir="$(cd "$(dirname "$0")" && pwd)"
repo_dir="$(cd "$node_dir/../.." && pwd)"
export DEVELOPER_DIR="${DEVELOPER_DIR:-/Applications/Xcode.app/Contents/Developer}"

cd "$node_dir"
swift build -c release --product protoc-gen-swift
swift build -c release --product protoc-gen-grpc-swift-2
plugin_dir="$node_dir/.build/release"
protoc="$(nix build --no-link --print-out-paths nixpkgs#protobuf)/bin/protoc"

protos=(registry.proto plugins/applehome.proto)
out="$node_dir/Sources/AppleHomeNode/Generated"
mkdir -p "$out"

cd "$repo_dir/proto"
"$protoc" \
  --plugin=protoc-gen-swift="$plugin_dir/protoc-gen-swift" \
  --plugin=protoc-gen-grpc-swift="$plugin_dir/protoc-gen-grpc-swift-2" \
  --swift_out="$out" --swift_opt=Visibility=Public,FileNaming=PathToUnderscores \
  --grpc-swift_out="$out" --grpc-swift_opt=Visibility=Public,FileNaming=PathToUnderscores \
  --descriptor_set_out="$node_dir/Sources/AppleHomeNode/Resources/applehome.pb" --include_imports \
  "${protos[@]}"
