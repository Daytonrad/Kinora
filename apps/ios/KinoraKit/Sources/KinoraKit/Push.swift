import CryptoKit
import Foundation

// iOS push through the push relay (docs/PUSH_RELAY.md). The app registers its APNs token
// with the relay and gets a handle that only one Kinora server can send to; it hands the
// server that handle and a fresh AES-256 key. The relay never sees the content, the server
// never sees the APNs token. The Notification Service Extension decrypts with the key.

// MARK: - Kinora server API

/// GET /api/push/ios
public struct PushStatus: Codable, Sendable, Equatable {
    public var enabled: Bool
    public var relayUrl: String?
    /// The server's id at the relay; handles are bound to it.
    public var serverId: String?
    /// Whether this session's device is paired on the server.
    public var paired: Bool?

    public init(enabled: Bool, relayUrl: String? = nil, serverId: String? = nil, paired: Bool? = nil) {
        self.enabled = enabled; self.relayUrl = relayUrl; self.serverId = serverId; self.paired = paired
    }
}

struct PushPairBody: Codable, Sendable {
    var handle: String
    var key: String
}

extension KinoraClient {
    public func pushStatus() async throws -> PushStatus { try await get("/api/push/ios") }

    public func pairPush(handle: String, key: Data) async throws {
        try await post("/api/push/ios", PushPairBody(handle: handle, key: key.base64URLEncoded))
    }

    public func unpairPush() async throws { try await delete("/api/push/ios") }

    public func testPush() async throws { try await post("/api/push/ios/test") }
}

// MARK: - relay

/// What the relay answered with when it said no: `{"error": "<code>"}`.
public struct PushRelayError: Error, LocalizedError, Equatable, Sendable {
    public var status: Int
    public var code: String

    public var errorDescription: String? {
        switch code {
        case "rate_limited": L("Túl sok próbálkozás erről a hálózatról. Próbáld újra holnap.")
        case "attestation_required", "attestation_failed":
            L("Az értesítéseket csak az App Store-ból telepített hivatalos Kinora app használhatja ezen az eszközön.")
        case "disabled": L("Ezen a szerveren az értesítéseket letiltották.")
        default: L("A push-közvetítő hibát jelzett ({0}).", code)
        }
    }
}

/// One device registration (POST /v1/devices).
public struct PushDeviceRegistration: Codable, Sendable {
    public var keyId: String
    public var serverId: String
    public var apnsToken: String
    public var environment: String
    public var challenge: String
    public var attestation: String?
    public var assertion: String?

    public init(keyId: String, serverId: String, apnsToken: String, environment: String, challenge: String,
                attestation: String? = nil, assertion: String? = nil) {
        self.keyId = keyId; self.serverId = serverId; self.apnsToken = apnsToken; self.environment = environment
        self.challenge = challenge; self.attestation = attestation; self.assertion = assertion
    }

    /// The bytes App Attest signs (their SHA-256 is the clientDataHash).
    public var clientData: Data { PushRelay.clientData(challenge: challenge, serverId: serverId, apnsToken: apnsToken, environment: environment) }
}

/// The push relay's device endpoints. The server-side endpoints are the Kinora server's business.
public struct PushRelay: Sendable {
    public let base: URL
    private let session: URLSession

    public init?(_ url: String, session: URLSession = .shared) {
        let trimmed = url.trimmingCharacters(in: CharacterSet(charactersIn: "/ "))
        guard let u = URL(string: trimmed), u.scheme == "https" || u.scheme == "http", u.host() != nil else { return nil }
        base = u
        self.session = session
    }

    /// `kinora-relay-v1\n<challenge>\n<server_id>\n<apns_token>\n<environment>`
    public static func clientData(challenge: String, serverId: String, apnsToken: String, environment: String) -> Data {
        Data("kinora-relay-v1\n\(challenge)\n\(serverId)\n\(apnsToken)\n\(environment)".utf8)
    }

    public static func clientDataHash(_ data: Data) -> Data { Data(SHA256.hash(data: data)) }

    public func challenge() async throws -> String {
        struct Out: Decodable { var challenge: String }
        return try await send("GET", "/v1/challenge", nil, as: Out.self).challenge
    }

    /// Registers the device for one server and returns its handle.
    public func register(_ r: PushDeviceRegistration) async throws -> String {
        struct Out: Decodable { var handle: String }
        return try await send("POST", "/v1/devices", try Self.encoder.encode(r), as: Out.self).handle
    }

    private static let encoder: JSONEncoder = {
        let e = JSONEncoder()
        e.keyEncodingStrategy = .convertToSnakeCase
        return e
    }()

    private func send<R: Decodable>(_ method: String, _ path: String, _ body: Data?, as: R.Type) async throws -> R {
        var req = URLRequest(url: base.appending(path: path), timeoutInterval: 20)
        req.httpMethod = method
        req.httpBody = body
        req.setValue("application/json", forHTTPHeaderField: "Content-Type")
        let data: Data, res: URLResponse
        do { (data, res) = try await session.data(for: req) } catch { throw KinoraError.network }
        let status = (res as? HTTPURLResponse)?.statusCode ?? 0
        guard (200..<300).contains(status) else {
            throw PushRelayError(status: status, code: (try? JSONDecoder().decode(RelayErrorBody.self, from: data))?.error ?? "http_\(status)")
        }
        return try JSONDecoder().decode(R.self, from: data)
    }
}

private struct RelayErrorBody: Decodable { var error: String? }

// MARK: - payload

/// userInfo keys the Notification Service Extension adds for the app's tap handling.
public enum PushKeys {
    /// SavedServer.id of the server that sent it.
    public static let server = "kinora_server"
    /// kinora://item/42
    public static let link = "kinora_link"
}


/// The decrypted content of a notification.
public struct PushMessage: Codable, Sendable, Equatable {
    public var v: Int
    public var type: String?
    public var title: String
    public var body: String
    /// Library item to open.
    public var item: Int64?
    public var href: String?
    /// Path on the Kinora server (`/api/images/42/poster`), fetched with the session token.
    public var image: String?
    public var tag: String?

    public init(v: Int = 1, type: String? = nil, title: String, body: String, item: Int64? = nil,
                href: String? = nil, image: String? = nil, tag: String? = nil) {
        self.v = v; self.type = type; self.title = title; self.body = body
        self.item = item; self.href = href; self.image = image; self.tag = tag
    }
}

public enum PushCrypto {
    /// A new AES-256 key for one server.
    public static func newKey() -> Data { SymmetricKey(size: .bits256).withUnsafeBytes { Data($0) } }

    /// Opens CryptoKit's combined layout: nonce(12) || ciphertext || tag(16), which the server writes.
    public static func open(_ sealed: Data, key: Data) throws -> Data {
        try AES.GCM.open(AES.GCM.SealedBox(combined: sealed), using: SymmetricKey(data: key))
    }

    public static func seal(_ plain: Data, key: Data) throws -> Data {
        guard let out = try AES.GCM.seal(plain, using: SymmetricKey(data: key)).combined else { throw CryptoKitError.incorrectParameterSize }
        return out
    }

    /// Decrypts and decodes the `k` field of a notification.
    public static func message(_ k: String, key: Data) throws -> PushMessage {
        guard let sealed = Data(base64URLEncoded: k) else { throw CryptoKitError.incorrectParameterSize }
        return try JSONDecoder().decode(PushMessage.self, from: open(sealed, key: key))
    }
}

// MARK: - storage

/// The pairing of one saved server with the relay. The AES key is in the Keychain.
public struct PushPairing: Codable, Sendable, Equatable {
    /// SavedServer.id
    public var savedServerId: String
    public var relayUrl: String
    /// The server's id at the relay; the `s` field of a notification.
    public var relayServerId: String
    public var handle: String
    public var apnsToken: String
    public var environment: String
    public var updated: Date

    public init(savedServerId: String, relayUrl: String, relayServerId: String, handle: String,
                apnsToken: String, environment: String, updated: Date = .now) {
        self.savedServerId = savedServerId; self.relayUrl = relayUrl; self.relayServerId = relayServerId
        self.handle = handle; self.apnsToken = apnsToken; self.environment = environment; self.updated = updated
    }
}

/// Pairings (UserDefaults) and their keys (Keychain), both in the App Group so the
/// Notification Service Extension can decrypt.
public final class PushStore: @unchecked Sendable {
    private let defaults: UserDefaults
    private let keychain: Keychain
    private let listKey = "kinora.push.pairings"
    private let attestKey = "kinora.push.attestKey"

    public init(defaults: UserDefaults = .standard, keychain: Keychain = Keychain(service: "io.github.scotylucas.kinora.push")) {
        self.defaults = defaults
        self.keychain = keychain
    }

    public static func shared() -> PushStore {
        guard let suite = AppGroup.defaults else { return PushStore() }
        return PushStore(defaults: suite, keychain: Keychain(service: "io.github.scotylucas.kinora.push", accessGroup: AppGroup.id))
    }

    public var pairings: [PushPairing] {
        guard let data = defaults.data(forKey: listKey) else { return [] }
        return (try? JSONDecoder().decode([PushPairing].self, from: data)) ?? []
    }

    public func pairing(saved id: String) -> PushPairing? { pairings.first { $0.savedServerId == id } }

    public func pairing(relayServer id: String) -> PushPairing? { pairings.first { $0.relayServerId == id } }

    public func save(_ p: PushPairing, key: Data) {
        keychain.write(key.base64EncodedString(), account: p.relayServerId)
        var list = pairings.filter { $0.savedServerId != p.savedServerId && $0.relayServerId != p.relayServerId }
        list.append(p)
        defaults.set(try? JSONEncoder().encode(list), forKey: listKey)
    }

    public func remove(saved id: String) {
        let gone = pairings.filter { $0.savedServerId == id }
        gone.forEach { keychain.delete(account: $0.relayServerId) }
        defaults.set(try? JSONEncoder().encode(pairings.filter { $0.savedServerId != id }), forKey: listKey)
    }

    public func key(relayServer id: String) -> Data? {
        keychain.read(account: id).flatMap { Data(base64Encoded: $0) }
    }

    /// The App Attest key of this install, and whether the relay has seen its attestation.
    public struct AttestKey: Codable, Sendable, Equatable {
        public var id: String
        public var attested: Bool
        public var environment: String
        public init(id: String, attested: Bool, environment: String) { self.id = id; self.attested = attested; self.environment = environment }
    }

    public var attest: AttestKey? {
        get { defaults.data(forKey: attestKey).flatMap { try? JSONDecoder().decode(AttestKey.self, from: $0) } }
        set { defaults.set(newValue.flatMap { try? JSONEncoder().encode($0) }, forKey: attestKey) }
    }
}

/// Which APNs environment this build gets tokens for: `sandbox` when it is signed for
/// development (Xcode, ad hoc), `production` from the App Store and TestFlight.
public enum PushEnvironment {
    public static func current(_ bundle: Bundle = .main) -> String {
        guard let url = bundle.url(forResource: "embedded", withExtension: "mobileprovision"),
              let raw = try? Data(contentsOf: url),
              let start = raw.range(of: Data("<?xml".utf8)),
              let end = raw.range(of: Data("</plist>".utf8), in: start.lowerBound..<raw.endIndex),
              let plist = try? PropertyListSerialization.propertyList(from: raw[start.lowerBound..<end.upperBound], format: nil) as? [String: Any],
              let ent = plist["Entitlements"] as? [String: Any] else { return "production" }
        return (ent["aps-environment"] as? String) == "production" ? "production" : "sandbox"
    }
}

extension Data {
    public var base64URLEncoded: String {
        base64EncodedString().replacingOccurrences(of: "+", with: "-").replacingOccurrences(of: "/", with: "_")
            .replacingOccurrences(of: "=", with: "")
    }

    public init?(base64URLEncoded s: String) {
        var b = s.replacingOccurrences(of: "-", with: "+").replacingOccurrences(of: "_", with: "/")
        while b.count % 4 != 0 { b += "=" }
        self.init(base64Encoded: b)
    }
}
