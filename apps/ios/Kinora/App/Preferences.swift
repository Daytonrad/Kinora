import Foundation
import Network
import KinoraKit

/// Settings on this device only (not stored on the server).
enum AppSettings {
    private static let backgroundAudioKey = "kinora.backgroundAudio"

    /// Keep the sound going when the app leaves the screen (and picture in picture did not start).
    static var backgroundAudio: Bool {
        get { UserDefaults.standard.bool(forKey: backgroundAudioKey) }
        set { UserDefaults.standard.set(newValue, forKey: backgroundAudioKey) }
    }
}

/// Device-only player preferences (the profile's own settings live on the server).
extension AppSettings {
    private static let styleKey = "kinora.subtitleStyle"
    private static let cellularKey = "kinora.cellularMaxBitrate"
    private static let languagesKey = "kinora.seriesLanguages"
    private static let wifiOnlyKey = "kinora.downloadsWifiOnly"
    private static let deleteWatchedKey = "kinora.deleteWatchedDownloads"
    private static let limitKey = "kinora.downloadLimitGB"

    /// Downloads wait for Wi-Fi instead of using mobile data.
    static var downloadsWifiOnly: Bool {
        get { UserDefaults.standard.bool(forKey: wifiOnlyKey) }
        set { UserDefaults.standard.set(newValue, forKey: wifiOnlyKey) }
    }

    /// A downloaded title is removed once it has been watched to the end.
    static var deleteWatchedDownloads: Bool {
        get { UserDefaults.standard.bool(forKey: deleteWatchedKey) }
        set { UserDefaults.standard.set(newValue, forKey: deleteWatchedKey) }
    }

    /// Space downloads may take, in GB; 0 is no limit.
    static var downloadLimitGB: Int {
        get { UserDefaults.standard.integer(forKey: limitKey) }
        set { UserDefaults.standard.set(newValue, forKey: limitKey) }
    }

    static var subtitleStyle: SubtitleStyle {
        get {
            UserDefaults.standard.data(forKey: styleKey).flatMap { try? JSONDecoder().decode(SubtitleStyle.self, from: $0) }
                ?? SubtitleStyle()
        }
        set { UserDefaults.standard.set(try? JSONEncoder().encode(newValue), forKey: styleKey) }
    }

    /// kbps limit on mobile data and personal hotspots; 0 is no limit.
    static var cellularMaxBitrate: Int {
        get { UserDefaults.standard.object(forKey: cellularKey) as? Int ?? 4000 }
        set { UserDefaults.standard.set(newValue, forKey: cellularKey) }
    }

    /// Audio and subtitle language last picked per series.
    static func languages(series id: Int) -> LanguageChoice? {
        allLanguages[String(id)]
    }

    static func setLanguages(_ choice: LanguageChoice, series id: Int) {
        var all = allLanguages
        all[String(id)] = choice
        UserDefaults.standard.set(try? JSONEncoder().encode(all), forKey: languagesKey)
    }

    private static var allLanguages: [String: LanguageChoice] {
        UserDefaults.standard.data(forKey: languagesKey)
            .flatMap { try? JSONDecoder().decode([String: LanguageChoice].self, from: $0) } ?? [:]
    }
}

/// Whether the current connection is metered (cellular, personal hotspot).
final class NetworkStatus: @unchecked Sendable {
    static let shared = NetworkStatus()
    private let monitor = NWPathMonitor()
    private let lock = NSLock()
    private var expensive = false
    private var online = true

    /// Posted on the main queue when the device gets a connection again.
    static let cameOnline = Notification.Name("kinora.cameOnline")

    private init() {
        monitor.pathUpdateHandler = { [weak self] path in
            guard let self else { return }
            let now = path.status == .satisfied
            let back = lock.withLock {
                self.expensive = path.isExpensive || path.isConstrained
                defer { self.online = now }
                return now && !self.online
            }
            if back { DispatchQueue.main.async { NotificationCenter.default.post(name: Self.cameOnline, object: nil) } }
        }
        monitor.start(queue: DispatchQueue(label: "kinora.network"))
    }

    var isExpensive: Bool { lock.withLock { expensive } }
}
