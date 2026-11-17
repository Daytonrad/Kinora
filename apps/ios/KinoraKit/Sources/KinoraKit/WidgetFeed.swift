import Foundation

/// A link into the app: widgets and other apps open these.
///   kinora://item/42 – the detail page
///   kinora://play/42 – start (or resume) playback
///   kinora://party/ABC234 – join a watch party
public enum KinoraLink: Equatable, Sendable {
    case item(Int)
    case play(Int)
    case party(String)

    public static let scheme = "kinora"

    public init?(url: URL) {
        guard url.scheme?.lowercased() == Self.scheme, let host = url.host()?.lowercased(),
              let arg = url.pathComponents.dropFirst().first else { return nil }
        switch host {
        case "item": guard let id = Int(arg) else { return nil }; self = .item(id)
        case "play": guard let id = Int(arg) else { return nil }; self = .play(id)
        case "party": guard let code = PartySync.normalizedCode(arg) else { return nil }; self = .party(code)
        default: return nil
        }
    }

    public var url: URL {
        switch self {
        case .item(let id): URL(string: "\(Self.scheme)://item/\(id)")!
        case .play(let id): URL(string: "\(Self.scheme)://play/\(id)")!
        case .party(let code): URL(string: "\(Self.scheme)://party/\(code)")!
        }
    }
}

/// What the widgets show, picked from the home rows the server already builds.
public enum WidgetFeed {
    /// Started titles first, then the next episode of series in progress; one entry per series.
    public static func continueWatching(_ home: HomeData) -> [Item] {
        let rows = ["continue", "nextup"].compactMap { id in home.rows.first { $0.id == id } }
        var seen = Set<String>()
        return rows.flatMap(\.items).filter { item in
            seen.insert(item.seriesId.map { "s\($0)" } ?? "i\(item.id)").inserted
        }
    }

    /// Newest titles of every library, newest library row first, without repeats.
    public static func recentlyAdded(_ home: HomeData) -> [Item] {
        let rows = home.rows.filter { $0.id.hasPrefix("new-") }
        var seen = Set<Int>()
        var out: [Item] = []
        // Interleave the libraries, so one big movie library does not hide the series.
        let longest = rows.map(\.items.count).max() ?? 0
        for i in 0..<longest {
            for row in rows where i < row.items.count {
                let item = row.items[i]
                if seen.insert(item.id).inserted { out.append(item) }
            }
        }
        return out
    }
}
