import Foundation
import Testing
@testable import KinoraKit

@Suite struct PreferencesTests {
    func sub(_ i: Int, _ l: String, forced: Bool = false) -> SubtitleTrack {
        SubtitleTrack(idx: i, lang: l, title: nil, forced: forced, textBased: true, url: "/s/\(i)")
    }

    @Test func remembersLanguages() {
        let audio = [MediaStream(type: "audio", typeIndex: 0, lang: "eng"), MediaStream(type: "audio", typeIndex: 1, lang: "hun")]
        let subs = [sub(0, "eng"), sub(1, "hun", forced: true), sub(2, "hun")]
        #expect(LanguageChoice(audio: "hu").audioTrack(in: audio)?.typeIndex == 1)
        #expect(LanguageChoice(audio: "ger").audioTrack(in: audio) == nil)
        #expect(LanguageChoice(subtitle: "hun").subtitleTrack(in: subs)??.idx == 2)
        #expect(LanguageChoice(subtitle: "hu", subtitleForced: true).subtitleTrack(in: subs)??.idx == 1)
        let off = LanguageChoice(subtitle: "").subtitleTrack(in: subs)
        #expect(off != nil && off! == nil)
        #expect(LanguageChoice(subtitle: "fra").subtitleTrack(in: subs) == nil)
        #expect(LanguageChoice().subtitleTrack(in: subs) == nil)
    }

    @Test func speedsAndChapters() {
        #expect(PlaybackSpeeds.label(1) == "1×")
        #expect(PlaybackSpeeds.label(1.25) == "1\(Language.shared.decimalSeparator)25×") // the app language's separator
        let ch = [Chapter(start: 0, title: "A"), Chapter(start: 60, title: "B")]
        #expect(ch.current(at: 30)?.title == "A")
        #expect(ch.current(at: 59.7)?.title == "B")
        #expect(SubtitleStyle().points == 20)
    }

    @Test func decodesDownloads() throws {
        let json = #"{"jobs":[{"key":"k1","item_id":5,"status":"running","progress":0.4}],"qualities":[{"name":"720p","label":"720p"}]}"#
        let d = JSONDecoder(); d.keyDecodingStrategy = .convertFromSnakeCase
        let list = try d.decode(DownloadList.self, from: Data(json.utf8))
        #expect(list.jobs.first?.itemId == 5 && list.jobs.first?.isDone == false)
    }
}
