import Foundation
import Testing
@testable import KinoraKit

@Suite struct LanguageTests {
    func temp() -> (Language, URL, UserDefaults) {
        let dir = FileManager.default.temporaryDirectory.appendingPathComponent("lang-\(UUID().uuidString)")
        let suite = "lang-test-\(UUID().uuidString)"
        let d = UserDefaults(suiteName: suite)!
        return (Language(dir: dir, defaults: d), dir, d)
    }

    let en = LanguagePack(code: "en", name: "English", version: "v1", strings: .init(
        locale: "en-GB",
        app: ["Beállítások": .text("Settings"),
              "{0} cím": .plural(["one": "{0} title", "other": "{0} titles", "=0": "No titles"]),
              "Évad {0:00}": .text("Season {0:00}"),
              "Üres": .text("")],
        srv: ["nincs ilyen nyelv": "no such language", "hiba": "error"],
        srvRe: [["^(.+) csatlakozott$", "{0} joined"]]))

    @Test func sourceLanguageNeedsNoFile() {
        let (l, _, _) = temp()
        #expect(l.code == "hu")
        #expect(l.text("Beállítások", []) == "Beállítások")
        #expect(l.text("{0} cím", [3]) == "3 cím")
        #expect(l.server("nincs ilyen nyelv") == "nincs ilyen nyelv")
    }

    @Test func downloadedLanguage() throws {
        let (l, _, _) = temp()
        try l.savePack(en)
        l.saveIndex([LanguageInfo(code: "en", name: "English", version: "v1")], downloaded: ["en": "v1"])
        l.choice = "en"
        #expect(l.code == "en")
        #expect(l.cachedVersion("en") == "v1")
        #expect(l.text("Beállítások", []) == "Settings")
        #expect(l.text("{0} cím", [1]) == "1 title")
        #expect(l.text("{0} cím", [5]) == "5 titles")
        #expect(l.text("{0} cím", [0]) == "No titles")
        #expect(l.text("Évad {0:00}", [3]) == "Season 03")
        #expect(l.text("Üres", []) == "Üres") // an empty translation falls back to the source
        #expect(l.text("Nincs fordítás", []) == "Nincs fordítás")
        #expect(l.server("nincs ilyen nyelv") == "no such language")
        #expect(l.server("Anna csatlakozott") == "Anna joined")
        #expect(l.server("hiba: nincs ilyen nyelv") == "error: no such language")
        #expect(l.server("ismeretlen") == "ismeretlen")
        // a choice that is not on the device falls back to the system language
        l.choice = "fr"
        #expect(l.code == Language.resolve(choice: "", available: ["hu", "en"]))
    }

    @Test func indexKeepsDownloadedLanguages() throws {
        let (l, _, _) = temp()
        try l.savePack(en)
        l.saveIndex([LanguageInfo(code: "en", name: "English", version: "v1"), LanguageInfo(code: "de", name: "Deutsch", version: "v9")], downloaded: ["en": "v1"])
        #expect(l.available.map(\.code) == ["hu", "en"])
        #expect(l.known.map(\.code) == ["hu", "en", "de"])
        #expect(l.cachedVersion("de") == nil)
        // the server dropped English: the device keeps it
        l.saveIndex([], downloaded: [:])
        #expect(l.available.map(\.code) == ["hu", "en"])
        #expect(l.cachedVersion("en") == "v1")
    }

    @Test func resolve() {
        #expect(Language.resolve(choice: "de", available: ["hu", "en", "de"], preferred: ["en-US"]) == "de")
        #expect(Language.resolve(choice: "", available: ["hu", "en", "de"], preferred: ["de-AT", "en"]) == "de")
        #expect(Language.resolve(choice: "", available: ["hu", "en", "pt-br"], preferred: ["pt-BR"]) == "pt-br")
        #expect(Language.resolve(choice: "", available: ["hu", "en"], preferred: ["fr-FR"]) == "en")
        #expect(Language.resolve(choice: "", available: ["hu"], preferred: ["fr-FR"]) == "hu")
    }

    @Test func plurals() {
        #expect(Plural.category(1, "en-GB") == "one")
        #expect(Plural.category(2, "de") == "other")
        #expect(Plural.category(0, "fr-FR") == "one")
        #expect(Plural.category(3, "ru") == "few")
        #expect(Plural.category(5, "ru") == "many")
        #expect(Plural.category(21, "ru") == "one")
        #expect(Plural.category(22, "pl") == "few")
        #expect(Plural.category(7, "ja") == "other")
    }

    @Test func lenientDecoding() throws {
        let json = #"{"code":"fr","name":"Français","version":"x","strings":{"locale":"fr-FR","app":{"a":"b","bad":5,"p":{"one":"un","other":"des"},"noOther":{"one":"x"}},"srv":{"x":"y","n":1},"srvRe":[["^a$","b"],5,["^c$"]],"home":"Accueil"}}"#
        let p = try JSONDecoder().decode(LanguagePack.self, from: Data(json.utf8))
        #expect(p.strings.app == ["a": .text("b"), "p": .plural(["one": "un", "other": "des"])])
        #expect(p.strings.srv == ["x": "y"])
        #expect(p.strings.srvRe == [["^a$", "b"], ["^c$"]])
    }
}
