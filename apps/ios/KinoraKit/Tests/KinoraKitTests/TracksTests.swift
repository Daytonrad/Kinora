import Foundation
import Testing
@testable import KinoraKit

@Suite struct TracksTests {
    @Test func parsesVTT() {
        let src = "WEBVTT\r\n\r\n1\r\n00:00:01.000 --> 00:00:02.500 line:90%\r\n<i>Szia</i> &amp; hello\r\n\r\n01:02.000 --> 01:03.000\nKét\nsor\n"
        let cues = WebVTT.parse(src)
        #expect(cues == [Cue(start: 1, end: 2.5, text: "Szia & hello"), Cue(start: 62, end: 63, text: "Két\nsor")])
        #expect(WebVTT.text(at: 1.2, in: cues) == "Szia & hello")
        #expect(WebVTT.text(at: 3, in: cues) == nil)
    }

    @Test func vttReferencesDecodeOnce() {
        let cues = WebVTT.parse("WEBVTT\n\n00:00:01.000 --> 00:00:02.000\n&amp;lt;b&amp;gt; &lt;3 &#39;x&#x27; &#x1F600; &bogus; & a\n")
        #expect(cues.first?.text == "&lt;b&gt; <3 'x' \u{1F600} &bogus; & a")
    }

    @Test func picksDefaultSubtitle() {
        func t(_ i: Int, _ l: String, forced: Bool = false) -> SubtitleTrack {
            SubtitleTrack(idx: i, lang: l, title: nil, forced: forced, textBased: true, url: "/s/\(i)")
        }
        let tracks = [t(0, "eng"), t(1, "hun"), t(2, "eng", forced: true)]
        #expect(defaultSubtitle(tracks, mode: "off", langs: "hu") == nil)
        #expect(defaultSubtitle(tracks, mode: "always", langs: "hu,en")?.idx == 1)
        #expect(defaultSubtitle(tracks, mode: "auto", langs: "hu")?.idx == 2)
        #expect(defaultSubtitle([t(0, "eng")], mode: "auto", langs: "hu") == nil)
    }

    @Test func decodesStreams() throws {
        let json = #"{"type":"audio","type_index":1,"codec":"eac3","lang":"hun","channels":6,"default":true}"#
        let d = JSONDecoder(); d.keyDecodingStrategy = .convertFromSnakeCase
        let s = try d.decode(MediaStream.self, from: Data(json.utf8))
        #expect(s.typeIndex == 1 && s.isDefault == true)
        #expect(TrackLabel.audio(s, locale: Locale(identifier: "hu")) == "Magyar · EAC3 5.1")
    }
}
