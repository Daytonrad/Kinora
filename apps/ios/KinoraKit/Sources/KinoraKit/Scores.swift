import Foundation

/// One site's rating of a title (GET /api/items/{id}/scores). `value` is on the site's own
/// scale, 0…`max`: IMDb and TMDB out of 10, Rotten Tomatoes and Metacritic out of 100.
public struct Score: Codable, Sendable, Hashable, Identifiable {
    public var source: String
    public var value: Double
    public var max: Double
    public var votes: Int64?
    public var url: String?

    public var id: String { source }

    public init(source: String, value: Double, max: Double, votes: Int64? = nil, url: String? = nil) {
        self.source = source
        self.value = value
        self.max = max
        self.votes = votes
        self.url = url
    }

    /// The site's name as it writes it.
    public var siteName: String {
        switch source {
        case "imdb": "IMDb"
        case "rt": "Rotten Tomatoes"
        case "metacritic": "Metacritic"
        case "tmdb": "TMDB"
        default: source
        }
    }

    /// 0…1 for the meter.
    public var fraction: Double { max > 0 ? Swift.min(Swift.max(value / max, 0), 1) : 0 }

    /// Rated by critics (Rotten Tomatoes, Metacritic) rather than by viewers.
    public var isCritics: Bool { source == "rt" || source == "metacritic" }
}

/// The reception of a film or series: other sites' ratings and, for films, the budget and the
/// worldwide gross in USD (from TMDB).
public struct Scores: Codable, Sendable, Hashable {
    public var ratings: [Score]
    public var budget: Int64?
    public var revenue: Int64?
    /// The server has an OMDb key (IMDb, Rotten Tomatoes and Metacritic need one).
    public var omdb: Bool

    public init(ratings: [Score] = [], budget: Int64? = nil, revenue: Int64? = nil, omdb: Bool = false) {
        self.ratings = ratings
        self.budget = budget
        self.revenue = revenue
        self.omdb = omdb
    }

    /// The budget and the gross are worth showing.
    public var hasBoxOffice: Bool { (budget ?? 0) > 0 || (revenue ?? 0) > 0 }

    /// Gross ÷ budget, when both are known.
    public var ratio: Double? {
        guard let b = budget, let r = revenue, b > 0, r > 0 else { return nil }
        return Double(r) / Double(b)
    }
}

extension KinoraClient {
    /// Ratings and box office of a film or series in the library.
    public func itemScores(_ id: Int) async throws -> Scores { try await get("/api/items/\(id)/scores") }

    /// Ratings and box office of a TMDB title (Felfedezés).
    public func discoverScores(_ tmdbId: Int, series: Bool) async throws -> Scores {
        try await get("/api/discover/\(series ? "series" : "movies")/\(tmdbId)/scores")
    }
}
