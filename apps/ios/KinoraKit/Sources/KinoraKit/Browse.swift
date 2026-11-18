import Foundation

// MARK: - models

public struct Library: Codable, Sendable, Identifiable, Hashable {
    public var id: Int
    public var name: String
    /// movies, shows or mixed
    public var kind: String
    public var count: Int?

    public init(id: Int, name: String, kind: String, count: Int? = nil) {
        self.id = id; self.name = name; self.kind = kind; self.count = count
    }
}

/// The profile's own state for an item.
public struct UserData: Codable, Sendable, Hashable {
    public var position: Double = 0
    public var played = false
    public var playCount = 0
    public var favorite = false
    public var stars: Int?
    public var hidden = false

    public init() {}

    public init(from decoder: Decoder) throws {
        let c = try decoder.container(keyedBy: CodingKeys.self)
        position = try c.decodeIfPresent(Double.self, forKey: .position) ?? 0
        played = try c.decodeIfPresent(Bool.self, forKey: .played) ?? false
        playCount = try c.decodeIfPresent(Int.self, forKey: .playCount) ?? 0
        favorite = try c.decodeIfPresent(Bool.self, forKey: .favorite) ?? false
        stars = try c.decodeIfPresent(Int.self, forKey: .stars)
        hidden = try c.decodeIfPresent(Bool.self, forKey: .hidden) ?? false
    }
}

/// A movie, series, season or episode.
public struct Item: Codable, Sendable, Identifiable, Hashable {
    public enum Kind: String, Codable, Sendable {
        case movie, series, season, episode, unknown
        public init(from decoder: Decoder) throws {
            self = Kind(rawValue: try decoder.singleValueContainer().decode(String.self)) ?? .unknown
        }
    }

    public var id: Int
    public var libraryId: Int?
    public var kind: Kind
    public var title: String
    public var year: Int?
    public var overview: String?
    public var contentRating: String?
    public var communityRating: Double?
    public var runtimeSec: Double?
    public var parentId: Int?
    public var seriesId: Int?
    public var seriesTitle: String?
    public var seasonNum: Int?
    public var episodeNum: Int?
    public var width: Int?
    public var height: Int?
    public var hasPoster: Bool?
    public var hasBackdrop: Bool?
    public var genres: [String]?
    public var episodeCount: Int?
    public var unplayedCount: Int?
    public var userData: UserData?

    public init(id: Int, kind: Kind, title: String) {
        self.id = id; self.kind = kind; self.title = title
    }

    /// "S1 · E3" for episodes, nil otherwise.
    public var episodeLabel: String? {
        guard kind == .episode else { return nil }
        switch (seasonNum, episodeNum) {
        case let (s?, e?): return "S\(s) · E\(e)"
        case let (nil, e?): return L("{0}. rész", e)
        default: return nil
        }
    }

    /// Watched share 0…1 when started but not finished.
    public var progress: Double? {
        guard let pos = userData?.position, pos > 0, let len = runtimeSec, len > 0,
              userData?.played != true else { return nil }
        return min(1, pos / len)
    }
}

public struct HomeRow: Codable, Sendable, Identifiable, Hashable {
    public enum Style: String, Codable, Sendable {
        case poster, landscape
        public init(from decoder: Decoder) throws {
            self = Style(rawValue: try decoder.singleValueContainer().decode(String.self)) ?? .poster
        }
    }
    public var id: String
    public var title: String
    public var style: Style
    public var items: [Item]
    /// Why an item is in the row ("magasra értékelt…"), keyed by item id.
    public var reasons: [String: String]?
}

public struct HomeData: Codable, Sendable {
    public var hero: Item?
    public var rows: [HomeRow]
}

public struct ItemPage: Codable, Sendable {
    public var items: [Item]
    public var total: Int
}

public struct ItemDetail: Codable, Sendable {
    public var item: Item
    public var seasons: [Item]?
    public var playNext: Item?
    public var similar: [Item]?
}

public enum ItemSort: String, CaseIterable, Sendable, Identifiable {
    case title, added, year, communityRating = "community_rating"
    public var id: String { rawValue }
    public var label: String {
        switch self {
        case .title: L("Cím")
        case .added: L("Hozzáadás")
        case .year: L("Év")
        case .communityRating: L("Értékelés")
        }
    }
    /// Natural direction: A→Z for titles, newest/best first otherwise.
    public var order: String { self == .title ? "asc" : "desc" }
}

public enum ImageKind: String, Sendable {
    case poster, backdrop, thumb
}

// MARK: - requests

extension KinoraClient {
    public func home() async throws -> HomeData { try await get("/api/home") }

    public func libraries() async throws -> [Library] { try await get("/api/libraries") }

    public func items(library: Int? = nil, sort: ItemSort = .title, offset: Int = 0, limit: Int = 60) async throws -> ItemPage {
        var q = ["sort=\(sort.rawValue)", "order=\(sort.order)", "offset=\(offset)", "limit=\(limit)"]
        if let library { q.append("library=\(library)") }
        return try await get("/api/items?" + q.joined(separator: "&"))
    }

    public func item(_ id: Int) async throws -> ItemDetail { try await get("/api/items/\(id)") }

    public func children(of id: Int) async throws -> [Item] { try await get("/api/items/\(id)/children") }

    /// One result per series (episodes are grouped under their show).
    public func search(_ query: String) async throws -> [Item] {
        let q = query.addingPercentEncoding(withAllowedCharacters: .urlQueryValueAllowed) ?? query
        return try await get("/api/search?group=1&q=" + q)
    }

    public func setFavorite(_ id: Int, _ on: Bool) async throws -> UserData {
        try await post("/api/items/\(id)/favorite", ["favorite": on])
    }

    public func setPlayed(_ id: Int, _ on: Bool) async throws -> UserData {
        try await post("/api/items/\(id)/played", ["played": on])
    }

    /// Artwork bytes; the server falls back season → series → episode when one is missing.
    public func image(_ id: Int, _ kind: ImageKind) async throws -> Data {
        try await data("/api/images/\(id)/\(kind.rawValue)")
    }
}
