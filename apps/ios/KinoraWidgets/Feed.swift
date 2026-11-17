import Foundation
import UIKit
import WidgetKit
import KinoraKit

/// One title on a widget. Images travel as small JPEG data, because widget views
/// cannot load anything themselves.
struct Card: Codable, Hashable, Identifiable {
    var id: Int
    var title: String
    var subtitle: String?
    /// Watched share 0…1 for started titles.
    var progress: Double?
    var image: Data?
    var link: URL
}

struct FeedEntry: TimelineEntry {
    enum State: Equatable {
        case cards([Card])
        case empty
        case signedOut
        case offline
    }

    var date: Date
    var state: State
    /// From the last good fetch, because the server could not be reached now.
    var stale = false

    var cards: [Card] {
        if case .cards(let c) = state { c } else { [] }
    }
}

enum FeedKind: String {
    case continueWatching = "continue"
    case recentlyAdded = "recent"

    var limit: Int { self == .continueWatching ? 4 : 8 }
    /// Wide stills for "continue", posters for "new".
    var imageWidth: CGFloat { self == .continueWatching ? 480 : 220 }
}

/// Fetches /api/home with the app's login, and keeps the last good result in the
/// App Group container for when the server is out of reach (away from home).
enum Feed {
    static func load(_ kind: FeedKind) async -> FeedEntry {
        let store = ServerStore.shared()
        guard let server = store.current, let token = store.token(for: server.id) else {
            return FeedEntry(date: .now, state: .signedOut)
        }
        let client = KinoraClient(server: server, token: token)
        do {
            let home = try await client.home()
            let items = kind == .continueWatching ? WidgetFeed.continueWatching(home) : WidgetFeed.recentlyAdded(home)
            var cards: [Card] = []
            for item in items.prefix(kind.limit) {
                cards.append(await card(item, kind: kind, client: client))
            }
            save(cards, kind: kind)
            return FeedEntry(date: .now, state: cards.isEmpty ? .empty : .cards(cards))
        } catch KinoraError.unauthorized {
            return FeedEntry(date: .now, state: .signedOut)
        } catch {
            if let cached = cached(kind), !cached.isEmpty {
                return FeedEntry(date: .now, state: .cards(cached), stale: true)
            }
            return FeedEntry(date: .now, state: .offline)
        }
    }

    private static func card(_ item: Item, kind: FeedKind, client: KinoraClient) async -> Card {
        let episode = item.kind == .episode
        let title = episode ? (item.seriesTitle ?? item.title) : item.title
        let subtitle: String? = episode
            ? [item.episodeLabel, item.title].compactMap { $0 }.joined(separator: " · ")
            : item.year.map(String.init)
        let imageKind: ImageKind = switch kind {
        case .recentlyAdded: .poster
        case .continueWatching: episode ? .thumb : .backdrop
        }
        var data = try? await client.image(item.id, imageKind)
        if data == nil, kind == .continueWatching, episode, let series = item.seriesId {
            data = try? await client.image(series, .backdrop)
        }
        return Card(id: item.id, title: title, subtitle: subtitle,
                    progress: kind == .continueWatching ? item.progress : nil,
                    image: data.flatMap { shrink($0, width: kind.imageWidth) },
                    link: (kind == .continueWatching ? KinoraLink.play(item.id) : .item(item.id)).url)
    }

    /// Widgets have a tight memory budget: keep images just big enough for their spot.
    private static func shrink(_ data: Data, width: CGFloat) -> Data? {
        guard let img = UIImage(data: data) else { return nil }
        guard img.size.width > width else { return img.jpegData(compressionQuality: 0.8) }
        let size = CGSize(width: width, height: (img.size.height * width / img.size.width).rounded())
        let format = UIGraphicsImageRendererFormat()
        format.scale = 1
        return UIGraphicsImageRenderer(size: size, format: format)
            .image { _ in img.draw(in: CGRect(origin: .zero, size: size)) }
            .jpegData(compressionQuality: 0.8)
    }

    // MARK: - last good result

    private static func file(_ kind: FeedKind) -> URL? {
        AppGroup.container?.appending(path: "Library/Caches/widget-\(kind.rawValue).json")
    }

    private static func save(_ cards: [Card], kind: FeedKind) {
        guard let url = file(kind) else { return }
        try? FileManager.default.createDirectory(at: url.deletingLastPathComponent(), withIntermediateDirectories: true)
        try? JSONEncoder().encode(cards).write(to: url, options: .atomic)
    }

    private static func cached(_ kind: FeedKind) -> [Card]? {
        guard let url = file(kind), let data = try? Data(contentsOf: url) else { return nil }
        return try? JSONDecoder().decode([Card].self, from: data)
    }
}

struct FeedProvider: TimelineProvider {
    let kind: FeedKind

    func placeholder(in context: Context) -> FeedEntry {
        FeedEntry(date: .now, state: .cards(Sample.cards(kind)))
    }

    func getSnapshot(in context: Context, completion: @escaping (FeedEntry) -> Void) {
        if context.isPreview { return completion(placeholder(in: context)) }
        let done = Handoff(completion)
        let kind = kind
        Task { done.call(await Feed.load(kind)) }
    }

    func getTimeline(in context: Context, completion: @escaping (Timeline<FeedEntry>) -> Void) {
        let done = Handoff(completion)
        let kind = kind
        Task {
            let entry = await Feed.load(kind)
            // The app asks for a reload after playback; this is for changes made elsewhere.
            let next = Date.now.addingTimeInterval(entry.stale || entry.state == .offline ? 15 * 60 : 30 * 60)
            done.call(Timeline(entries: [entry], policy: .after(next)))
        }
    }
}

/// WidgetKit's completion handlers predate Sendable; each is called once, from one task.
private struct Handoff<T>: @unchecked Sendable {
    let call: (T) -> Void
    init(_ call: @escaping (T) -> Void) { self.call = call }
}

/// Shown in the widget gallery before anything is loaded.
enum Sample {
    static func cards(_ kind: FeedKind) -> [Card] {
        switch kind {
        case .continueWatching:
            [Card(id: 1, title: "Csillagközi utazás", subtitle: "2014", progress: 0.4, link: KinoraLink.play(1).url),
             Card(id: 2, title: "A sorozat", subtitle: "S2 · E5 · A visszatérés", progress: 0.7, link: KinoraLink.play(2).url),
             Card(id: 3, title: "Dokumentumfilm", subtitle: "2021", progress: 0.15, link: KinoraLink.play(3).url),
             Card(id: 4, title: "Vígjáték", subtitle: "2019", progress: 0.55, link: KinoraLink.play(4).url)]
        case .recentlyAdded:
            (1...8).map { Card(id: $0, title: "Film \($0)", link: KinoraLink.item($0).url) }
        }
    }
}
