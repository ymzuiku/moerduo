import SwiftUI
import AppShell
import AuthenticationServices

// Google OAuth config
private let googleClientID = "555010677252-e2cbci6okaoj34naqoa81reiq1nqfod5.apps.googleusercontent.com"
private let googleRedirectURI = "com.googleusercontent.apps.555010677252-e2cbci6okaoj34naqoa81reiq1nqfod5:/oauth2callback"

@main
struct ListeningFirstApp: App {
    @StateObject private var authManager = AuthManager()

    init() {
        // AppShell will auto-update client from server when version hash changes
    }

    var body: some Scene {
        WindowGroup {
            AppShellView(config: .init(
                serverURL: serverURL(),
                bundledZip: Bundle.main.url(forResource: "client", withExtension: "zip"),
                adapters: [
                    AppleSignInAdapter(authManager: authManager),
                    GoogleSignInAdapter(authManager: authManager),
                ],
                devMode: false
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

// MARK: - Auth Manager

class AuthManager: NSObject, ObservableObject, ASAuthorizationControllerDelegate {

    // ── Apple Sign In ──

    private var appleCompletion: ((String?, String?, Error?) -> Void)?

    func appleSignIn(completion: @escaping (String?, String?, Error?) -> Void) {
        appleCompletion = completion
        let request = ASAuthorizationAppleIDProvider().createRequest()
        request.requestedScopes = [.email, .fullName]
        let controller = ASAuthorizationController(authorizationRequests: [request])
        controller.delegate = self
        controller.performRequests()
    }

    func authorizationController(controller: ASAuthorizationController,
                                  didCompleteWithAuthorization authorization: ASAuthorization) {
        guard let credential = authorization.credential as? ASAuthorizationAppleIDCredential,
              let tokenData = credential.identityToken,
              let idToken = String(data: tokenData, encoding: .utf8) else {
            appleCompletion?(nil, nil, NSError(domain: "auth", code: -1,
                userInfo: [NSLocalizedDescriptionKey: "No identity token"]))
            return
        }
        let name = [credential.fullName?.givenName, credential.fullName?.familyName]
            .compactMap { $0 }
            .joined(separator: " ")
        appleCompletion?(idToken, name.isEmpty ? nil : name, nil)
    }

    func authorizationController(controller: ASAuthorizationController, didCompleteWithError error: Error) {
        appleCompletion?(nil, nil, error)
    }

    // ── Google Sign In (via ASWebAuthenticationSession) ──

    func googleSignIn(from scene: UIWindowScene?, completion: @escaping (String?, String?, Error?) -> Void) {
        // Build Google OAuth URL
        var components = URLComponents(string: "https://accounts.google.com/o/oauth2/v2/auth")!
        components.queryItems = [
            URLQueryItem(name: "client_id", value: googleClientID),
            URLQueryItem(name: "redirect_uri", value: googleRedirectURI),
            URLQueryItem(name: "response_type", value: "code"),
            URLQueryItem(name: "scope", value: "openid email profile"),
            URLQueryItem(name: "access_type", value: "offline"),
        ]

        guard let authURL = components.url else {
            completion(nil, nil, NSError(domain: "auth", code: -1,
                userInfo: [NSLocalizedDescriptionKey: "Invalid Google auth URL"]))
            return
        }

        let session = ASWebAuthenticationSession(url: authURL, callbackURLScheme: "com.googleusercontent.apps.555010677252-e2cbci6okaoj34naqoa81reiq1nqfod5") { callbackURL, error in
            if let error = error {
                completion(nil, nil, error)
                return
            }
            guard let callbackURL = callbackURL,
                  let code = URLComponents(url: callbackURL, resolvingAgainstBaseURL: false)?
                      .queryItems?.first(where: { $0.name == "code" })?.value else {
                completion(nil, nil, NSError(domain: "auth", code: -1,
                    userInfo: [NSLocalizedDescriptionKey: "No auth code returned"]))
                return
            }

            // Exchange code for id_token via Google token endpoint
            Self.exchangeGoogleCode(code) { idToken, name, error in
                completion(idToken, name, error)
            }
        }

        if let scene = scene,
           let window = scene.windows.first,
           let rootVC = window.rootViewController {
            session.presentationContextProvider = PresentationContext(anchor: window)
        }

        session.prefersEphemeralWebBrowserSession = false
        session.start()
    }

    private static func exchangeGoogleCode(_ code: String, completion: @escaping (String?, String?, Error?) -> Void) {
        var request = URLRequest(url: URL(string: "https://oauth2.googleapis.com/token")!)
        request.httpMethod = "POST"
        request.setValue("application/x-www-form-urlencoded", forHTTPHeaderField: "Content-Type")

        let params = [
            "code": code,
            "client_id": googleClientID,
            "redirect_uri": googleRedirectURI,
            "grant_type": "authorization_code",
        ]
        request.httpBody = params.map { "\($0.key)=\($0.value.addingPercentEncoding(withAllowedCharacters: .urlQueryAllowed) ?? $0.value)" }
            .joined(separator: "&").data(using: .utf8)

        URLSession.shared.dataTask(with: request) { data, _, error in
            DispatchQueue.main.async {
                if let error = error {
                    completion(nil, nil, error)
                    return
                }
                guard let data = data,
                      let json = try? JSONSerialization.jsonObject(with: data) as? [String: Any],
                      let idToken = json["id_token"] as? String else {
                    completion(nil, nil, NSError(domain: "auth", code: -1,
                        userInfo: [NSLocalizedDescriptionKey: "Failed to exchange Google code"]))
                    return
                }
                completion(idToken, nil, nil)
            }
        }.resume()
    }
}

// MARK: - Presentation Context

class PresentationContext: NSObject, ASWebAuthenticationPresentationContextProviding {
    let anchor: ASPresentationAnchor
    init(anchor: ASPresentationAnchor) { self.anchor = anchor }
    func presentationAnchor(for session: ASWebAuthenticationSession) -> ASPresentationAnchor { anchor }
}

// MARK: - AppShell Adapters

struct AppleSignInAdapter: AppShellAdapter {
    let authManager: AuthManager
    var name: String { "appleSignIn" }

    func handle(message: AppShellMessage, webView: Any?, reply: @escaping (Result<Any?, Error>) -> Void) {
        authManager.appleSignIn { idToken, name, error in
            if let error = error { reply(.failure(error)); return }
            guard let idToken = idToken else {
                reply(.failure(NSError(domain: "auth", code: -1))); return
            }
            reply(.success(["id_token": idToken, "name": name ?? ""] as [String: Any]))
        }
    }
}

struct GoogleSignInAdapter: AppShellAdapter {
    let authManager: AuthManager
    var name: String { "googleSignIn" }

    func handle(message: AppShellMessage, webView: Any?, reply: @escaping (Result<Any?, Error>) -> Void) {
        let scene = UIApplication.shared.connectedScenes.first as? UIWindowScene
        authManager.googleSignIn(from: scene) { idToken, name, error in
            if let error = error { reply(.failure(error)); return }
            guard let idToken = idToken else {
                reply(.failure(NSError(domain: "auth", code: -1))); return
            }
            reply(.success(["id_token": idToken, "name": name ?? ""] as [String: Any]))
        }
    }
}
