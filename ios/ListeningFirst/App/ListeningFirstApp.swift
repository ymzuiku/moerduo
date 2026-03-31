import SwiftUI
import AppShell
import AuthenticationServices
#if canImport(GoogleSignIn)
import GoogleSignIn
#endif

// Google OAuth config
private let googleClientID = "555010677252-e2cbci6okaoj34naqoa81reiq1nqfod5.apps.googleusercontent.com"

@main
struct ListeningFirstApp: App {

    init() {
        // Configure Google Sign-In client ID programmatically
        // (no GoogleService-Info.plist required)
        #if canImport(GoogleSignIn)
        GIDSignIn.sharedInstance.configuration = GIDConfiguration(clientID: googleClientID)
        #endif
    }

    var body: some Scene {
        WindowGroup {
            AppShellView(config: .init(
                serverURL: serverURL(),
                bundledZip: Bundle.main.url(forResource: "client", withExtension: "zip"),
                adapters: [
                    AuthAdapter(providers: [.apple, .google]),
                ],
                devMode: true
            ))
        }
    }

    private func serverURL() -> String {
        if let url = ProcessInfo.processInfo.environment["SERVER_URL"] {
            return url
        }
        return "https://listening.moerduo.com"
    }
}
