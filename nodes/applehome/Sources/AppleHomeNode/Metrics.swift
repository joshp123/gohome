import Foundation

/// Renders /metrics/applehome in the Prometheus text format. Value metrics carry only
/// current values (see `currentValueWindow`); a value that is not current is absent,
/// and its `characteristic_last_read_timestamp_seconds` shows how old it is.
enum Metrics {
    static func render(_ snapshot: HomeState.Snapshot, now: Date = Date()) -> String {
        var out = MetricsWriter()

        out.gauge("gohome_applehome_scrape_success", "1 when the last poll read at least one value from Apple Home", [
            ([], snapshot.lastPollSucceeded ? 1 : 0),
        ])
        out.gauge("gohome_applehome_last_success_timestamp_seconds", "Unix time of the last successful poll", [
            ([], snapshot.lastSuccess.map(\.timeIntervalSince1970) ?? 0),
        ])
        out.gauge("gohome_applehome_home_access_granted", "1 when macOS allows this app to read Apple Home", [
            ([], snapshot.access == .granted ? 1 : 0),
        ])
        out.gauge("gohome_applehome_homes", "Homes HomeKit returned", [([], Double(snapshot.homeCount))])
        out.gauge("gohome_applehome_accessories", "Accessories across all homes", [([], Double(snapshot.accessoriesAsRead.count))])

        let accessories = snapshot.currentAccessories(at: now).sorted { ($0.room, $0.name) < ($1.room, $1.name) }
        out.gauge("gohome_applehome_accessory_reachable", "1 when HomeKit can reach the accessory", accessories.map {
            (labels($0) + [("category", $0.category)], $0.reachable ? 1 : 0)
        })

        let climate = accessories.compactMap { accessory in accessory.climateReading.map { (accessory, $0) } }
        out.gauge("gohome_applehome_temperature_celsius", "Current temperature", climate.compactMap { accessory, reading in
            reading.hasTemperatureCelsius ? (labels(accessory), reading.temperatureCelsius) : nil
        })
        out.gauge("gohome_applehome_humidity_percent", "Current relative humidity", climate.compactMap { accessory, reading in
            reading.hasHumidityPercent ? (labels(accessory), reading.humidityPercent) : nil
        })
        let thermostats = climate.filter { $0.1.hasThermostat }
        out.gauge("gohome_applehome_thermostat_target_temperature_celsius", "Thermostat target temperature", thermostats.compactMap { accessory, reading in
            reading.thermostat.hasTargetTemperatureCelsius ? (labels(accessory), reading.thermostat.targetTemperatureCelsius) : nil
        })
        out.gauge("gohome_applehome_thermostat_heating", "1 when the thermostat is heating now", thermostats.compactMap { accessory, reading in
            reading.thermostat.currentMode == .unspecified ? nil : (labels(accessory), reading.thermostat.currentMode == .heat ? 1 : 0)
        })
        out.gauge("gohome_applehome_thermostat_target_mode", "1 for the mode the thermostat is set to (off, heat, cool, auto)", thermostats.flatMap { accessory, reading in
            guard reading.thermostat.targetMode != .unspecified else { return [([(String, String)], Double)]() }
            return [("off", Gohome_Plugins_Applehome_V1_HeatingCoolingMode.off), ("heat", .heat), ("cool", .cool), ("auto", .auto)].map { name, mode in
                (labels(accessory) + [("mode", name)], reading.thermostat.targetMode == mode ? 1 : 0)
            }
        })

        // Every numeric or on/off value Apple Home holds, so lights, shutters and sensors
        // are in the time series without a metric per type, and when each was last read.
        var values: [([(String, String)], Double)] = []
        var lastReads: [([(String, String)], Double)] = []
        for accessory in accessories {
            for service in accessory.services {
                for characteristic in service.characteristics {
                    let labels = labels(accessory) + [
                        ("service_id", service.id),
                        ("service", service.name.isEmpty ? service.type : service.name),
                        ("service_type", service.type),
                        ("characteristic_id", characteristic.id),
                        ("characteristic", characteristic.type),
                        ("unit", characteristic.unit),
                    ]
                    if characteristic.readable {
                        lastReads.append((labels, characteristic.hasReadAt ? characteristic.readAt.date.timeIntervalSince1970 : 0))
                    }
                    switch characteristic.value {
                    case .numberValue(let number)?: values.append((labels, number))
                    case .boolValue(let flag)?: values.append((labels, flag ? 1 : 0))
                    default: break
                    }
                }
            }
        }
        out.gauge("gohome_applehome_characteristic_value", "Every current numeric or on/off characteristic value in Apple Home", values)
        out.gauge("gohome_applehome_characteristic_last_read_timestamp_seconds", "Unix time the reader last got the characteristic's value from the accessory; 0 when it never has", lastReads)

        return out.text
    }

    private static func labels(_ accessory: Accessory) -> [(String, String)] {
        [
            ("accessory_id", accessory.id), ("accessory", accessory.name),
            ("room", accessory.room), ("sensor", accessory.sensorName), ("home", accessory.home),
        ]
    }
}

private struct MetricsWriter {
    var text = ""

    mutating func gauge(_ name: String, _ help: String, _ samples: [([(String, String)], Double)]) {
        text += "# HELP \(name) \(help)\n# TYPE \(name) gauge\n"
        for (labels, value) in samples {
            let rendered = labels.map { "\($0.0)=\"\(escape($0.1))\"" }.joined(separator: ",")
            text += labels.isEmpty ? name : "\(name){\(rendered)}"
            text += " \(format(value))\n"
        }
    }

    private func escape(_ value: String) -> String {
        value.replacingOccurrences(of: "\\", with: "\\\\")
            .replacingOccurrences(of: "\"", with: "\\\"")
            .replacingOccurrences(of: "\n", with: "\\n")
    }

    private func format(_ value: Double) -> String {
        if value.isNaN { return "NaN" }
        if value == value.rounded(), abs(value) < 1e15 { return String(Int64(value)) }
        return String(value)
    }
}
