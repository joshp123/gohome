import AppleHomeNode
import Foundation
import HomeKit
import SwiftProtobuf

/// Reads every accessory in every Apple Home home through HomeKit, read-only.
///
/// Once a minute it asks each accessory for every readable characteristic (`readValue`),
/// and between polls it takes HomeKit's change notifications. It never calls `writeValue`
/// and never changes a home, room, accessory, scene or automation.
final class HomeKitReader: NSObject, HMHomeManagerDelegate, HMAccessoryDelegate {
    private static let pollInterval: TimeInterval = 60
    private static let pollGiveUp: TimeInterval = 300

    private let state: HomeState
    private let manager = HMHomeManager()
    private var readAt: [UUID: Date] = [:]
    private var readError: [UUID: String] = [:]
    private var notifying: Set<UUID> = []
    private var pollStarted: Date?
    private var timer: Timer?

    init(state: HomeState) {
        self.state = state
        super.init()
        manager.delegate = self
        timer = Timer.scheduledTimer(withTimeInterval: Self.pollInterval, repeats: true) { [weak self] _ in
            self?.poll()
        }
    }

    // MARK: HMHomeManagerDelegate

    func homeManager(_ manager: HMHomeManager, didUpdate status: HMHomeManagerAuthorizationStatus) {
        report(status)
    }

    private func report(_ status: HMHomeManagerAuthorizationStatus) {
        let access: HomeAccess = status.contains(.authorized) ? .granted
            : status.contains(.determined) ? .denied
            : .awaitingAnswer
        NSLog("applehome: Home access %@", String(describing: access))
        state.setAccess(access)
    }

    func homeManagerDidUpdateHomes(_ manager: HMHomeManager) {
        NSLog("applehome: %d homes, %d accessories", manager.homes.count, accessories.count)
        // HomeKit calls didUpdate status only when the status changes, so after a
        // restart with access already granted this is where it first shows.
        report(manager.authorizationStatus)
        poll()
    }

    // MARK: HMAccessoryDelegate

    func accessory(_ accessory: HMAccessory, service: HMService, didUpdateValueFor characteristic: HMCharacteristic) {
        readAt[characteristic.uniqueIdentifier] = Date()
        readError[characteristic.uniqueIdentifier] = nil
        state.updateAccessories(snapshot())
    }

    func accessoryDidUpdateReachability(_ accessory: HMAccessory) {
        state.updateAccessories(snapshot())
    }

    // MARK: Polling

    private var accessories: [(home: HMHome, accessory: HMAccessory)] {
        manager.homes.flatMap { home in home.accessories.map { (home, $0) } }
    }

    private func poll() {
        if let started = pollStarted, Date().timeIntervalSince(started) < Self.pollGiveUp { return }

        let characteristics = accessories.flatMap { _, accessory -> [HMCharacteristic] in
            accessory.delegate = self
            return accessory.services.flatMap(\.characteristics)
        }
        for characteristic in characteristics where characteristic.supportsNotifications
            && !notifying.contains(characteristic.uniqueIdentifier)
        {
            notifying.insert(characteristic.uniqueIdentifier)
            characteristic.enableNotification(true) { _ in }
        }

        let readable = characteristics.filter(\.isReadable)
        guard !readable.isEmpty else {
            state.recordPoll(homeCount: manager.homes.count, accessories: snapshot())
            return
        }

        let started = Date()
        pollStarted = started
        var remaining = readable.count
        for characteristic in readable {
            characteristic.readValue { [weak self] error in
                DispatchQueue.main.async {
                    guard let self, self.pollStarted == started else { return }
                    if let error {
                        self.readError[characteristic.uniqueIdentifier] = error.localizedDescription
                    } else {
                        self.readAt[characteristic.uniqueIdentifier] = Date()
                        self.readError[characteristic.uniqueIdentifier] = nil
                    }
                    remaining -= 1
                    if remaining == 0 {
                        self.pollStarted = nil
                        self.state.recordPoll(homeCount: self.manager.homes.count, accessories: self.snapshot())
                    }
                }
            }
        }
    }

    // MARK: Conversion to the applehome proto

    private func snapshot() -> [Accessory] {
        accessories.map { home, accessory in
            var out = Accessory()
            out.id = accessory.uniqueIdentifier.uuidString
            out.name = accessory.name
            out.home = home.name
            out.room = accessory.room?.name ?? ""
            out.category = accessory.category.localizedDescription
            out.manufacturer = accessory.manufacturer ?? ""
            out.model = accessory.model ?? ""
            out.firmwareVersion = accessory.firmwareVersion ?? ""
            out.reachable = accessory.isReachable
            out.bridged = accessory.isBridged
            out.services = accessory.services.map(convert)
            return out
        }
    }

    private func convert(_ service: HMService) -> AccessoryService {
        var out = AccessoryService()
        out.id = service.uniqueIdentifier.uuidString
        out.name = service.name
        out.type = service.localizedDescription
        out.typeUuid = service.serviceType
        out.characteristics = service.characteristics.map(convert)
        return out
    }

    private func convert(_ characteristic: HMCharacteristic) -> Characteristic {
        var out = Characteristic()
        out.id = characteristic.uniqueIdentifier.uuidString
        out.type = characteristic.localizedDescription
        out.typeUuid = characteristic.characteristicType
        out.unit = characteristic.metadata?.units ?? ""
        out.readable = characteristic.isReadable
        out.writable = characteristic.properties.contains(HMCharacteristicPropertyWritable)
        out.value = Self.protoValue(characteristic.value, format: characteristic.metadata?.format)
        if let read = readAt[characteristic.uniqueIdentifier] {
            out.readAt = Google_Protobuf_Timestamp(date: read)
        }
        out.readError = readError[characteristic.uniqueIdentifier] ?? ""
        return out
    }

    private static func protoValue(_ value: Any?, format: String?) -> Characteristic.OneOf_Value? {
        switch value {
        case let number as NSNumber where format == HMCharacteristicMetadataFormatBool:
            return .boolValue(number.boolValue)
        case let number as NSNumber:
            return .numberValue(number.doubleValue)
        case let string as String:
            return .stringValue(string)
        case let data as Data:
            return .dataValue(data)
        default:
            return nil
        }
    }
}

private extension HMCharacteristic {
    var isReadable: Bool { properties.contains(HMCharacteristicPropertyReadable) }
    var supportsNotifications: Bool { properties.contains(HMCharacteristicPropertySupportsEventNotification) }
}
