import Foundation

// Felfedezés and requests: a film or series from TMDB the server does not have yet can be asked for;
// once an admin approves it (or the server does it on its own), the server downloads it into a library.
// Only when /api/me says `can_request` (the acquisition module is on and this is not a kids profile).

/// A title on a discover list or in the discover search (GET /api/discover, /api/discover/search).
public struct DiscoverItem: Codable, Sendable, Identifiable, Hashable {
    /// The TMDB id.
    public var id: Int
    public var title: String
    public var year: Int?
    public var overview: String?
    /// Full image URL (image.tmdb.org).
    public var poster: String?
    public var rating: Double?
    /// movie | series
    public var kind: String?
    /// Already in a library: open this item.
    public var itemId: Int?
    /// State of the newest request for it.
    public var request: String?
    public var followed: Bool?

    public init(id: Int, title: String, kind: String = "movie") {
        self.id = id; self.title = title; self.kind = kind
    }

    public var isSeries: Bool { kind == "series" }
    /// Unique across films and series (the TMDB ids of the two overlap).
    public var key: String { "\(kind ?? "movie")/\(id)" }
}

/// A streaming service (TMDB, data by JustWatch).
public struct WatchProvider: Codable, Sendable, Hashable, Identifiable {
    public var id: Int
    public var name: String
    public var logo: String?
}

/// Where a title streams in the server's region.
public struct WhereToWatch: Codable, Sendable, Hashable {
    public var region: String
    public var link: String?
    public var stream: [WatchProvider]?
    public var rent: [WatchProvider]?
    public var buy: [WatchProvider]?
}

/// One request (GET /api/requests, and the request on a discover detail).
public struct AcqRequest: Codable, Sendable, Identifiable, Hashable {
    public enum Status: String, Codable, Sendable {
        case pending, approved, waiting, downloading, available, denied, failed, canceled, unknown
        public init(from decoder: Decoder) throws {
            self = Status(rawValue: try decoder.singleValueContainer().decode(String.self)) ?? .unknown
        }
    }

    public var id: Int
    public var kind: String
    public var tmdbId: Int
    public var title: String
    public var year: Int?
    public var poster: String?
    public var userId: Int?
    public var userName: String?
    public var status: Status
    public var note: String?
    /// Series: "all", "future" or "1,2".
    public var seasons: String?
    public var libraryName: String?
    public var createdAt: Int?
    /// The running download, 0…1.
    public var progress: Double?

    public init(id: Int, kind: String = "movie", tmdbId: Int, title: String, status: Status) {
        self.id = id; self.kind = kind; self.tmdbId = tmdbId; self.title = title; self.status = status
    }

    public var isSeries: Bool { kind == "series" }
    /// Still being worked on.
    public var isOpen: Bool { [.pending, .approved, .waiting, .downloading].contains(status) }
    /// The requester may still take it back (a running download only an admin can stop).
    public var cancellable: Bool { [.pending, .approved, .waiting].contains(status) }
}

extension AcqRequest.Status {
    public var label: String {
        switch self {
        case .pending: L("Jóváhagyásra vár")
        case .approved: L("Jóváhagyva, keresés…")
        case .waiting: L("Várakozik")
        case .downloading: L("Letöltés alatt")
        case .available: L("Megérkezett")
        case .denied: L("Elutasítva")
        case .failed: L("Sikertelen")
        case .canceled: L("Visszavonva")
        case .unknown: L("Ismeretlen")
        }
    }

    /// A list state string ("pending"…) as a status; nil when empty.
    public init?(state: String?) {
        guard let state, !state.isEmpty else { return nil }
        self = AcqRequest.Status(rawValue: state) ?? .unknown
    }
}

/// "all", "future" or "1,2" in words.
public func seasonsLabel(_ s: String?) -> String {
    switch s ?? "" {
    case "", "all": return L("Minden évad")
    case "future": return L("Csak az új részek")
    case let list:
        let nums = list.split(separator: ",").map { $0.trimmingCharacters(in: .whitespaces) }
        return nums.map { L("{0}. évad", $0) }.joined(separator: ", ")
    }
}

/// GET /api/discover/movies/{tmdb id}
public struct DiscoverMovie: Codable, Sendable {
    public var id: Int
    public var title: String
    public var originalTitle: String?
    public var year: Int?
    public var overview: String?
    public var poster: String?
    public var backdrop: String?
    public var rating: Double?
    public var runtime: Int?
    public var genres: [String]?
    public var watch: WhereToWatch?
    public var itemId: Int?
    /// The open request, or the newest closed one.
    public var request: AcqRequest?
}

/// One season of a series on TMDB, with what the library has of it.
public struct SeasonState: Codable, Sendable, Hashable, Identifiable {
    public var number: Int
    public var name: String?
    public var episodeCount: Int?
    public var airDate: String?
    /// Episodes already in the library.
    public var have: Int?
    public var id: Int { number }
}

/// GET /api/discover/series/{tmdb id}
public struct DiscoverSeries: Codable, Sendable {
    public var id: Int
    public var title: String
    public var originalTitle: String?
    public var year: Int?
    public var overview: String?
    public var poster: String?
    public var backdrop: String?
    public var ended: Bool?
    public var genres: [String]?
    public var watch: WhereToWatch?
    public var itemId: Int?
    public var request: AcqRequest?
    public var followed: Bool?
    public var seasons: [SeasonState]?
}

/// GET /api/discover/libraries: where a request can go.
public struct RequestLibraries: Codable, Sendable {
    public struct Lib: Codable, Sendable, Hashable, Identifiable {
        public var id: Int
        public var name: String
    }
    public var movies: [Lib]
    public var shows: [Lib]
}

/// The TMDB lists the discover page shows.
public enum DiscoverList: String, CaseIterable, Sendable {
    case trending, upcoming, popular, top
    case tvTrending = "tv_trending", tvAiring = "tv_airing", tvPopular = "tv_popular"

    public static let movies: [DiscoverList] = [.trending, .upcoming, .popular, .top]
    public static let series: [DiscoverList] = [.tvTrending, .tvAiring, .tvPopular]

    public var title: String {
        switch self {
        case .trending: L("Felkapott a héten")
        case .upcoming: L("Hamarosan a mozikban")
        case .popular: L("Népszerű")
        case .top: L("Minden idők legjobbjai")
        case .tvTrending: L("Felkapott sorozatok")
        case .tvAiring: L("Most futó sorozatok")
        case .tvPopular: L("Népszerű sorozatok")
        }
    }
}

private struct RequestBody: Encodable, Sendable {
    var kind: String
    var tmdbId: Int
    var seasons: String?
    var libraryId: Int?
}

extension KinoraClient {
    private func q(_ s: String) -> String { s.addingPercentEncoding(withAllowedCharacters: .urlQueryValueAllowed) ?? s }

    public func discover(_ list: DiscoverList) async throws -> [DiscoverItem] {
        try await get("/api/discover?list=\(list.rawValue)")
    }

    /// `series` false: films.
    public func discoverSearch(_ query: String, series: Bool) async throws -> [DiscoverItem] {
        try await get("/api/discover/search?kind=\(series ? "series" : "movie")&q=\(q(query))")
    }

    public func discoverMovie(_ tmdbId: Int) async throws -> DiscoverMovie {
        try await get("/api/discover/movies/\(tmdbId)")
    }

    public func discoverSeries(_ tmdbId: Int) async throws -> DiscoverSeries {
        try await get("/api/discover/series/\(tmdbId)")
    }

    public func requestLibraries() async throws -> RequestLibraries {
        try await get("/api/discover/libraries")
    }

    /// `library` nil: the server's default (only when there is a single library).
    public func requestMovie(_ tmdbId: Int, library: Int?) async throws -> AcqRequest {
        try await post("/api/requests", RequestBody(kind: "movie", tmdbId: tmdbId, libraryId: library))
    }

    /// `seasons`: "all", "future" or "1,2".
    public func requestSeries(_ tmdbId: Int, seasons: String, library: Int?) async throws -> AcqRequest {
        try await post("/api/requests", RequestBody(kind: "series", tmdbId: tmdbId, seasons: seasons, libraryId: library))
    }

    public func myRequests() async throws -> [AcqRequest] { try await get("/api/requests") }

    public func cancelRequest(_ id: Int) async throws { try await delete("/api/requests/\(id)") }

    /// Fulfilled requests the user was not told about yet; asking marks them told.
    public func unseenRequests() async throws -> [AcqRequest] { try await get("/api/requests/unseen") }
}
