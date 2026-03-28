import SwiftUI
import AppShell

@main
struct ListeningFirstApp: App {
    init() {
        let cacheDir = FileManager.default.urls(for: .cachesDirectory, in: .userDomainMask)[0]
        try? FileManager.default.removeItem(at: cacheDir.appendingPathComponent("appshell_client"))
    }

    var body: some Scene {
        WindowGroup {
            AppShellView(config: .init(
                serverURL: "http://192.168.1.100:20100",
                bundledZip: Bundle.main.url(forResource: "client", withExtension: "zip"),
                adapters: [],
                devMode: true
            ))
        }
    }
}
