import Foundation

/// A server-side offline download (transcoded to MP4).
public struct DownloadJob: Codable, Sendable, Equatable, Identifiable {
    public var key: String
    public var itemId: Int
    public var title: String?
    public var fileName: String?
    public var quality: String?
    public var status: String
    public var progress: Double?
    public var error: String?
    public var size: Int64?
    public var id: String { key }

    public var isDone: Bool { status == "done" }
    public var isFailed: Bool { status == "error" }
}

public struct DownloadQuality: Codable, Sendable, Equatable, Hashable {
    public var name: String
    public var label: String
}

public struct DownloadList: Codable, Sendable {
    public var jobs: [DownloadJob]
    public var qualities: [DownloadQuality]?
}

struct DownloadBody: Codable, Sendable {
    var quality: String
    var audioIdx: Int?
}

struct ItemStreams: Decodable, Sendable {
    struct Inner: Decodable, Sendable { var streams: [MediaStream]? }
    var item: Inner
}

extension KinoraClient {
    /// Starts (or finds) the server-side transcode of an item for offline viewing.
    public func createDownload(_ id: Int, quality: String, audioIdx: Int? = nil) async throws -> DownloadJob {
        try await post("/api/items/\(id)/download", DownloadBody(quality: quality, audioIdx: audioIdx))
    }

    public func downloads() async throws -> DownloadList {
        try await get("/api/downloads")
    }

    /// The finished file, fetched with this client's session token.
    public func downloadRequest(_ key: String) -> URLRequest? {
        let path = "/api/downloads/" + (key.addingPercentEncoding(withAllowedCharacters: .alphanumerics) ?? key) + "/file"
        guard let url = url(path) else { return nil }
        var r = URLRequest(url: url)
        if let token { r.setValue("Bearer " + token, forHTTPHeaderField: "Authorization") }
        return r
    }

    /// The subtitle streams of an item (for saving them with a download).
    public func subtitleStreams(_ id: Int) async throws -> [SubtitleTrack] {
        let s: ItemStreams = try await get("/api/items/\(id)")
        return (s.item.streams ?? []).filter { $0.type == "subtitle" && $0.textBased != false }.map {
            SubtitleTrack(idx: $0.typeIndex, lang: $0.lang, title: $0.title, forced: $0.forced, textBased: true,
                          url: "/api/items/\(id)/subtitles/\($0.typeIndex)")
        }
    }
}
