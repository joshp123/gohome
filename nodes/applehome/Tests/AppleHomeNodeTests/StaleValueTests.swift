import Foundation
import SwiftProtobuf
import Testing
@testable import AppleHomeNode

/// A sensor that stops answering must disappear from metrics and the API instead of
/// repeating its last reading, and a fresh value must not make an old one look fresh.
@Test func failedAndOldReadsAreNotReportedAsCurrent() throws {
    let now = Date(timeIntervalSince1970: 1_800_000_000)
    let tenMinutesAgo = now.addingTimeInterval(-600)
    let tenSecondsAgo = now.addingTimeInterval(-10)

    // Old temperature (22.1, ten minutes old) beside a fresh humidity reading.
    let livingRoom = sensor("Living room", [
        characteristic(HAPType.currentTemperature, "Current Temperature", 22.1, readAt: tenMinutesAgo),
        characteristic(HAPType.currentRelativeHumidity, "Current Relative Humidity", 48, readAt: tenSecondsAgo),
    ])
    // The last read failed; HomeKit still holds its cached 19.4.
    let bedroom = sensor("Bedroom", [
        characteristic(HAPType.currentTemperature, "Current Temperature", 19.4, readAt: tenMinutesAgo, error: "Operation timed out"),
    ])

    let state = HomeState()
    state.setAccess(.granted)
    state.recordPoll(homeCount: 1, accessories: [livingRoom, bedroom], finishedAt: tenSecondsAgo)
    let snapshot = state.current()

    let metrics = Metrics.render(snapshot, now: now)
    #expect(!metrics.contains("22.1"))
    #expect(!metrics.contains("19.4"))
    #expect(!metrics.contains("gohome_applehome_temperature_celsius{"))
    #expect(metrics.contains(#"gohome_applehome_humidity_percent{accessory_id="Living room",accessory="Living room",room="Living room",sensor="IKEA of Sweden ALPSTUGA air quality monitor",home="Home"} 48"#))
    #expect(metrics.contains(#"characteristic="Current Temperature",unit="celsius"} \#(Int(tenMinutesAgo.timeIntervalSince1970))"#))

    let readings = Dictionary(uniqueKeysWithValues: snapshot.currentAccessories(at: now).compactMap(\.climateReading).map { ($0.accessoryName, $0) })
    let living = try #require(readings["Living room"])
    #expect(!living.hasTemperatureCelsius)
    #expect(living.temperatureReadAt.date == tenMinutesAgo)
    #expect(living.humidityPercent == 48)
    #expect(living.humidityReadAt.date == tenSecondsAgo)
    let bed = try #require(readings["Bedroom"])
    #expect(!bed.hasTemperatureCelsius)
}

private func sensor(_ name: String, _ characteristics: [Characteristic]) -> Accessory {
    var service = AccessoryService()
    service.id = name + " sensor"
    service.type = "Sensor"
    service.characteristics = characteristics
    var accessory = Accessory()
    accessory.id = name
    accessory.name = name
    accessory.room = name
    accessory.home = "Home"
    accessory.manufacturer = "IKEA of Sweden"
    accessory.model = "ALPSTUGA air quality monitor"
    accessory.reachable = true
    accessory.services = [service]
    return accessory
}

private func characteristic(_ typeUUID: String, _ type: String, _ value: Double, readAt: Date, error: String = "") -> Characteristic {
    var characteristic = Characteristic()
    characteristic.id = type
    characteristic.type = type
    characteristic.typeUuid = typeUUID
    characteristic.unit = typeUUID == HAPType.currentTemperature ? "celsius" : "percentage"
    characteristic.readable = true
    characteristic.value = .numberValue(value)
    characteristic.readAt = Google_Protobuf_Timestamp(date: readAt)
    characteristic.readError = error
    return characteristic
}
