import Foundation
import TVServices
import KinoraKit

/// The Apple TV home screen's top shelf: while the Kinora icon has the focus, the titles in
/// progress (large stills with progress) and the newest ones (posters) show above the apps.
///
/// It uses the app's login through the App Group, like the iPhone widgets. Images are fetched
/// with the token and stored in the App Group container, because the system loads them from a
/// URL and the server's image endpoints need the login. The last good shelf stays there for
/// when the server cannot be reached.
final class ContentProvider: TVTopShelfContentProvider {
    override func loadTopShelfContent(completionHandler: @escaping ((any TVTopShelfContent)?) -> Void) {
        let done = Handoff(completionHandler)
        Task { done.call(await Self.load()?.content) }
    }

    private static func load() async -> Shelf? {
        let store = ServerStore.shared()
        guard let server = store.current, let token = store.token(for: server.id) else { return nil }
        let client = KinoraClient(server: server, token: token)
        guard let home = try? await client.home() else { return Shelf.cached() }

        let shelf = Shelf(
            resume: await Shelf.cards(WidgetFeed.continueWatching(home).prefix(10), wide: true, client: client),
            recent: await Shelf.cards(WidgetFeed.recentlyAdded(home).prefix(12), wide: false, client: client)
        )
        shelf.save()
        return shelf
    }
}

/// The completion handler predates Sendable; it is called once, from one task.
private struct Handoff: @unchecked Sendable {
    let completion: ((any TVTopShelfContent)?) -> Void
    init(_ c: @escaping ((any TVTopShelfContent)?) -> Void) { completion = c }
    func call(_ v: (any TVTopShelfContent)?) { completion(v) }
}

/// What the shelf shows, kept as JSON next to the images.
struct Shelf: Codable, Sendable {
    struct Card: Codable, Sendable {
        var id: Int
        var title: String
        var progress: Double?
        var image: String?
    }

    var resume: [Card]
    var recent: [Card]

    var content: (any TVTopShelfContent)? {
        var sections: [TVTopShelfItemCollection<TVTopShelfSectionedItem>] = []
        if !resume.isEmpty { sections.append(Self.section("Folytatás", resume, wide: true)) }
        if !recent.isEmpty { sections.append(Self.section("Új a könyvtárban", recent, wide: false)) }
        return sections.isEmpty ? nil : TVTopShelfSectionedContent(sections: sections)
    }

    private static func section(_ title: String, _ cards: [Card], wide: Bool) -> TVTopShelfItemCollection<TVTopShelfSectionedItem> {
        let items = cards.map { c in
            let item = TVTopShelfSectionedItem(identifier: "\(wide ? "r" : "n")\(c.id)")
            item.title = c.title
            item.imageShape = wide ? .hdtv : .poster
            if let name = c.image, let dir = Self.directory {
                item.setImageURL(dir.appending(path: name), for: [.screenScale1x, .screenScale2x])
            }
            if let p = c.progress, p > 0 { item.playbackProgress = min(1, p) }
            // A click opens the title (or resumes it), the play button always plays.
            item.displayAction = TVTopShelfAction(url: (wide ? KinoraLink.play(c.id) : .item(c.id)).url)
            item.playAction = TVTopShelfAction(url: KinoraLink.play(c.id).url)
            return item
        }
        let collection = TVTopShelfItemCollection(items: items)
        collection.title = title
        return collection
    }

    static func cards(_ items: ArraySlice<Item>, wide: Bool, client: KinoraClient) async -> [Card] {
        var out: [Card] = []
        for item in items {
            let episode = item.kind == .episode
            let title = episode
                ? [item.seriesTitle, item.episodeLabel].compactMap { $0 }.joined(separator: " · ")
                : item.title
            var data: Data?
            if wide {
                data = try? await client.image(item.id, episode ? .thumb : .backdrop)
                if data == nil, let series = item.seriesId { data = try? await client.image(series, .backdrop) }
            } else {
                data = try? await client.image(item.seriesId ?? item.id, .poster)
                // No poster (a home video, say): the still still beats an empty card.
                if data == nil { data = try? await client.image(item.id, .backdrop) }
            }
            let name = "\(wide ? "wide" : "poster")-\(item.id).jpg"
            var stored: String?
            if let data, let dir = directory {
                try? FileManager.default.createDirectory(at: dir, withIntermediateDirectories: true)
                if (try? data.write(to: dir.appending(path: name), options: .atomic)) != nil { stored = name }
            }
            out.append(Card(id: item.id, title: title, progress: wide ? item.progress : nil, image: stored))
        }
        return out
    }

    // MARK: - storage

    static var directory: URL? { AppGroup.container?.appending(path: "Library/Caches/TopShelf") }

    func save() {
        guard let dir = Self.directory else { return }
        try? JSONEncoder().encode(self).write(to: dir.appending(path: "shelf.json"), options: .atomic)
        // Drop images of titles that left the shelf.
        let keep = Set((resume + recent).compactMap(\.image) + ["shelf.json"])
        for name in (try? FileManager.default.contentsOfDirectory(atPath: dir.path())) ?? [] where !keep.contains(name) {
            try? FileManager.default.removeItem(at: dir.appending(path: name))
        }
    }

    static func cached() -> Shelf? {
        guard let dir = directory, let data = try? Data(contentsOf: dir.appending(path: "shelf.json")) else { return nil }
        return try? JSONDecoder().decode(Shelf.self, from: data)
    }
}
