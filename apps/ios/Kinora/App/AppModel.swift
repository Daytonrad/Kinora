import SwiftUI
import UIKit
import WidgetKit
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
    /// The current playback; it outlives the player screen in picture in picture.
    private(set) var player: PlayerModel?
    /// The full-screen player is shown.
    var playerVisible = false
    /// The settings sheet is shown.
    var settingsVisible = false
    /// Bumped when a playback ends, so screens refresh progress and watched marks.
    private(set) var playbackGeneration = 0

    enum Tab: Hashable { case home, libraries, search, downloads }
    /// The selected tab and the home tab's stack, so links from widgets can open a title.
    var tab: Tab = .home
    var homePath = NavigationPath()
    /// A link that arrived before the user reached home (still launching, picking a profile).
    private var pendingLink: KinoraLink?

    private let store = ServerStore.shared()
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
        settingsVisible = false
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
        DownloadStore.shared.attach(c)
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
        reloadWidgets()
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
        reloadWidgets()
    }

    func switchProfile() {
        settingsVisible = false
        player?.close()
        homePath = NavigationPath()
        tab = .home
        screen = .profiles
    }

    func logout() async {
        guard let server else { return }
        settingsVisible = false
        player?.close()
        await client?.logout()
        store.setToken(nil, for: server.id)
        await client?.setToken(nil)
        me = nil
        screen = .login
        reloadWidgets()
    }

    // MARK: - settings

    /// Saves profile settings and refreshes `me`, so the player picks them up next time.
    func updateProfile(_ update: ProfileUpdate) async throws {
        guard let id = me?.profile?.id else { return }
        _ = try await api { try await $0.updateProfile(id, update) }
        try await loadMe()
    }

    /// Adds another address (e.g. the public one) to the current server.
    /// Throws when a different Kinora server answers there.
    func addAddress(_ input: String) async throws {
        guard var saved = server else { return }
        let found = try await Discovery.resolve(input)
        if let other = found.status.serverId, !saved.id.hasPrefix("http"), other != saved.id {
            throw KinoraError.http(409, "Ezen a címen egy másik Kinora-szerver válaszol.")
        }
        saved.urls = ServerAddress.merge(saved.urls, [found.url])
        store.upsert(saved)
        servers = store.servers
        server = servers.first { $0.id == saved.id } ?? saved
        await client?.setAddresses(server?.urls ?? saved.urls)
    }

    /// The token was rejected: back to login, keep the server.
    func sessionLost() {
        settingsVisible = false
        if let server { store.setToken(nil, for: server.id) }
        me = nil
        notice = KinoraError.unauthorized.errorDescription
        screen = .login
        reloadWidgets()
    }

    /// Runs a server call; a rejected token sends the user back to login.
    func guarded<T>(_ work: () async throws -> T) async throws -> T {
        do { return try await work() }
        catch KinoraError.unauthorized { sessionLost(); throw KinoraError.unauthorized }
    }

    // MARK: - playback

    /// Plays a movie or episode; `from` nil resumes where the profile left off.
    func play(_ item: Item, from start: Double? = nil) {
        guard let client else { return }
        player?.close()
        let resume = item.progress != nil ? (item.userData?.position ?? 0) : 0
        // A downloaded copy plays from the device: no server load, works without a connection.
        let local = DownloadStore.shared.localInfo(for: item)
        let p = PlayerModel(item: item, start: start ?? resume, client: client, profile: me?.profile, local: local)
        p.onClose = { [weak self, weak p] in
            guard let self, self.player === p else { return }
            self.player = nil
            self.playerVisible = false
            self.playbackGeneration += 1
            self.reloadWidgets(after: .seconds(2))
        }
        // Picture in picture: the full-screen player steps aside and comes back on restore.
        p.onPiPStart = { [weak self] in self?.playerVisible = false }
        p.onPiPRestore = { [weak self] in self?.playerVisible = true }
        player = p
        playerVisible = true
        Task { await p.begin() }
    }

    /// A guarded call on the current server's client (browse screens).
    func api<T: Sendable>(_ work: (KinoraClient) async throws -> T) async throws -> T {
        guard let client else { throw KinoraError.network }
        return try await guarded { try await work(client) }
    }

    // MARK: - widgets and links

    /// Widgets show the profile's progress; ask for fresh data after it changed. The delay
    /// lets the last progress report reach the server first.
    func reloadWidgets(after delay: Duration = .zero) {
        Task {
            if delay > .zero { try? await Task.sleep(for: delay) }
            WidgetCenter.shared.reloadAllTimelines()
        }
    }

    /// kinora://item/42 or kinora://play/42; waits until the user is on home.
    func open(_ url: URL) {
        guard let link = KinoraLink(url: url) else { return }
        pendingLink = link
        handlePendingLink()
    }

    func handlePendingLink() {
        guard screen == .home, let link = pendingLink, client != nil else { return }
        pendingLink = nil
        Task {
            switch link {
            case .item(let id):
                guard let detail = try? await api({ try await $0.item(id) }) else { return }
                playerVisible = false
                tab = .home
                homePath = NavigationPath([detail.item])
            case .play(let id):
                guard let detail = try? await api({ try await $0.item(id) }) else { return }
                // A series or season has nothing to play itself: show it instead.
                if detail.item.kind == .movie || detail.item.kind == .episode {
                    play(detail.item)
                } else {
                    tab = .home
                    homePath = NavigationPath([detail.item])
                }
            }
        }
    }
}
