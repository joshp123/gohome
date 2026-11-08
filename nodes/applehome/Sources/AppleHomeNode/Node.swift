import Foundation
import GRPCCore
import GRPCNIOTransportHTTP2
import GRPCReflectionService
import NIOCore
import NIOHTTP1
import NIOPosix

/// A gohome node serving the applehome plugin: the registry and applehome gRPC services
/// with reflection, and /health, /metrics/applehome and the dashboard over HTTP.
public enum Node {
    public struct Addresses: Sendable {
        public var host: String
        public var grpcPort: Int
        public var httpPort: Int

        public init(host: String, grpcPort: Int, httpPort: Int) {
            self.host = host
            self.grpcPort = grpcPort
            self.httpPort = httpPort
        }
    }

    /// Serves until the task is cancelled or a server fails.
    public static func serve(state: HomeState, at addresses: Addresses) async throws {
        let reflection = try ReflectionService(descriptorSetFileURLs: [Plugin.descriptorSetURL])
        let grpc = GRPCServer(
            transport: .http2NIOPosix(
                address: .ipv4(host: addresses.host, port: addresses.grpcPort),
                transportSecurity: .plaintext
            ),
            services: [RegistryService(state: state), AppleHomeService(state: state), reflection]
        )
        let http = try await HTTPServer.bind(host: addresses.host, port: addresses.httpPort, state: state)
        print("applehome node: gRPC on \(addresses.host):\(addresses.grpcPort), HTTP on \(addresses.host):\(addresses.httpPort)")

        try await withThrowingDiscardingTaskGroup { group in
            group.addTask { try await grpc.serve() }
            group.addTask { try await http.closeFuture.get() }
        }
    }
}

enum HTTPServer {
    static func bind(host: String, port: Int, state: HomeState) async throws -> Channel {
        try await ServerBootstrap(group: MultiThreadedEventLoopGroup.singleton)
            .serverChannelOption(.socketOption(.so_reuseaddr), value: 1)
            .childChannelInitializer { channel in
                channel.pipeline.configureHTTPServerPipeline().flatMap {
                    channel.pipeline.addHandler(HTTPHandler(state: state))
                }
            }
            .bind(host: host, port: port)
            .get()
    }

    static func respond(to path: String, state: HomeState) -> (HTTPResponseStatus, String, Data) {
        switch path {
        case "/health":
            return (.ok, "text/plain", Data("ok".utf8))
        case "/metrics/\(Plugin.id)":
            return (.ok, "text/plain; version=0.0.4", Data(Metrics.render(state.current()).utf8))
        case Plugin.dashboardPath:
            return (.ok, "application/json", Plugin.dashboardJSON)
        default:
            return (.notFound, "text/plain", Data("not found".utf8))
        }
    }
}

private final class HTTPHandler: ChannelInboundHandler, Sendable {
    typealias InboundIn = HTTPServerRequestPart
    typealias OutboundOut = HTTPServerResponsePart

    let state: HomeState

    init(state: HomeState) {
        self.state = state
    }

    func channelRead(context: ChannelHandlerContext, data: NIOAny) {
        guard case .head(let request) = unwrapInboundIn(data) else { return }
        let path = String(request.uri.split(separator: "?", maxSplits: 1).first ?? "")
        let (status, contentType, body) = request.method == .GET
            ? HTTPServer.respond(to: path, state: state)
            : (.methodNotAllowed, "text/plain", Data("GET only".utf8))

        var headers = HTTPHeaders()
        headers.add(name: "Content-Type", value: contentType)
        headers.add(name: "Content-Length", value: String(body.count))
        headers.add(name: "Connection", value: "close")
        context.write(wrapOutboundOut(.head(HTTPResponseHead(version: request.version, status: status, headers: headers))), promise: nil)
        context.write(wrapOutboundOut(.body(.byteBuffer(ByteBuffer(bytes: body)))), promise: nil)
        let boundContext = NIOLoopBound(context, eventLoop: context.eventLoop)
        context.writeAndFlush(wrapOutboundOut(.end(nil))).whenComplete { _ in boundContext.value.close(promise: nil) }
    }
}
