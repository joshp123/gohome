import Foundation
import SwiftProtobuf
import Synchronization

public typealias Accessory = Gohome_Plugins_Applehome_V1_Accessory
public typealias AccessoryService = Gohome_Plugins_Applehome_V1_Service
public typealias Characteristic = Gohome_Plugins_Applehome_V1_Characteristic
typealias ClimateReading = Gohome_Plugins_Applehome_V1_ClimateReading

/// Whether macOS lets this app read the user's Apple Home.
public enum HomeAccess: Sendable, Equatable {
    case awaitingAnswer
    case denied
    case granted
}

/// The latest picture of Apple Home, written by the HomeKit reader and read by the
/// gRPC services, the metrics endpoint and the registry.
public final class HomeState: Sendable {
    struct Snapshot {
        var access: HomeAccess = .awaitingAnswer
        var homeCount = 0
        /// As the reader saw them, with HomeKit's cached values. Callers report
        /// `currentAccessories(at:)` instead.
        var accessoriesAsRead: [Accessory] = []
        var lastPollFinished: Date?
        var lastPollSucceeded = false
        var lastSuccess: Date?

        func currentAccessories(at now: Date) -> [Accessory] {
            accessoriesAsRead.map { $0.current(at: now) }
        }
    }

    private let snapshot = Mutex(Snapshot())

    public init() {}

    public func setAccess(_ access: HomeAccess) {
        snapshot.withLock { $0.access = access }
    }

    /// Records one finished poll. A poll succeeds when HomeKit returned at least one home
    /// and at least one value is current.
    public func recordPoll(homeCount: Int, accessories: [Accessory], finishedAt: Date = Date()) {
        let anyValueCurrent = accessories.contains { accessory in
            accessory.current(at: finishedAt).services.contains { service in
                service.characteristics.contains { $0.value != nil }
            }
        }
        snapshot.withLock {
            $0.homeCount = homeCount
            $0.accessoriesAsRead = accessories
            $0.lastPollFinished = finishedAt
            $0.lastPollSucceeded = homeCount > 0 && anyValueCurrent
            if $0.lastPollSucceeded { $0.lastSuccess = finishedAt }
        }
    }

    /// Replaces the accessory list between polls, after a HomeKit change notification.
    public func updateAccessories(_ accessories: [Accessory]) {
        snapshot.withLock { $0.accessoriesAsRead = accessories }
    }

    func current() -> Snapshot {
        snapshot.withLock { $0 }
    }
}

// MARK: - Health

enum PluginStatus: String {
    case healthy = "HEALTHY"
    case degraded = "DEGRADED"
    case error = "ERROR"
}

struct PluginHealth {
    var status: PluginStatus
    var message: String
    var lastSuccess: Date?
}

extension HomeState.Snapshot {
    /// The same rules as gohome's Go plugins: a source read must have succeeded within
    /// the last 15 minutes, and a poll must have finished within the last 3.
    func health(now: Date = Date()) -> PluginHealth {
        switch access {
        case .denied:
            return PluginHealth(status: .error, message: "Home access denied: allow it in System Settings, Privacy & Security, HomeKit", lastSuccess: lastSuccess)
        case .awaitingAnswer where lastPollFinished == nil:
            return PluginHealth(status: .degraded, message: "Waiting for Home access or the first read", lastSuccess: lastSuccess)
        default:
            break
        }
        guard let finished = lastPollFinished else {
            return PluginHealth(status: .degraded, message: "Awaiting first source read", lastSuccess: lastSuccess)
        }
        if homeCount == 0 {
            return PluginHealth(status: .error, message: "HomeKit returned no homes", lastSuccess: lastSuccess)
        }
        guard lastPollSucceeded, now.timeIntervalSince(finished) < 180,
              let good = lastSuccess, now.timeIntervalSince(good) < 900
        else {
            return PluginHealth(status: .degraded, message: "Source read failed, missing, or overdue", lastSuccess: lastSuccess)
        }
        return PluginHealth(status: .healthy, message: "Source read successfully", lastSuccess: lastSuccess)
    }
}

// MARK: - Climate

/// HAP type UUIDs. HomeKit reports types as these full UUID strings.
enum HAPType {
    static let currentTemperature = hap("11")
    static let currentRelativeHumidity = hap("10")
    static let targetTemperature = hap("35")
    static let currentHeatingCoolingState = hap("F")
    static let targetHeatingCoolingState = hap("33")
    static let thermostatService = hap("4A")

    private static func hap(_ short: String) -> String {
        String(repeating: "0", count: 8 - short.count) + short + "-0000-1000-8000-0026BB765291"
    }
}

// MARK: - Naming

extension Accessory {
    /// What the device is, for the `sensor` label: its manufacturer and model, which
    /// stay the same when the accessory is renamed in Apple Home. The Apple Home name
    /// when HomeKit gives neither, as for some bridges.
    var sensorName: String {
        let product = [manufacturer, model].filter { !$0.isEmpty }.joined(separator: " ")
        return product.isEmpty ? name : product
    }
}

// MARK: - Freshness

/// How long a value read from an accessory stays current: three of the reader's
/// one-minute polls. A value older than this, or whose last read failed, is not
/// reported, so a silent sensor shows as missing rather than as its last reading.
let currentValueWindow: TimeInterval = 180

extension Characteristic {
    func isCurrent(at now: Date) -> Bool {
        hasReadAt && readError.isEmpty && now.timeIntervalSince(readAt.date) < currentValueWindow
    }
}

extension Accessory {
    /// The accessory with every value that is not current removed. `read_at` and
    /// `read_error` stay, so callers can see how old the last good read is.
    func current(at now: Date) -> Accessory {
        var out = self
        for s in out.services.indices {
            for c in out.services[s].characteristics.indices
            where !reachable || !out.services[s].characteristics[c].isCurrent(at: now) {
                out.services[s].characteristics[c].value = nil
            }
        }
        return out
    }
}

// MARK: - Climate

extension Accessory {
    /// The first characteristic of the type, preferring one that holds a number.
    func characteristic(_ type: String, inService serviceType: String? = nil) -> Characteristic? {
        let matches = services
            .filter { serviceType == nil || $0.typeUuid.uppercased() == serviceType }
            .flatMap(\.characteristics)
            .filter { $0.typeUuid.uppercased() == type }
        return matches.first { $0.number != nil } ?? matches.first
    }

    var isThermostat: Bool {
        services.contains { $0.typeUuid.uppercased() == HAPType.thermostatService }
    }

    /// The accessory's climate reading, or nil when it has no temperature, humidity or
    /// thermostat. Call it on `current(at:)`, so only current values are set.
    var climateReading: ClimateReading? {
        let temperature = characteristic(HAPType.currentTemperature)
        let humidity = characteristic(HAPType.currentRelativeHumidity)
        guard temperature != nil || humidity != nil || isThermostat else { return nil }

        var reading = ClimateReading()
        reading.accessoryID = id
        reading.accessoryName = name
        reading.room = room
        reading.reachable = reachable
        if let temperature {
            if let value = temperature.number { reading.temperatureCelsius = value }
            if temperature.hasReadAt { reading.temperatureReadAt = temperature.readAt }
        }
        if let humidity {
            if let value = humidity.number { reading.humidityPercent = value }
            if humidity.hasReadAt { reading.humidityReadAt = humidity.readAt }
        }
        if isThermostat {
            let service = HAPType.thermostatService
            var thermostat = Gohome_Plugins_Applehome_V1_ThermostatState()
            if let current = characteristic(HAPType.currentHeatingCoolingState, inService: service) {
                if let value = current.number { thermostat.currentMode = .fromHAP(value) }
                if current.hasReadAt { thermostat.currentModeReadAt = current.readAt }
            }
            if let target = characteristic(HAPType.targetHeatingCoolingState, inService: service) {
                if let value = target.number { thermostat.targetMode = .fromHAP(value) }
                if target.hasReadAt { thermostat.targetModeReadAt = target.readAt }
            }
            if let target = characteristic(HAPType.targetTemperature, inService: service) {
                if let value = target.number { thermostat.targetTemperatureCelsius = value }
                if target.hasReadAt { thermostat.targetTemperatureReadAt = target.readAt }
            }
            reading.thermostat = thermostat
        }
        return reading
    }
}

extension Characteristic {
    var number: Double? {
        if case .numberValue(let value)? = value { return value }
        return nil
    }
}

extension Gohome_Plugins_Applehome_V1_HeatingCoolingMode {
    /// HAP heating/cooling state: 0 off, 1 heat, 2 cool, 3 auto (target only).
    static func fromHAP(_ value: Double) -> Self {
        switch Int(value) {
        case 0: .off
        case 1: .heat
        case 2: .cool
        case 3: .auto
        default: .unspecified
        }
    }
}
