import Foundation

/// Talks to one Kinora server. Holds the current address and token; when the address
/// stops answering (left home) it fails over to the server's other known addresses.
public actor KinoraClient {
    public private(set) var server: SavedServer
    public private(set) var token: String?
    /// Called when the client moved to another address of the same server.
    private var onAddressChange: (@Sendable (SavedServer) -> Void)?
    private let session: URLSession

    public init(server: SavedServer, token: String?, session: URLSession = .kinora) {
        self.server = server
        self.token = token
        self.session = session
    }

    public var baseURL: String { server.url }

    public func setToken(_ token: String?) { self.token = token }

    /// Another known address of the same server (used by failover).
    public func setAddresses(_ urls: [String]) {
        server.urls = ServerAddress.merge([server.url], urls)
    }

    public func setOnAddressChange(_ handler: @escaping @Sendable (SavedServer) -> Void) {
        onAddressChange = handler
    }

    // MARK: - requests

    public func get<R: Decodable & Sendable>(_ path: String, as: R.Type = R.self) async throws -> R {
        try await decode(send("GET", path, body: Optional<Empty>.none))
    }

    public func post<R: Decodable & Sendable>(_ path: String, _ body: some Encodable & Sendable = Empty(), as: R.Type = R.self) async throws -> R {
        try await decode(send("POST", path, body: body))
    }

    /// POST that ignores the response body.
    public func post(_ path: String, _ body: some Encodable & Sendable = Empty()) async throws {
        _ = try await send("POST", path, body: body)
    }

    public func patch<R: Decodable & Sendable>(_ path: String, _ body: some Encodable & Sendable, as: R.Type = R.self) async throws -> R {
        try await decode(send("PATCH", path, body: body))
    }

    public func put<R: Decodable & Sendable>(_ path: String, _ body: some Encodable & Sendable, as: R.Type = R.self) async throws -> R {
        try await decode(send("PUT", path, body: body))
    }

    public func delete(_ path: String) async throws {
        _ = try await send("DELETE", path, body: Optional<Empty>.none)
    }

    /// GET returning the raw body (images).
    public func data(_ path: String) async throws -> Data {
        try await send("GET", path, body: Optional<Empty>.none)
    }

    /// Absolute URL of a server path, with the media token when given (images, streams).
    public func url(_ path: String, mediaToken: String? = nil) -> URL? {
        var s = path.hasPrefix("http") ? path : server.url + path
        if let mediaToken, !mediaToken.isEmpty {
            s += (s.contains("?") ? "&" : "?") + "st=" + (mediaToken.addingPercentEncoding(withAllowedCharacters: .urlQueryValueAllowed) ?? mediaToken)
        }
        return URL(string: s)
    }

    private func decode<R: Decodable>(_ data: Data) throws -> R {
        if R.self == Empty.self, let e = Empty() as? R { return e }
        do { return try JSONDecoder.kinora.decode(R.self, from: data) }
        catch { throw KinoraError.http(200, L("Váratlan válasz a szervertől.")) }
    }

    private func send(_ method: String, _ path: String, body: (some Encodable & Sendable)?, retried: Bool = false) async throws -> Data {
        guard let url = URL(string: server.url + path) else { throw KinoraError.network }
        var req = URLRequest(url: url, cachePolicy: .reloadIgnoringLocalCacheData, timeoutInterval: 20)
        req.httpMethod = method
        if let token { req.setValue("Bearer " + token, forHTTPHeaderField: "Authorization") }
        req.setValue(Language.shared.code, forHTTPHeaderField: "X-Kinora-Lang") // titles and overviews follow the UI language
        if let body {
            req.setValue("application/json", forHTTPHeaderField: "Content-Type")
            req.httpBody = try JSONEncoder.kinora.encode(body)
        }
        let data: Data
        let response: URLResponse
        do {
            (data, response) = try await session.data(for: req)
        } catch {
            // A cancelled task (typing in search, leaving a page) is not a dead server:
            // URLSession reports it as URLError.cancelled, so no failover and no error message.
            if error is CancellationError || (error as? URLError)?.code == .cancelled || Task.isCancelled {
                throw CancellationError()
            }
            if !retried, await failover() { return try await send(method, path, body: body, retried: true) }
            throw KinoraError.network
        }
        let code = (response as? HTTPURLResponse)?.statusCode ?? 0
        guard (200..<300).contains(code) else {
            if code == 401 && token != nil && !path.hasPrefix("/api/quickconnect") { throw KinoraError.unauthorized }
            let msg = (try? JSONDecoder.kinora.decode(ErrorBody.self, from: data))?.error ?? ""
            throw KinoraError.http(code, msg)
        }
        return data
    }

    // MARK: - failover

    private var failoverTask: Task<SavedServer?, Never>?

    /// The current address stopped answering: try the server's other addresses.
    /// Returns true when the client moved to a different address.
    public func failover() async -> Bool {
        let before = server
        let task: Task<SavedServer?, Never>
        if let failoverTask {
            task = failoverTask
        } else {
            let session = self.session
            task = Task { await Discovery.reach(before, session: session) }
            failoverTask = task
        }
        let reached = await task.value
        failoverTask = nil
        guard let reached, reached.url != before.url else { return false }
        server = reached
        onAddressChange?(reached)
        return true
    }

    // MARK: - auth helpers

    public func login(username: String, password: String, device: DeviceInfo) async throws -> Session {
        try await post("/api/login", LoginBody(username: username, password: password,
                                               deviceName: device.deviceName, platform: device.platform,
                                               appVersion: device.appVersion))
    }

    public func me() async throws -> Me { try await get("/api/me") }

    public func selectProfile(_ id: Int, pin: String? = nil) async throws {
        try await post("/api/profiles/\(id)/select", PinBody(pin: pin))
    }

    /// Clears the session's profile, so the next start asks "who's watching?" again.
    public func leaveProfile() async throws {
        try await post("/api/profiles/leave")
    }

    public func logout() async {
        try? await post("/api/logout")
    }
}

public struct Empty: Codable, Sendable { public init() {} }
struct ErrorBody: Decodable { var error: String? }
struct LoginBody: Encodable, Sendable {
    var username: String; var password: String
    var deviceName: String; var platform: String; var appVersion: String
}
struct PinBody: Encodable, Sendable { var pin: String? }

extension CharacterSet {
    static let urlQueryValueAllowed: CharacterSet = {
        var s = CharacterSet.urlQueryAllowed
        // ";" too: Go's url.ParseQuery drops a whole "key=value" pair that contains one.
        s.remove(charactersIn: "&+=?;")
        return s
    }()
}

extension URLSession {
    /// No cookies (the server would otherwise mix the cookie session with the token).
    public static let kinora: URLSession = {
        let c = URLSessionConfiguration.default
        c.httpCookieStorage = nil
        c.httpShouldSetCookies = false
        c.requestCachePolicy = .reloadIgnoringLocalCacheData
        c.timeoutIntervalForRequest = 20
        c.waitsForConnectivity = false
        return URLSession(configuration: c)
    }()
}
