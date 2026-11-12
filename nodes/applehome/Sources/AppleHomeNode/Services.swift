import Foundation
import GRPCCore
import SwiftProtobuf

/// The plugin's identity and assets, as the registry reports them.
enum Plugin {
    static let id = "applehome"
    static let displayName = "Apple Home"
    static let version = "0.1.0"
    static let services = ["gohome.plugins.applehome.v1.AppleHomeService"]

    static let agentsMD = resource("AGENTS", "md")
    static let descriptorSetURL = Bundle.module.url(forResource: "applehome", withExtension: "pb")!

    private static func resource(_ name: String, _ ext: String) -> String {
        let url = Bundle.module.url(forResource: name, withExtension: ext)!
        return try! String(contentsOf: url, encoding: .utf8)
    }
}

/// gohome's registry service, for a node that runs one plugin.
struct RegistryService: Gohome_Registry_V1_Registry.SimpleServiceProtocol {
    let state: HomeState

    func listPlugins(
        request: Gohome_Registry_V1_ListPluginsRequest,
        context: ServerContext
    ) async throws -> Gohome_Registry_V1_ListPluginsResponse {
        let health = state.current().health()
        var summary = Gohome_Registry_V1_PluginSummary()
        summary.pluginID = Plugin.id
        summary.displayName = Plugin.displayName
        summary.version = Plugin.version
        summary.status = health.status.rawValue
        summary.healthMessage = health.message
        if let last = health.lastSuccess { summary.lastSuccess = Google_Protobuf_Timestamp(date: last) }
        var response = Gohome_Registry_V1_ListPluginsResponse()
        response.plugins = [summary]
        return response
    }

    func describePlugin(
        request: Gohome_Registry_V1_DescribePluginRequest,
        context: ServerContext
    ) async throws -> Gohome_Registry_V1_DescribePluginResponse {
        // Matches the Go registry: an unknown plugin gets an empty response.
        guard request.pluginID == Plugin.id else { return Gohome_Registry_V1_DescribePluginResponse() }
        let health = state.current().health()
        var descriptor = Gohome_Registry_V1_PluginDescriptor()
        descriptor.pluginID = Plugin.id
        descriptor.displayName = Plugin.displayName
        descriptor.version = Plugin.version
        descriptor.services = Plugin.services
        descriptor.agentsMd = Plugin.agentsMD
        descriptor.status = health.status.rawValue
        descriptor.healthMessage = health.message
        if let last = health.lastSuccess { descriptor.lastSuccess = Google_Protobuf_Timestamp(date: last) }
        var response = Gohome_Registry_V1_DescribePluginResponse()
        response.plugin = descriptor
        return response
    }

    func watchPlugins(
        request: Gohome_Registry_V1_WatchPluginsRequest,
        response: RPCWriter<Gohome_Registry_V1_PluginEvent>,
        context: ServerContext
    ) async throws {
        // Matches the Go registry: the plugin set is static, so the stream ends at once.
    }
}

/// The applehome plugin's read-only API.
struct AppleHomeService: Gohome_Plugins_Applehome_V1_AppleHomeService.SimpleServiceProtocol {
    let state: HomeState

    func getClimate(
        request: Gohome_Plugins_Applehome_V1_GetClimateRequest,
        context: ServerContext
    ) async throws -> Gohome_Plugins_Applehome_V1_GetClimateResponse {
        let snapshot = try readableSnapshot()
        var response = Gohome_Plugins_Applehome_V1_GetClimateResponse()
        response.readings = snapshot.currentAccessories(at: Date()).compactMap(\.climateReading)
        return response
    }

    func listAccessories(
        request: Gohome_Plugins_Applehome_V1_ListAccessoriesRequest,
        context: ServerContext
    ) async throws -> Gohome_Plugins_Applehome_V1_ListAccessoriesResponse {
        let snapshot = try readableSnapshot()
        var response = Gohome_Plugins_Applehome_V1_ListAccessoriesResponse()
        response.accessories = snapshot.currentAccessories(at: Date())
        return response
    }

    private func readableSnapshot() throws -> HomeState.Snapshot {
        let snapshot = state.current()
        switch snapshot.access {
        case .denied:
            throw RPCError(code: .permissionDenied, message: "macOS denied this app access to Apple Home")
        case .awaitingAnswer where snapshot.lastPollFinished == nil:
            throw RPCError(code: .unavailable, message: "waiting for Home access or the first read")
        default:
            return snapshot
        }
    }
}
