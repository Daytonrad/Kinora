import Foundation

/// How subtitles are drawn over the video.
public struct SubtitleStyle: Codable, Sendable, Equatable {
    public enum Size: String, Codable, Sendable, CaseIterable { case small, medium, large, huge }
    public enum Background: String, Codable, Sendable, CaseIterable { case none, shadow, box }
    public enum Color: String, Codable, Sendable, CaseIterable { case white, yellow }

    public var size: Size
    public var background: Background
    public var color: Color

    public init(size: Size = .medium, background: Background = .box, color: Color = .white) {
        self.size = size; self.background = background; self.color = color
    }

    public var points: Double {
        switch size { case .small: 16; case .medium: 20; case .large: 26; case .huge: 32 }
    }

    public static func label(_ s: Size) -> String {
        switch s { case .small: "Kicsi"; case .medium: "Közepes"; case .large: "Nagy"; case .huge: "Nagyon nagy" }
    }
    public static func label(_ b: Background) -> String {
        switch b { case .none: "Nincs"; case .shadow: "Árnyék"; case .box: "Sötét doboz" }
    }
    public static func label(_ c: Color) -> String {
        switch c { case .white: "Fehér"; case .yellow: "Sárga" }
    }
}

/// The audio and subtitle language last picked for a series, so the next episode starts the same way.
public struct LanguageChoice: Codable, Sendable, Equatable {
    /// Audio language code; nil keeps the server's choice.
    public var audio: String?
    /// Subtitle language code; "" means subtitles off, nil keeps the profile default.
    public var subtitle: String?
    public var subtitleForced: Bool?

    public init(audio: String? = nil, subtitle: String? = nil, subtitleForced: Bool? = nil) {
        self.audio = audio; self.subtitle = subtitle; self.subtitleForced = subtitleForced
    }

    /// The audio track in the remembered language, if the file has one.
    public func audioTrack(in tracks: [MediaStream]) -> MediaStream? {
        guard let audio, !audio.isEmpty else { return nil }
        return tracks.first { $0.lang.map { languageMatches($0, audio) } ?? false }
    }

    /// `.some(nil)` is "off", nil means no remembered choice applies to this file.
    public func subtitleTrack(in tracks: [SubtitleTrack]) -> SubtitleTrack?? {
        guard let subtitle else { return nil }
        if subtitle.isEmpty { return .some(nil) }
        let same = tracks.filter { $0.lang.map { languageMatches($0, subtitle) } ?? false }
        guard !same.isEmpty else { return nil }
        let pick = same.first { ($0.forced ?? false) == (subtitleForced ?? false) } ?? same.first
        return .some(pick)
    }
}

public enum PlaybackSpeeds {
    public static let all: [Float] = [0.5, 0.75, 1, 1.25, 1.5, 1.75, 2]

    public static func label(_ r: Float) -> String {
        let s = r == r.rounded() ? String(format: "%.0f", r) : String(format: "%g", r)
        return s.replacingOccurrences(of: ".", with: ",") + "×"
    }
}

public extension Array where Element == Chapter {
    /// The chapter playing at `t`.
    func current(at t: Double) -> Chapter? { last { $0.start <= t + 0.5 } }
}
