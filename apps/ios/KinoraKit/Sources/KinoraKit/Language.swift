import Foundation

// The apps' texts are written in Hungarian in the source; every other language comes from the server, in the
// same language files the web client uses (their "app" table, keyed by the Hungarian text). Downloaded languages
// are kept in the App Group container, so the widgets and the Top Shelf see them too, and they stay usable when the
// server is offline or later drops the language.

/// One language the server offers (or offered: the device keeps what it downloaded).
public struct LanguageInfo: Codable, Sendable, Hashable, Identifiable {
    public var code: String
    public var name: String
    public var locale: String?
    public var version: String?
    public var builtin: Bool?
    public var id: String { code }

    public init(code: String, name: String, locale: String? = nil, version: String? = nil, builtin: Bool? = nil) {
        self.code = code; self.name = name; self.locale = locale; self.version = version; self.builtin = builtin
    }
}

/// A language file as the server sends it (GET /api/languages/{code}).
public struct LanguagePack: Codable, Sendable {
    public var code: String
    public var name: String
    public var version: String?
    public var strings: Strings

    public struct Strings: Codable, Sendable {
        public var locale: String?
        public var app: [String: Entry]
        public var srv: [String: String]
        public var srvRe: [[String]]

        public init(locale: String? = nil, app: [String: Entry] = [:], srv: [String: String] = [:], srvRe: [[String]] = []) {
            self.locale = locale; self.app = app; self.srv = srv; self.srvRe = srvRe
        }

        // Lenient: a malformed entry is dropped, not the whole language.
        public init(from decoder: Decoder) throws {
            let c = try decoder.container(keyedBy: CodingKeys.self)
            locale = try? c.decode(String.self, forKey: .locale)
            app = ((try? c.decode([String: Lenient<Entry>].self, forKey: .app)) ?? [:]).compactMapValues(\.value)
            srv = ((try? c.decode([String: Lenient<String>].self, forKey: .srv)) ?? [:]).compactMapValues(\.value)
            srvRe = ((try? c.decode([Lenient<[Lenient<String>]>].self, forKey: .srvRe)) ?? [])
                .compactMap { $0.value?.compactMap(\.value) }.filter { $0.count >= 1 }
        }
    }

    /// A text, or its plural forms ("one", "other", "=0"…) chosen by the first argument.
    public enum Entry: Codable, Sendable, Equatable {
        case text(String)
        case plural([String: String])

        public init(from decoder: Decoder) throws {
            let c = try decoder.singleValueContainer()
            if let s = try? c.decode(String.self) { self = .text(s); return }
            let forms = try c.decode([String: Lenient<String>].self).compactMapValues(\.value)
            guard forms["other"] != nil else { throw DecodingError.dataCorruptedError(in: c, debugDescription: "no other form") }
            self = .plural(forms)
        }

        public func encode(to encoder: Encoder) throws {
            var c = encoder.singleValueContainer()
            switch self {
            case .text(let s): try c.encode(s)
            case .plural(let f): try c.encode(f)
            }
        }
    }

    public init(code: String, name: String, version: String? = nil, strings: Strings) {
        self.code = code; self.name = name; self.version = version; self.strings = strings
    }
}

struct Lenient<T: Decodable & Sendable>: Decodable, Sendable {
    let value: T?
    init(from decoder: Decoder) throws { value = try? T(from: decoder) }
}

/// The current language of the app and its extensions. Thread-safe; `L(…)` reads it.
public final class Language: @unchecked Sendable {
    public static let shared = Language()

    /// The source language: its texts are the keys, so it needs no file.
    public static let source = LanguageInfo(code: "hu", name: "Magyar", locale: "hu-HU", builtin: true)
    static let choiceKey = "kinora.language"
    static let stampKey = "kinora.language.stamp"

    private let lock = NSLock()
    private let dir: URL?
    private let defaults: UserDefaults
    private var compiled = Compiled.source
    private var loadedStamp = -1

    init(dir: URL? = Language.defaultDir(), defaults: UserDefaults = AppGroup.defaults ?? .standard) {
        self.dir = dir
        self.defaults = defaults
        reload()
    }

    static func defaultDir() -> URL? {
        let base = AppGroup.container ?? FileManager.default.urls(for: .applicationSupportDirectory, in: .userDomainMask).first
        return base?.appendingPathComponent("Languages", isDirectory: true)
    }

    // MARK: - state

    /// The user's choice: a language code, or "" to follow the system language.
    public var choice: String {
        get { defaults.string(forKey: Self.choiceKey) ?? "" }
        set {
            defaults.set(newValue, forKey: Self.choiceKey)
            changed()
        }
    }

    /// The languages the device can show: the source language and every downloaded one.
    public var available: [LanguageInfo] {
        let list = readIndex().filter { hasPack($0.code) && $0.code != Self.source.code }
        return [Self.source] + list
    }

    /// The languages the server listed last, downloaded or not (the picker offers them; picking downloads).
    public var known: [LanguageInfo] {
        let list = readIndex().filter { $0.code != Self.source.code }
        return [Self.source] + list
    }

    /// The language in use: the choice when it is on the device, else the best system language, else English or Hungarian.
    public var code: String { lock.withLock { compiled.code } }
    public var locale: Locale { Locale(identifier: lock.withLock { compiled.locale }) }
    public var decimalSeparator: String { locale.decimalSeparator ?? "." }

    public static func resolve(choice: String, available: [String], preferred: [String] = Locale.preferredLanguages) -> String {
        if !choice.isEmpty, available.contains(choice) { return choice }
        for p in preferred {
            let l = p.lowercased()
            if available.contains(l) { return l }
            let parts = l.split(separator: "-")
            if parts.count > 1, available.contains("\(parts[0])-\(parts[1])") { return "\(parts[0])-\(parts[1])" }
            if let base = parts.first, available.contains(String(base)) { return String(base) }
        }
        return available.contains("en") ? "en" : Self.source.code
    }

    /// Re-read the language from disk (another process changed it, or a sync saved a new version).
    public func reload() {
        let stamp = defaults.integer(forKey: Self.stampKey)
        let codes = available.map(\.code)
        let code = Self.resolve(choice: choice, available: codes)
        // Nothing downloaded yet (no server reached): the first screens use the few texts built into the app.
        let firstRun = codes == [Self.source.code]
        let bundledCode = Self.resolve(choice: choice, available: [Self.source.code] + Self.bundled.keys.sorted())
        let next: Compiled
        if code != Self.source.code, let pack = readPack(code) {
            next = Compiled(pack)
        } else if firstRun, let pack = Self.bundled[bundledCode] {
            next = Compiled(pack)
        } else if let pack = readPack(Self.source.code) {
            next = Compiled(pack) // an admin may have overridden some Hungarian texts
        } else {
            next = .source
        }
        lock.withLock { compiled = next; loadedStamp = stamp }
    }

    /// For extensions: picks up a change the app made since this process last looked.
    public func reloadIfNeeded() {
        let stamp = defaults.integer(forKey: Self.stampKey)
        if lock.withLock({ stamp != loadedStamp }) { reload() }
    }

    private func changed() {
        defaults.set(defaults.integer(forKey: Self.stampKey) &+ 1, forKey: Self.stampKey)
        reload()
    }

    // MARK: - storage

    private var indexURL: URL? { dir?.appendingPathComponent("index.json") }
    private func packURL(_ code: String) -> URL? { dir?.appendingPathComponent("\(code).json") }

    func readIndex() -> [LanguageInfo] {
        guard let u = indexURL, let d = try? Data(contentsOf: u) else { return [] }
        return (try? JSONDecoder().decode([LanguageInfo].self, from: d)) ?? []
    }

    func readPack(_ code: String) -> LanguagePack? {
        guard let u = packURL(code), let d = try? Data(contentsOf: u) else { return nil }
        return try? JSONDecoder().decode(LanguagePack.self, from: d)
    }

    func hasPack(_ code: String) -> Bool {
        guard let u = packURL(code) else { return false }
        return FileManager.default.fileExists(atPath: u.path)
    }

    /// The version of a downloaded language, to skip unchanged ones.
    public func cachedVersion(_ code: String) -> String? {
        readIndex().first { $0.code == code && hasPack(code) }?.version
    }

    private func write(_ data: Data, to url: URL) throws {
        try FileManager.default.createDirectory(at: url.deletingLastPathComponent(), withIntermediateDirectories: true)
        try data.write(to: url, options: .atomic)
    }

    /// Saves the server's list. Languages the device already downloaded stay, even if the server no longer has them.
    /// `versions` maps a code to the version that is on disk (nil = keep what the index says).
    func saveIndex(_ server: [LanguageInfo], downloaded: [String: String]) {
        guard let u = indexURL else { return }
        let old = readIndex()
        var out: [LanguageInfo] = []
        for var l in server {
            let onDisk = downloaded[l.code] ?? old.first { $0.code == l.code }?.version
            l.version = hasPack(l.code) ? onDisk : nil
            out.append(l)
        }
        for l in old where !out.contains(where: { $0.code == l.code }) && hasPack(l.code) {
            out.append(l)
        }
        if let d = try? JSONEncoder().encode(out) { try? write(d, to: u) }
    }

    func savePack(_ pack: LanguagePack) throws {
        guard let u = packURL(pack.code) else { return }
        try write(try JSONEncoder().encode(pack), to: u)
    }

    // MARK: - sync

    /// Downloads the server's languages that are new or changed. Returns true when the language in use changed.
    @discardableResult
    public func sync(_ client: KinoraClient) async -> Bool {
        struct List: Decodable, Sendable { let languages: [LanguageInfo] }
        guard let list = try? await client.get("/api/languages", as: List.self) else { return false } // older server
        let before = (code, cachedVersion(code))
        var downloaded: [String: String] = [:]
        // index first, so a pack written below is listed with its version
        saveIndex(list.languages, downloaded: [:])
        for l in list.languages {
            guard let v = l.version, v != cachedVersion(l.code) else { continue }
            guard let pack = try? await client.get("/api/languages/\(l.code)", as: LanguagePack.self), pack.code == l.code else { continue }
            if (try? savePack(pack)) != nil { downloaded[l.code] = v }
        }
        if downloaded.isEmpty { saveIndex(list.languages, downloaded: [:]); return false }
        saveIndex(list.languages, downloaded: downloaded)
        changed()
        return before != (code, cachedVersion(code))
    }

    // MARK: - lookup

    /// The text for a Hungarian source text, with {0}, {1}… filled in from `args`.
    public func text(_ key: String, _ args: [Any]) -> String {
        let c = lock.withLock { compiled }
        let template: String
        switch c.app[key] {
        case .text(let s)?: template = s.isEmpty ? key : s
        case .plural(let forms)?: template = Plural.pick(forms, args.first, locale: c.locale) ?? key
        case nil: template = key
        }
        return args.isEmpty && !template.contains("{") ? template : Self.fill(template, args)
    }

    /// Translates a message the server sent in Hungarian (errors, row titles), like the web client's srv().
    public func server(_ msg: String) -> String {
        let c = lock.withLock { compiled }
        guard c.code != Self.source.code, !msg.isEmpty else { return msg }
        if let t = c.srv[msg], !t.isEmpty { return t }
        let ns = msg as NSString
        for (re, tpl) in c.srvRe {
            guard let m = re.firstMatch(in: msg, range: NSRange(location: 0, length: ns.length)) else { continue }
            let groups: [Any] = (1..<max(1, m.numberOfRanges)).map { i in
                let r = m.range(at: i)
                return r.location == NSNotFound ? "" : ns.substring(with: r)
            }
            return Self.fill(tpl, groups)
        }
        if let r = msg.range(of: ": "), r.lowerBound > msg.startIndex, let head = c.srv[String(msg[..<r.lowerBound])] {
            return head + ": " + server(String(msg[r.upperBound...]))
        }
        return msg
    }

    static func fill(_ template: String, _ args: [Any]) -> String {
        var out = ""
        var i = template.startIndex
        while i < template.endIndex {
            let ch = template[i]
            if ch == "{", let close = template[i...].firstIndex(of: "}") {
                let inner = template[template.index(after: i)..<close]
                let parts = inner.split(separator: ":", maxSplits: 1, omittingEmptySubsequences: false)
                if let n = Int(parts[0]), parts[0].count == 1, parts.count == 1 || parts[1].allSatisfy({ $0 == "0" }) {
                    var v = n < args.count ? describe(args[n]) : ""
                    if parts.count == 2, v.count < parts[1].count { v = String(repeating: "0", count: parts[1].count - v.count) + v }
                    out += v
                    i = template.index(after: close)
                    continue
                }
            }
            out.append(ch)
            i = template.index(after: i)
        }
        return out
    }

    static func describe(_ v: Any) -> String {
        switch v {
        case let s as String: return s
        case let s as Substring: return String(s)
        case let a as [String]: return a.joined(separator: ", ")
        default:
            let m = Mirror(reflecting: v)
            if m.displayStyle == .optional { return m.children.first.map { describe($0.value) } ?? "" }
            return "\(v)"
        }
    }

    struct Compiled {
        var code: String
        var locale: String
        var app: [String: LanguagePack.Entry]
        var srv: [String: String]
        var srvRe: [(NSRegularExpression, String)]

        static var source: Compiled { Compiled(code: Language.source.code, locale: "hu-HU", app: [:], srv: [:], srvRe: []) }

        init(code: String, locale: String, app: [String: LanguagePack.Entry], srv: [String: String], srvRe: [(NSRegularExpression, String)]) {
            self.code = code; self.locale = locale; self.app = app; self.srv = srv; self.srvRe = srvRe
        }

        init(_ p: LanguagePack) {
            code = p.code
            locale = p.strings.locale ?? p.code
            app = p.strings.app
            srv = p.strings.srv
            srvRe = p.strings.srvRe.compactMap { pair in
                guard let re = try? NSRegularExpression(pattern: pair[0], options: [.dotMatchesLineSeparators]) else { return nil }
                return (re, pair.count > 1 ? pair[1] : "")
            }
        }
    }
}

/// Plural categories (CLDR, integers only) for the languages a translator is likely to add.
enum Plural {
    static func pick(_ forms: [String: String], _ arg: Any?, locale: String) -> String? {
        let n: Double? = switch arg {
        case let i as Int: Double(i)
        case let d as Double: d
        case let s as String: Double(s.replacingOccurrences(of: ",", with: "."))
        case let i as any BinaryInteger: Double(Int(i))
        default: nil
        }
        guard let n else { return forms["other"] }
        if n == n.rounded(), let exact = forms["=\(Int(n))"], !exact.isEmpty { return exact }
        let cat = category(n, locale)
        if let f = forms[cat], !f.isEmpty { return f }
        return forms["other"]
    }

    static func category(_ n: Double, _ locale: String) -> String {
        let lang = locale.lowercased().split(whereSeparator: { $0 == "-" || $0 == "_" }).first.map(String.init) ?? ""
        let i = Int(n.magnitude)
        let whole = n == n.rounded()
        switch lang {
        case "ja", "zh", "ko", "vi", "th", "id", "ms", "lo", "my":
            return "other"
        case "fr":
            return i == 0 || i == 1 ? "one" : "other"
        case "pt":
            if locale.lowercased().hasPrefix("pt-pt") { return n == 1 ? "one" : "other" }
            return i == 0 || i == 1 ? "one" : "other"
        case "ru", "uk", "be", "sr", "hr", "bs":
            guard whole else { return "other" }
            if i % 10 == 1 && i % 100 != 11 { return "one" }
            if (2...4).contains(i % 10) && !(12...14).contains(i % 100) { return "few" }
            return ["sr", "hr", "bs"].contains(lang) ? "other" : "many"
        case "pl":
            guard whole else { return "other" }
            if i == 1 { return "one" }
            if (2...4).contains(i % 10) && !(12...14).contains(i % 100) { return "few" }
            return "many"
        case "cs", "sk":
            guard whole else { return "many" }
            return i == 1 ? "one" : (2...4).contains(i) ? "few" : "other"
        case "ro":
            guard whole else { return "few" }
            return i == 1 ? "one" : (i == 0 || (1...19).contains(i % 100)) ? "few" : "other"
        case "lt":
            guard whole else { return "many" }
            if i % 10 == 1 && !(11...19).contains(i % 100) { return "one" }
            if (2...9).contains(i % 10) && !(11...19).contains(i % 100) { return "few" }
            return "other"
        case "lv":
            if i % 10 == 0 || (11...19).contains(i % 100) { return "zero" }
            return i % 10 == 1 && i % 100 != 11 ? "one" : "other"
        case "sl":
            switch i % 100 { case 1: return "one"; case 2: return "two"; case 3, 4: return "few"; default: return "other" }
        case "ar":
            switch i { case 0: return "zero"; case 1: return "one"; case 2: return "two"; default: break }
            if (3...10).contains(i % 100) { return "few" }
            return (11...99).contains(i % 100) ? "many" : "other"
        case "he":
            return i == 1 && whole ? "one" : i == 2 && whole ? "two" : "other"
        case "ga":
            return i == 1 ? "one" : i == 2 ? "two" : (3...6).contains(i) ? "few" : (7...10).contains(i) ? "many" : "other"
        default:
            return n == 1 ? "one" : "other"
        }
    }
}

/// The text in the app's language for a Hungarian source text. `{0}`, `{1}`… are filled from `args`; a plural
/// entry picks its form by the first argument.
public func L(_ key: String, _ args: Any...) -> String {
    Language.shared.text(key, args)
}
