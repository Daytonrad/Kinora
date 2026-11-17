import SwiftUI
import UIKit
import KinoraKit

/// App-wide state: which server, which session, which screen.
@MainActor @Observable
final class AppModel {
    enum Screen: Equatable {
        case launching
        case servers
        case login
        case quickConnect
        case profiles
        case home
    }

    private(set) var screen: Screen = .launching
    private(set) var servers: [SavedServer] = []
    private(set) var server: SavedServer?
    private(set) var status: ServerStatus?
    private(set) var me: Me?
    /// A message shown once on the next screen (e.g. "session expired").
    var notice: String?

    private let store = ServerStore()
    private(set) var client: KinoraClient?

    static let appVersion = Bundle.main.object(forInfoDictionaryKey: "CFBundleShortVersionString") as? String ?? "0"

    var device: DeviceInfo {
        DeviceInfo(deviceName: UIDevice.current.name, platform: "ios", appVersion: Self.appVersion)
    }

    // MARK: - start

    func start() async {
        guard screen == .launching else { return }
        servers = store.servers
        guard let saved = store.current else { return screen = .servers }
        use(saved)
        guard store.token(for: saved.id) != nil else { return screen = .login }
        // Show the profile picker or home right away; a network error just leaves the
        // user on a retryable screen instead of logging them out.
        do {
            try await loadMe()
            screen = me?.profile == nil ? .profiles : .home
        } catch KinoraError.unauthorized {
            sessionLost()
        } catch {
            notice = error.localizedDescription
            screen = .servers
        }
        Task { await refreshStatus() }
    }

    // MARK: - servers

    /// What the user typed or picked from discovery → saved and current server.
    func connect(to input: String) async throws {
        let r = try await Discovery.resolve(input)
        let id = r.status.serverId ?? r.url
        var s = store.servers.first { $0.id == id } ?? SavedServer(id: id, url: r.url, name: r.status.serverName)
        s.url = r.url
        s.urls = ServerAddress.merge([r.url], s.urls, [r.status.publicUrl])
        s.name = r.status.serverName
        s.lastUsed = .now
        store.upsert(s)
        servers = store.servers
        use(store.current ?? s)
        status = r.status
        screen = store.token(for: id) == nil ? .login : .profiles
        if screen == .profiles { try? await loadMe() }
    }

    func open(_ saved: SavedServer) async throws {
        guard let reached = await Discovery.reach(saved) else {
            throw KinoraError.unreachable(ServerAddress.host(of: saved.url), isPrivate: ServerAddress.isPrivateHost(ServerAddress.host(of: saved.url)))
        }
        var s = reached
        s.lastUsed = .now
        store.upsert(s)
        servers = store.servers
        use(s)
        await refreshStatus()
        if store.token(for: s.id) == nil { screen = .login; return }
        do {
            try await loadMe()
            screen = me?.profile == nil ? .profiles : .home
        } catch KinoraError.unauthorized {
            sessionLost()
        }
    }

    func forget(_ saved: SavedServer) {
        store.remove(saved.id)
        servers = store.servers
        if server?.id == saved.id { server = nil; client = nil; me = nil }
    }

    /// Switch between the auth screens of the current server.
    func go(_ s: Screen) {
        guard server != nil else { return showServers() }
        screen = s
    }

    func showServers() {
        servers = store.servers
        screen = .servers
    }

    private func use(_ s: SavedServer) {
        server = s
        let c = KinoraClient(server: s, token: store.token(for: s.id))
        let store = self.store
        Task {
            await c.setOnAddressChange { moved in
                store.upsert(moved)
            }
        }
        client = c
        me = nil
    }

    private func refreshStatus() async {
        guard let server else { return }
        status = try? await Discovery.probe(server.url).status
    }

    // MARK: - login

    func login(username: String, password: String) async throws {
        guard let client, var server else { return }
        let session = try await client.login(username: username, password: password, device: device)
        server.username = username
        server.deviceId = session.deviceId
        server.lastUsed = .now
        await finishLogin(server, token: session.token)
        // A single profile without PIN: no need to ask "who's watching?".
        if let profiles = me?.profiles, profiles.count == 1, !profiles[0].hasPin,
           (try? await selectProfile(profiles[0], pin: nil)) != nil { return }
        screen = .profiles
    }

    func startQuickConnect() async throws -> QuickConnectStart {
        guard let client else { throw KinoraError.network }
        do {
            return try await client.post("/api/quickconnect/initiate", device)
        } catch KinoraError.http(403, _) {
            throw KinoraError.http(403, "A belépés kóddal ki van kapcsolva ezen a szerveren.")
        }
    }

    /// One poll; returns the state. On approval it also finishes the login.
    func pollQuickConnect(_ start: QuickConnectStart) async throws -> QuickConnectState {
        guard let client, var server else { throw KinoraError.network }
        let secret = start.secret.addingPercentEncoding(withAllowedCharacters: .alphanumerics) ?? start.secret
        let st: QuickConnectState
        do {
            st = try await client.get("/api/quickconnect/status?secret=" + secret)
        } catch KinoraError.http(let code, _) where code == 404 || code == 410 {
            return QuickConnectState(status: .expired)
        }
        if st.status == .approved, let token = st.token {
            server.username = st.user?.username
            server.deviceId = st.deviceId
            server.lastUsed = .now
            await finishLogin(server, token: token)
            screen = me?.profile != nil ? .home : .profiles
        }
        return st
    }

    private func finishLogin(_ s: SavedServer, token: String) async {
        store.upsert(s)
        store.setToken(token, for: s.id)
        servers = store.servers
        server = s
        await client?.setToken(token)
        try? await loadMe()
    }

    // MARK: - session

    func loadMe() async throws {
        guard let client else { return }
        me = try await client.me()
    }

    func selectProfile(_ p: Profile, pin: String?) async throws {
        guard let client else { return }
        do {
            try await client.selectProfile(p.id, pin: pin)
        } catch KinoraError.http(403, let message) where message.localizedCaseInsensitiveContains("PIN") {
            throw KinoraError.http(403, "Hibás PIN.")
        }
        try await loadMe()
        screen = .home
    }

    func switchProfile() {
        screen = .profiles
    }

    func logout() async {
        guard let server else { return }
        await client?.logout()
        store.setToken(nil, for: server.id)
        await client?.setToken(nil)
        me = nil
        screen = .login
    }

    /// The token was rejected: back to login, keep the server.
    func sessionLost() {
        if let server { store.setToken(nil, for: server.id) }
        me = nil
        notice = KinoraError.unauthorized.errorDescription
        screen = .login
    }

    /// Runs a server call; a rejected token sends the user back to login.
    func guarded<T>(_ work: () async throws -> T) async throws -> T {
        do { return try await work() }
        catch KinoraError.unauthorized { sessionLost(); throw KinoraError.unauthorized }
    }
}
