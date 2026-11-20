import Foundation
import Testing
@testable import KinoraKit

// Shapes of the server's Felfedezés and request answers (internal/acquire/discover.go, store.AcqRequest).
@Suite struct RequestsTests {
    private func decode<T: Decodable>(_ json: String, as: T.Type) throws -> T {
        try JSONDecoder.kinora.decode(T.self, from: Data(json.utf8))
    }

    @Test func discoverItems() throws {
        let list = try decode("""
        [{"id":603,"title":"Mátrix","year":1999,"overview":"…","poster":"https://image.tmdb.org/t/p/w342/a.jpg","rating":8.2,"kind":"movie","item_id":12},
         {"id":1399,"title":"Trónok harca","year":2011,"overview":"","rating":8.4,"kind":"series","request":"downloading","followed":true}]
        """, as: [DiscoverItem].self)
        #expect(list[0].itemId == 12 && !list[0].isSeries && list[0].request == nil)
        #expect(list[1].isSeries && list[1].poster == nil && list[1].followed == true)
        #expect(AcqRequest.Status(state: list[1].request) == .downloading)
        #expect(AcqRequest.Status(state: "") == nil)
        #expect(list[0].key != DiscoverItem(id: 603, title: "x", kind: "series").key)
    }

    @Test func movieDetailWithRequest() throws {
        let d = try decode("""
        {"id":603,"imdb_id":"tt0133093","title":"Mátrix","original_title":"The Matrix","english_title":"The Matrix","year":1999,
         "overview":"…","rating":8.2,"runtime":136,"genres":["Akció","Sci-Fi"],
         "watch":{"region":"HU","link":"https://www.themoviedb.org/movie/603/watch","stream":[{"id":8,"name":"Netflix","logo":"https://x/n.jpg"}],"rent":[],"buy":null},
         "request":{"id":4,"kind":"movie","tmdb_id":603,"imdb_id":"tt0133093","title":"Mátrix","year":1999,"poster":"","user_id":2,"user_name":"anna",
                    "profile_id":null,"status":"pending","note":"","seasons":"","library_id":1,"library_name":"Filmek","attempts":0,"next_try_at":0,
                    "decided_by":null,"seen":false,"created_at":1763600000,"updated_at":1763600000,"progress":0}}
        """, as: DiscoverMovie.self)
        #expect(d.originalTitle == "The Matrix" && d.runtime == 136)
        #expect(d.watch?.stream?.first?.name == "Netflix" && d.watch?.buy == nil)
        #expect(d.request?.status == .pending && d.request?.cancellable == true && d.request?.isOpen == true)
        #expect(d.request?.userId == 2 && d.itemId == nil)
    }

    @Test func seriesDetail() throws {
        let d = try decode("""
        {"id":1399,"imdb_id":"","tvdb_id":121361,"title":"Trónok harca","original_title":"Game of Thrones","english_title":"","year":2011,
         "overview":"","ended":true,"genres":[],"followed":false,
         "seasons":[{"number":1,"name":"1. évad","episode_count":10,"air_date":"2011-04-17","have":3},{"number":2,"name":"","episode_count":10,"air_date":"","have":0}]}
        """, as: DiscoverSeries.self)
        #expect(d.ended == true && d.request == nil && d.watch == nil)
        #expect(d.seasons?.map(\.number) == [1, 2] && d.seasons?.first?.have == 3)
    }

    @Test func requestStates() throws {
        let list = try decode("""
        [{"id":1,"kind":"series","tmdb_id":1399,"title":"T","year":2011,"poster":"","user_id":null,"status":"downloading","seasons":"1,2","progress":0.4},
         {"id":2,"kind":"movie","tmdb_id":5,"title":"M","status":"someday"}]
        """, as: [AcqRequest].self)
        #expect(list[0].isOpen && !list[0].cancellable && list[0].progress == 0.4)
        #expect(list[1].status == .unknown && !list[1].isOpen)
        #expect(seasonsLabel("1,2") == "1. évad, 2. évad")
        #expect(seasonsLabel("future") == "Csak az új részek")
        #expect(seasonsLabel("all") == "Minden évad")
    }

    @Test func libraries() throws {
        let l = try decode(#"{"movies":[{"id":1,"name":"Filmek"}],"shows":[]}"#, as: RequestLibraries.self)
        #expect(l.movies.first?.name == "Filmek" && l.shows.isEmpty)
    }

    @Test func meCanRequest() throws {
        let me = try decode("""
        {"user":{"id":1,"username":"a","is_admin":true},"profile":null,"profiles":[],"device_id":"d","media_token":"t",
         "server_name":"S","version":"1.0.15","can_request":true}
        """, as: Me.self)
        #expect(me.canRequest == true)
    }
}
