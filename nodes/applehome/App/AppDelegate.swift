import AppleHomeNode
import UIKit

/// A windowless Mac Catalyst app: HomeKit is only available to Catalyst and iOS apps.
/// Settings come from launch arguments, for example `-listenHost 100.64.0.5`.
@main
final class AppDelegate: UIResponder, UIApplicationDelegate {
    private let state = HomeState()
    private var reader: HomeKitReader?
    private var keepAwake: NSObjectProtocol?

    func application(
        _ application: UIApplication,
        didFinishLaunchingWithOptions launchOptions: [UIApplication.LaunchOptionsKey: Any]?
    ) -> Bool {
        let defaults = UserDefaults.standard
        let addresses = Node.Addresses(
            host: defaults.string(forKey: "listenHost") ?? "127.0.0.1",
            // Launch arguments arrive as strings; integer(forKey:) converts them, and is 0 when unset.
            grpcPort: defaults.integer(forKey: "grpcPort").nonZero ?? 9000,
            httpPort: defaults.integer(forKey: "httpPort").nonZero ?? 8080
        )

        // A windowless app is a candidate for App Nap, which would stall the poll timer.
        keepAwake = ProcessInfo.processInfo.beginActivity(
            options: .userInitiatedAllowingIdleSystemSleep,
            reason: "gohome reads Apple Home once a minute"
        )
        reader = HomeKitReader(state: state)

        Task.detached { [state] in
            do {
                try await Node.serve(state: state, at: addresses)
            } catch {
                NSLog("applehome: server failed: %@", String(describing: error))
            }
            exit(1)
        }
        return true
    }
}

/// UIKit on macOS 27.2 kills an app at launch unless it adopts the scene lifecycle
/// (UIApplicationSceneManifest in Info.plist). The scene stays empty: the app has no window.
final class SceneDelegate: UIResponder, UIWindowSceneDelegate {}

private extension Int {
    var nonZero: Int? { self == 0 ? nil : self }
}
