import Foundation

/// PATCH /api/profiles/{id} – only the fields that are set change.
public struct ProfileUpdate: Codable, Sendable, Equatable {
    public var subLangs: String?
    public var audioLangs: String?
    public var subMode: String?
    public var autoplayNext: Bool?
    public var maxBitrate: Int?
    public var skipIntro: String?

    public init(subLangs: String? = nil, audioLangs: String? = nil, subMode: String? = nil,
                autoplayNext: Bool? = nil, maxBitrate: Int? = nil, skipIntro: String? = nil) {
        self.subLangs = subLangs; self.audioLangs = audioLangs; self.subMode = subMode
        self.autoplayNext = autoplayNext; self.maxBitrate = maxBitrate; self.skipIntro = skipIntro
    }
}

/// GET /api/devices – one logged-in device of the user.
public struct Device: Codable, Sendable, Equatable, Identifiable, Hashable {
    public var id: String
    public var name: String
    public var platform: String
    public var appVersion: String
    public var ip: String
    public var lastSeen: Int64
    public var current: Bool

    public init(id: String, name: String, platform: String, appVersion: String = "", ip: String = "",
                lastSeen: Int64 = 0, current: Bool = false) {
        self.id = id; self.name = name; self.platform = platform; self.appVersion = appVersion
        self.ip = ip; self.lastSeen = lastSeen; self.current = current
    }

    /// Readable platform name, as the web settings show it.
    public var platformLabel: String {
        switch platform {
        case "ios": "iOS"
        case "android": "Android"
        case "androidtv": "Android TV"
        case "webos": "LG TV"
        case "tizen": "Samsung TV"
        case "web": "Böngésző"
        default: platform
        }
    }

    public var isTV: Bool { ["tizen", "webos", "androidtv"].contains(platform) }
}

public struct DeviceList: Codable, Sendable {
    public var devices: [Device]
}

/// POST /api/quickconnect/lookup – the device waiting for approval.
public struct QuickConnectRequest: Codable, Sendable, Equatable {
    public var code: String
    public var deviceName: String?
    public var platform: String?
    public var appVersion: String?
    public var ip: String?
    public var expiresIn: Int?
}

/// Choices of the settings screen. Keys are what the server stores.
public enum SettingChoices {
    public static let subModes: [(String, String)] = [
        ("auto", "Automatikus (idegen nyelvű hangnál)"), ("always", "Mindig"), ("off", "Kikapcsolva"),
    ]
    public static let skipModes: [(String, String)] = [
        ("manual", "Gomb mutatása"), ("auto", "Automatikus átugrás"), ("off", "Kikapcsolva"),
    ]
    public static let bitrates: [(Int, String)] = [
        (0, "Eredeti"), (20000, "20 Mbps"), (8000, "8 Mbps"), (4000, "4 Mbps"), (2000, "2 Mbps"),
    ]

    /// "most", "5 perce", "3 órája", "2 napja" – same rounding as the web settings.
    public static func ago(_ seconds: Int64) -> String {
        let m = Int((Double(max(0, seconds)) / 60).rounded())
        if m < 1 { return "most" }
        if m < 60 { return "\(m) perce" }
        let h = Int((Double(m) / 60).rounded())
        if h < 48 { return "\(h) órája" }
        return "\(Int((Double(h) / 24).rounded())) napja"
    }
}

extension KinoraClient {
    public func updateProfile(_ id: Int, _ update: ProfileUpdate) async throws -> Profile {
        try await patch("/api/profiles/\(id)", update)
    }

    public func devices() async throws -> [Device] {
        try await get("/api/devices", as: DeviceList.self).devices
    }

    public func removeDevice(_ id: String) async throws {
        try await delete("/api/devices/" + (id.addingPercentEncoding(withAllowedCharacters: .alphanumerics) ?? id))
    }

    /// Logs out every device except this one.
    public func removeOtherDevices() async throws {
        try await delete("/api/devices")
    }

    public func lookupQuickConnect(_ code: String) async throws -> QuickConnectRequest {
        try await post("/api/quickconnect/lookup", QCBody(code: code))
    }

    /// profileID 0: the device shows its own profile picker.
    public func approveQuickConnect(_ code: String, profileID: Int) async throws {
        try await post("/api/quickconnect/approve", QCBody(code: code, profileId: profileID))
    }

    public func denyQuickConnect(_ code: String) async throws {
        try await post("/api/quickconnect/approve", QCBody(code: code, action: "deny"))
    }
}

struct QCBody: Codable, Sendable {
    var code: String
    var profileId: Int?
    var action: String?
}
