import Foundation
import CoreBluetooth

let resultPath = "/tmp/gohome-casambi-app-result.txt"
FileManager.default.createFile(atPath: resultPath, contents: nil, attributes: [.posixPermissions: 0o600])
let resultFile = FileHandle(forWritingAtPath: resultPath)!
func report(_ text: String) { resultFile.write(Data((text + "\n").utf8)) }

final class CasambiConnectionProbe: NSObject, CBCentralManagerDelegate, CBPeripheralDelegate {
    var manager: CBCentralManager!
    var endpoints: [UUID: CBPeripheral] = [:]
    var selected: CBPeripheral?
    var scanTimer: Timer?
    var deadline = Date(timeIntervalSinceNow: 120)
    let authenticationUUID = CBUUID(string: "c9ffde48-ca5a-0001-ab83-8f519b482f77")
    override init() {
        super.init()
        manager = CBCentralManager(delegate: self, queue: nil, options: [CBCentralManagerOptionShowPowerAlertKey: false])
    }
    func centralManagerDidUpdateState(_ central: CBCentralManager) {
        report("bluetooth_state=\(central.state.rawValue)")
        if central.state == .poweredOn {
            deadline = Date(timeIntervalSinceNow: 40)
            central.scanForPeripherals(withServices: nil)
            scanTimer = Timer.scheduledTimer(withTimeInterval: 10, repeats: false) { [weak self] _ in self?.connectEndpoint() }
        }
    }
    func centralManager(_ central: CBCentralManager, didDiscover peripheral: CBPeripheral, advertisementData: [String: Any], rssi RSSI: NSNumber) {
        let services = advertisementData[CBAdvertisementDataServiceUUIDsKey] as? [CBUUID] ?? []
        let data = advertisementData[CBAdvertisementDataManufacturerDataKey] as? Data ?? Data()
        guard data.count >= 2, Int(data[0]) | Int(data[1]) << 8 == 963, services.contains(CBUUID(string: "FE4D")) else { return }
        endpoints[peripheral.identifier] = peripheral
        report("network_endpoint_count=\(endpoints.count) rssi=\(RSSI)")
    }
    func connectEndpoint() {
        manager.stopScan()
        guard endpoints.count == 1, let peripheral = endpoints.values.first else {
            report("gatt_skipped=expected_one_network_endpoint count=\(endpoints.count)")
            return
        }
        selected = peripheral
        peripheral.delegate = self
        report("gatt_connect_start=true")
        manager.connect(peripheral)
    }
    func centralManager(_ central: CBCentralManager, didConnect peripheral: CBPeripheral) {
        report("gatt_connected=true")
        peripheral.discoverServices(nil)
    }
    func centralManager(_ central: CBCentralManager, didFailToConnect peripheral: CBPeripheral, error: Error?) {
        report("gatt_connect_error=\(error?.localizedDescription ?? "unknown")")
    }
    func centralManager(_ central: CBCentralManager, didDisconnectPeripheral peripheral: CBPeripheral, error: Error?) {
        report("gatt_disconnected=true error=\(error?.localizedDescription ?? "none")")
    }
    func peripheral(_ peripheral: CBPeripheral, didDiscoverServices error: Error?) {
        if let error { report("gatt_service_error=\(error.localizedDescription)"); return }
        report("gatt_service_count=\(peripheral.services?.count ?? 0)")
        for service in peripheral.services ?? [] { peripheral.discoverCharacteristics(nil, for: service) }
    }
    func peripheral(_ peripheral: CBPeripheral, didDiscoverCharacteristicsFor service: CBService, error: Error?) {
        if let error { report("gatt_characteristic_error=\(error.localizedDescription)"); return }
        for characteristic in service.characteristics ?? [] {
            if characteristic.uuid == authenticationUUID {
                report("authentication_characteristic_found=true")
                peripheral.readValue(for: characteristic)
            }
        }
    }
    func peripheral(_ peripheral: CBPeripheral, didUpdateValueFor characteristic: CBCharacteristic, error: Error?) {
        if let error { report("gatt_read_error=\(error.localizedDescription)") }
        else if characteristic.uuid == authenticationUUID {
            let value = characteristic.value ?? Data()
            report("authentication_challenge_read=true bytes=\(value.count)")
            if value.count >= 2 { report("challenge_type=\(value[0]) protocol_byte=\(value[1])") }
        }
        manager.cancelPeripheralConnection(peripheral)
    }
    func finish() {
        scanTimer?.invalidate()
        manager.stopScan()
        if let selected { manager.cancelPeripheralConnection(selected) }
        report("probe_finished=true")
    }
}
let probe = CasambiConnectionProbe()
while Date() < probe.deadline {
    RunLoop.main.run(until: Date(timeIntervalSinceNow: 0.5))
}
probe.finish()
RunLoop.main.run(until: Date(timeIntervalSinceNow: 2))
