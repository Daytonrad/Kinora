import Foundation

// Subtitle search and download through the server (OpenSubtitles). The server keeps the
// downloaded file and lists it as an external track of the item from then on.

/// One subtitle the service offers.
public struct SubtitleResult: Codable, Sendable, Identifiable, Hashable {
    public var fileId: Int
    /// Kinora's 3-letter code (hun, eng…).
    public var lang: String?
    public var release: String?
    public var fileName: String?
    public var downloads: Int?
    public var rating: Double?
    public var hearingImpaired: Bool?
    public var forced: Bool?
    /// Made for exactly this file: the timing fits.
    public var hashMatch: Bool?
    public var trusted: Bool?
    /// AI or machine translated.
    public var machineMade: Bool?
    public var uploader: String?
    /// Already downloaded for this item.
    public var have: Bool?
    public var id: Int { fileId }

    public var name: String {
        [release, fileName].compactMap { $0 }.first { !$0.isEmpty } ?? L("Ismeretlen")
    }
}

/// GET /api/items/{id}/subtitles/search
public struct SubtitleSearchResult: Codable, Sendable {
    /// False when the server has no OpenSubtitles key.
    public var enabled: Bool
    public var langs: [String]?
    public var results: [SubtitleResult]
}

/// POST /api/items/{id}/subtitles/download
public struct SubtitleDownloadResult: Decodable, Sendable {
    public var stream: MediaStream
    /// Downloads left today on the server's account; negative or nil when unknown.
    public var remaining: Int?
}

private struct SubtitleDownloadRequest: Encodable, Sendable {
    var fileId: Int
    var lang: String
    var release: String
    var hearingImpaired: Bool
    var forced: Bool
}

extension KinoraClient {
    /// `langs` nil: the profile's subtitle languages (Hungarian and English when it has none).
    public func searchSubtitles(_ id: Int, langs: [String]? = nil) async throws -> SubtitleSearchResult {
        var path = "/api/items/\(id)/subtitles/search"
        if let langs, !langs.isEmpty {
            let q = langs.joined(separator: ",")
            path += "?langs=" + (q.addingPercentEncoding(withAllowedCharacters: .urlQueryValueAllowed) ?? q)
        }
        return try await get(path)
    }

    public func downloadSubtitle(_ id: Int, _ r: SubtitleResult) async throws -> SubtitleDownloadResult {
        try await post("/api/items/\(id)/subtitles/download",
                       SubtitleDownloadRequest(fileId: r.fileId, lang: r.lang ?? "", release: r.release ?? r.fileName ?? "",
                                               hearingImpaired: r.hearingImpaired ?? false, forced: r.forced ?? false))
    }
}
