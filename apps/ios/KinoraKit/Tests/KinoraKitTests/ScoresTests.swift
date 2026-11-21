import Foundation
import Testing
@testable import KinoraKit

// Shape of GET /api/items/{id}/scores and /api/discover/…/scores (internal/meta/scores.go).
@Suite struct ScoresTests {
    private func decode(_ json: String) throws -> Scores {
        try JSONDecoder.kinora.decode(Scores.self, from: Data(json.utf8))
    }

    @Test func film() throws {
        let s = try decode("""
        {"ratings":[{"source":"imdb","value":8.7,"max":10,"votes":2123456,"url":"https://www.imdb.com/title/tt0133093/"},
                    {"source":"rt","value":83,"max":100},{"source":"metacritic","value":73,"max":100},
                    {"source":"tmdb","value":8.2,"max":10,"votes":26000,"url":"https://www.themoviedb.org/movie/603"}],
         "budget":63000000,"revenue":467222728,"omdb":true}
        """)
        #expect(s.ratings.map(\.source) == ["imdb", "rt", "metacritic", "tmdb"])
        #expect(s.ratings[0].votes == 2_123_456 && s.ratings[0].siteName == "IMDb" && !s.ratings[0].isCritics)
        #expect(s.ratings[1].fraction == 0.83 && s.ratings[1].isCritics && s.ratings[1].url == nil)
        #expect(s.hasBoxOffice && s.omdb)
        #expect(abs((s.ratio ?? 0) - 7.416) < 0.01)
    }

    @Test func seriesWithoutOmdb() throws {
        let s = try decode(#"{"ratings":[{"source":"tmdb","value":8.4,"max":10,"votes":100}],"omdb":false}"#)
        #expect(s.ratings.count == 1 && !s.omdb && !s.hasBoxOffice && s.ratio == nil && s.budget == nil)
        #expect(try decode(#"{"ratings":[],"budget":5000000,"omdb":true}"#).ratio == nil)
    }
}
