import SwiftUI
import AppShell

@main
struct ListeningFirstApp: App {
    init() {
        // Force update server URL in case it was cached from a previous config
        UserDefaults.standard.set("http://192.168.1.13:20300", forKey: "appshell_server_url")
    }

    var body: some Scene {
        WindowGroup {
            AppShellView(config: .init(
                serverURL: "http://192.168.1.13:20300",
                bundledZip: Bundle.main.url(forResource: "client", withExtension: "zip"),
                adapters: [],
                devMode: true
            ))
        }
    }
}
