import Foundation

/// A stream of the file as the server probed it (video, audio or subtitle).
public struct MediaStream: Decodable, Sendable, Hashable {
    public var type: String
    public var typeIndex: Int
    public var codec: String?
    public var lang: String?
    public var title: String?
    public var channels: Int?
    public var isDefault: Bool?
    public var forced: Bool?
    public var textBased: Bool?

    enum CodingKeys: String, CodingKey {
        case type, typeIndex, codec, lang, title, channels, forced, textBased
        case isDefault = "default"
    }

    public init(type: String, typeIndex: Int, codec: String? = nil, lang: String? = nil, title: String? = nil,
                channels: Int? = nil, isDefault: Bool? = nil) {
        self.type = type; self.typeIndex = typeIndex; self.codec = codec; self.lang = lang
        self.title = title; self.channels = channels; self.isDefault = isDefault
    }
}

/// A chapter mark of the file.
public struct Chapter: Codable, Sendable, Hashable {
    public var start: Double
    public var title: String?

    public init(start: Double, title: String? = nil) { self.start = start; self.title = title }

    // The server sends the Go field names as they are ("Start", "Title").
    enum CodingKeys: String, CodingKey { case start, title, Start, Title }

    public init(from decoder: Decoder) throws {
        let c = try decoder.container(keyedBy: CodingKeys.self)
        start = try c.decodeIfPresent(Double.self, forKey: .Start) ?? c.decode(Double.self, forKey: .start)
        title = try c.decodeIfPresent(String.self, forKey: .Title) ?? c.decodeIfPresent(String.self, forKey: .title)
    }

    public func encode(to encoder: Encoder) throws {
        var c = encoder.container(keyedBy: CodingKeys.self)
        try c.encode(start, forKey: .start)
        try c.encodeIfPresent(title, forKey: .title)
    }
}

public extension PlaybackInfo {
    var audioTracks: [MediaStream] { (streams ?? []).filter { $0.type == "audio" } }
    /// Subtitles the player can draw (image-based ones such as PGS can't be converted to text).
    var textSubtitles: [SubtitleTrack] { (subtitles ?? []).filter { $0.textBased != false } }
}

public enum TrackLabel {
    /// "Magyar", or the raw code when the system doesn't know it.
    public static func language(_ code: String?, locale: Locale = .current) -> String? {
        guard let code, !code.isEmpty, code != "und" else { return nil }
        let name = locale.localizedString(forLanguageCode: code) ?? code
        return name.prefix(1).uppercased() + name.dropFirst()
    }

    public static func audio(_ s: MediaStream, locale: Locale = .current) -> String {
        var parts = [language(s.lang, locale: locale) ?? "\(s.typeIndex + 1). hangsáv"]
        if let t = s.title, !t.isEmpty, t != parts[0] { parts.append(t) }
        var tech: [String] = []
        if let c = s.codec, !c.isEmpty { tech.append(c.uppercased()) }
        switch s.channels {
        case 1: tech.append("mono")
        case 2: tech.append("sztereó")
        case let n? where n > 2: tech.append(n == 6 ? "5.1" : n == 8 ? "7.1" : "\(n) csatorna")
        default: break
        }
        if !tech.isEmpty { parts.append(tech.joined(separator: " ")) }
        return parts.joined(separator: " · ")
    }

    public static func subtitle(_ s: SubtitleTrack, locale: Locale = .current) -> String {
        var parts = [language(s.lang, locale: locale) ?? "\(s.idx + 1). felirat"]
        if let t = s.title, !t.isEmpty, t != parts[0] { parts.append(t) }
        if s.forced == true { parts.append("kényszerített") }
        return parts.joined(separator: " · ")
    }
}

/// Picks the subtitle a profile wants by default.
/// mode: "off" none, "always" the first in a preferred language (else any), "auto" forced tracks only.
public func defaultSubtitle(_ tracks: [SubtitleTrack], mode: String?, langs: String?) -> SubtitleTrack? {
    let prefs = (langs ?? "").split(separator: ",").map { $0.trimmingCharacters(in: .whitespaces).lowercased() }.filter { !$0.isEmpty }
    func rank(_ t: SubtitleTrack) -> Int {
        guard let l = t.lang?.lowercased(), let i = prefs.firstIndex(where: { languageMatches($0, l) }) else { return prefs.count }
        return i
    }
    switch mode ?? "auto" {
    case "off":
        return nil
    case "always":
        let preferred = tracks.filter { rank($0) < prefs.count && $0.forced != true }
        return preferred.min { rank($0) < rank($1) } ?? tracks.first { $0.forced != true } ?? tracks.first
    default:
        let forced = tracks.filter { $0.forced == true }
        return forced.min { rank($0) < rank($1) }
    }
}

/// "hu" matches "hun" and "hu-HU"; ISO 639-1 and 639-2 codes.
public func languageMatches(_ a: String, _ b: String) -> Bool {
    func norm(_ s: String) -> String {
        let base = String(s.lowercased().split(separator: "-").first ?? "")
        return Locale.LanguageCode(base).identifier(.alpha2) ?? base
    }
    return norm(a) == norm(b)
}

/// One timed line of a WebVTT file.
public struct Cue: Sendable, Hashable {
    public var start: Double
    public var end: Double
    public var text: String
}

public enum WebVTT {
    /// Parses cues, dropping styling tags; sorted by start.
    public static func parse(_ source: String) -> [Cue] {
        var cues: [Cue] = []
        let blocks = source.replacingOccurrences(of: "\r\n", with: "\n").replacingOccurrences(of: "\r", with: "\n")
            .components(separatedBy: "\n\n")
        for block in blocks {
            let lines = block.split(separator: "\n", omittingEmptySubsequences: false).map(String.init)
            guard let ti = lines.firstIndex(where: { $0.contains("-->") }) else { continue }
            let sides = lines[ti].components(separatedBy: "-->")
            guard sides.count == 2,
                  let s = time(sides[0]),
                  let e = time(sides[1].trimmingCharacters(in: .whitespaces).split(separator: " ").first.map(String.init) ?? "")
            else { continue }
            let text = lines[(ti + 1)...].joined(separator: "\n")
                .replacingOccurrences(of: "<[^>]+>", with: "", options: .regularExpression)
                .replacingOccurrences(of: "&amp;", with: "&").replacingOccurrences(of: "&lt;", with: "<")
                .replacingOccurrences(of: "&gt;", with: ">").replacingOccurrences(of: "&nbsp;", with: " ")
                .trimmingCharacters(in: .whitespacesAndNewlines)
            if !text.isEmpty { cues.append(Cue(start: s, end: e, text: text)) }
        }
        return cues.sorted { $0.start < $1.start }
    }

    /// "01:02:03.456" or "02:03.456".
    static func time(_ raw: String) -> Double? {
        let parts = raw.trimmingCharacters(in: .whitespaces).replacingOccurrences(of: ",", with: ".").split(separator: ":")
        guard (2...3).contains(parts.count) else { return nil }
        var total = 0.0
        for p in parts {
            guard let v = Double(p) else { return nil }
            total = total * 60 + v
        }
        return total
    }

    /// The text showing at `t`, overlapping cues joined.
    public static func text(at t: Double, in cues: [Cue]) -> String? {
        let active = cues.prefix { $0.start <= t }.filter { $0.end > t }
        return active.isEmpty ? nil : active.map(\.text).joined(separator: "\n")
    }
}
