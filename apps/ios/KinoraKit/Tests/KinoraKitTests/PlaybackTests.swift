import Foundation
import Testing
@testable import KinoraKit

struct PlaybackTests {
    @Test func directWithSegments() throws {
        let json = """
        {"audio_idx":0,"chapters":[{"Start":0,"Title":"Intro"}],"credits_start":50,"duration":60,"encoder":"",
         "item":{"episode_num":null,"id":8,"kind":"movie","season_num":null,"series_id":null,"series_title":"","title":"Big Test Movie"},
         "mode":"direct","reasons":[],"segments":[{"type":"intro","start":0,"end":50,"source":"chapter","confidence":1},
         {"type":"credits","start":52,"end":60,"source":"chapter","confidence":1},{"type":"sponsor","start":1,"end":2}],
         "source":{"hdr":""},"start_offset":0,"stream_token":"t",
         "subtitles":[{"codec":"subrip","forced":false,"idx":0,"lang":"hun","text_based":true,"title":"","url":"/api/items/8/subtitles/0?st=t"}],
         "url":"/api/stream/8?st=t"}
        """
        let p = try JSONDecoder.kinora.decode(PlaybackInfo.self, from: Data(json.utf8))
        #expect(p.mode == .direct)
        #expect(p.session == nil)
        #expect(p.segments?.map(\.type) == [.intro, .credits, .unknown])
        #expect(p.creditsAt == 52)
        #expect(p.chapters == [Chapter(start: 0, title: "Intro")])
        #expect(p.subtitles?.first?.lang == "hun")
        #expect(p.item.kind == .movie)
    }

    @Test func remuxEpisode() throws {
        let json = """
        {"audio_idx":0,"chapters":null,"credits_start":null,"duration":20,"encoder":"copy",
         "item":{"episode_num":1,"id":10,"kind":"episode","season_num":2,"series_id":3,"series_title":"Test Show","title":"1. rész"},
         "mode":"remux","reasons":["x"],"segments":[],"session":"abc","start_offset":12.5,"stream_token":"t",
         "subtitles":[],"url":"/api/hls/abc/index.m3u8?st=t"}
        """
        let p = try JSONDecoder.kinora.decode(PlaybackInfo.self, from: Data(json.utf8))
        #expect(p.mode == .remux)
        #expect(p.session == "abc")
        #expect(p.startOffset == 12.5)
        #expect(p.creditsAt == nil)
        #expect(p.item.seasonNum == 2)
    }

    @Test func titleLanguages() throws {
        // the server's starting subtitle and the series' saved choice come with the playback answer
        let json = """
        {"audio_idx":1,"sub_idx":-1,"title_langs":{"title_id":3,"audio_lang":"hun","sub_lang":"off"},"duration":20,
         "item":{"id":10,"kind":"episode","series_id":3,"title":"1. rész"},"mode":"direct","url":"/api/stream/10?st=t"}
        """
        let p = try JSONDecoder.kinora.decode(PlaybackInfo.self, from: Data(json.utf8))
        #expect(p.subIdx == -1 && p.titleLangs?.titleId == 3 && p.titleLangs?.subLang == TitleLangs.off)
        // an older server sends neither
        let old = try JSONDecoder.kinora.decode(PlaybackInfo.self, from: Data(#"{"mode":"direct","url":"/x","item":{"id":1,"kind":"movie","title":"M"}}"#.utf8))
        #expect(old.subIdx == nil && old.titleLangs == nil)
        let l = try JSONDecoder.kinora.decode(TitleLangs.self, from: Data(#"{"title_id":8,"audio_lang":"","sub_lang":"eng","audio":["eng","hun"],"subs":["eng"]}"#.utf8))
        #expect(l.audio == ["eng", "hun"] && l.subLang == "eng" && l.audioLang.isEmpty)
    }

    @Test func requestEncodesSnakeCase() throws {
        let body = PlaybackRequest(start: 30, force: "transcode", prevSession: "s1")
        let obj = try JSONSerialization.jsonObject(with: JSONEncoder.kinora.encode(body)) as? [String: Any]
        #expect(obj?["prev_session"] as? String == "s1")
        #expect(obj?["start"] as? Double == 30)
        #expect((obj?["caps"] as? [String: Any])?["containers"] as? [String] == ["mp4", "mov"])
    }

    @Test func nextEpisode() throws {
        let none = try JSONDecoder.kinora.decode(NextBody.self, from: Data(#"{"next":null}"#.utf8))
        #expect(none.next == nil)
        let some = try JSONDecoder.kinora.decode(NextBody.self, from: Data(#"{"next":{"id":12,"kind":"episode","title":"Pilot Part 2","season_num":1,"episode_num":2}}"#.utf8))
        #expect(some.next?.id == 12)
    }
}
