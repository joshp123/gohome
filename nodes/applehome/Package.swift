// swift-tools-version:6.0
import PackageDescription

// The node's server half: registry, applehome gRPC service and metrics.
// It has no HomeKit code, so it builds and tests on plain macOS; the Mac Catalyst app
// in App/ supplies the HomeKit reader.
let package = Package(
    name: "AppleHomeNode",
    platforms: [.macOS(.v15), .macCatalyst(.v18)],
    products: [
        .library(name: "AppleHomeNode", targets: ["AppleHomeNode"]),
    ],
    dependencies: [
        .package(url: "https://github.com/grpc/grpc-swift-2.git", from: "2.4.4"),
        .package(url: "https://github.com/grpc/grpc-swift-nio-transport.git", from: "2.12.0"),
        .package(url: "https://github.com/grpc/grpc-swift-protobuf.git", from: "2.5.0"),
        .package(url: "https://github.com/grpc/grpc-swift-extras.git", from: "2.2.2"),
        .package(url: "https://github.com/apple/swift-protobuf.git", from: "1.39.0"),
        .package(url: "https://github.com/apple/swift-nio.git", from: "2.105.0"),
        // 1.7 fails to compile with the Swift 6.4 beta in Xcode 27 beta (BorrowingIteratorProtocol).
        .package(url: "https://github.com/apple/swift-collections.git", "1.3.0"..<"1.7.0"),
    ],
    targets: [
        .target(
            name: "AppleHomeNode",
            dependencies: [
                .product(name: "GRPCCore", package: "grpc-swift-2"),
                .product(name: "GRPCNIOTransportHTTP2", package: "grpc-swift-nio-transport"),
                .product(name: "GRPCProtobuf", package: "grpc-swift-protobuf"),
                .product(name: "GRPCReflectionService", package: "grpc-swift-extras"),
                .product(name: "SwiftProtobuf", package: "swift-protobuf"),
                .product(name: "NIOCore", package: "swift-nio"),
                .product(name: "NIOPosix", package: "swift-nio"),
                .product(name: "NIOHTTP1", package: "swift-nio"),
            ],
            resources: [
                .copy("Resources/AGENTS.md"),
                .copy("Resources/applehome.pb"),
            ]
        ),
        .testTarget(name: "AppleHomeNodeTests", dependencies: ["AppleHomeNode"]),
    ]
)
