import Foundation
import Testing
@testable import KinoraKit

// Shapes copied from a real server's responses (trimmed).
@Suite struct BrowseTests {
    private func decode<T: Decodable>(_ json: String, as: T.Type) throws -> T {
        try JSONDecoder.kinora.decode(T.self, from: Data(json.utf8))
    }

    @Test func home() throws {
        let h = try decode("""
        {"hero":{"id":6,"library_id":1,"kind":"movie","title":"Space Drama","year":2015,"community_rating":8.1,
          "runtime_sec":40,"has_poster":false,"genres":["Sci-Fi"],
          "user_data":{"position":10,"played":false,"play_count":0,"last_played_at":null,"favorite":true,"stars":null,"hidden":false}},
         "rows":[{"id":"gems","title":"Rejtett kincsek","style":"poster","items":[{"id":6,"kind":"movie","title":"Space Drama"}],
                  "reasons":{"6":"magasra értékelt"}},
                 {"id":"x","title":"X","style":"wide","items":[]}]}
        """, as: HomeData.self)
        #expect(h.hero?.communityRating == 8.1)
        #expect(h.hero?.userData?.favorite == true)
        #expect(h.hero?.progress == 0.25)
        #expect(h.rows[0].reasons?["6"] == "magasra értékelt")
        #expect(h.rows[1].style == .poster) // unknown styles fall back
    }

    @Test func seriesDetail() throws {
        let d = try decode("""
        {"collections":[],"item":{"id":1,"kind":"series","title":"Other Series","episode_count":1,"unplayed_count":1},
         "play_next":{"id":13,"kind":"episode","parent_id":2,"series_id":1,"title":"1. rész","season_num":1,"episode_num":3,
                      "series_title":"Other Series"},
         "seasons":[{"id":2,"kind":"season","title":"1. évad","season_num":1}],"similar":[]}
        """, as: ItemDetail.self)
        #expect(d.item.kind == .series)
        #expect(d.playNext?.episodeLabel == "S1 · E3")
        #expect(d.seasons?.first?.seasonNum == 1)
    }

    @Test func unknownKindAndMissingUserData() throws {
        let i = try decode(#"{"id":3,"kind":"trailer","title":"T"}"#, as: Item.self)
        #expect(i.kind == .unknown)
        #expect(i.progress == nil)
        let u = try decode("{}", as: UserData.self)
        #expect(u.played == false && u.position == 0)
    }
}
