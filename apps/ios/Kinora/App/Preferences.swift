import Foundation
import Network
import KinoraKit

/// Device-only player preferences (the profile's own settings live on the server).
extension AppSettings {
    private static let styleKey = "kinora.subtitleStyle"
    private static let cellularKey = "kinora.cellularMaxBitrate"
    private static let languagesKey = "kinora.seriesLanguages"

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

    private init() {
        monitor.pathUpdateHandler = { [weak self] path in
            guard let self else { return }
            lock.withLock { self.expensive = path.isExpensive || path.isConstrained }
        }
        monitor.start(queue: DispatchQueue(label: "kinora.network"))
    }

    var isExpensive: Bool { lock.withLock { expensive } }
}
