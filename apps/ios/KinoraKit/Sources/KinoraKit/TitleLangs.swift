import Foundation

// The audio and subtitle language a profile starts one film or series with (GET/PUT /api/items/{id}/langs).
// An episode or season stands for its series, so a choice made while watching carries to the next episodes.
// The playback answer applies it: `audio_idx`, and `sub_idx` for the subtitle.

public struct TitleLangs: Codable, Sendable, Equatable {
    public var titleId: Int
    /// "" = the profile's audio languages.
    public var audioLang: String
    /// "" = the profile's subtitle setting, "off", or a language.
    public var subLang: String
    /// The languages the title's files have.
    public var audio: [String]?
    public var subs: [String]?

    public static let off = "off"
}

private struct TitleLangsBody: Encodable, Sendable {
    var audioLang: String?
    var subLang: String?
}

extension KinoraClient {
    public func titleLangs(_ id: Int) async throws -> TitleLangs { try await get("/api/items/\(id)/langs") }

    /// nil leaves that one as it is; "" gives it back to the profile's settings.
    @discardableResult
    public func setTitleLangs(_ id: Int, audio: String? = nil, subtitle: String? = nil) async throws -> TitleLangs {
        try await put("/api/items/\(id)/langs", TitleLangsBody(audioLang: audio, subLang: subtitle))
    }
}
