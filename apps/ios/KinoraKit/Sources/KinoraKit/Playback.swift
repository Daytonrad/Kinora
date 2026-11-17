import Foundation

/// What this device plays without help. The server direct-plays a file only when all of
/// it fits; otherwise it repackages (remux) or transcodes it to HLS.
public struct PlaybackCaps: Codable, Sendable, Equatable {
    public var video: [String]
    public var audio: [String]
    public var containers: [String]
    public var hdr: Bool

    public init(video: [String], audio: [String], containers: [String], hdr: Bool) {
        self.video = video
        self.audio = audio
        self.containers = containers
        self.hdr = hdr
    }

    /// AVPlayer on iPhone and iPad.
    public static let apple = PlaybackCaps(
        video: ["h264", "hevc"],
        audio: ["aac", "mp3", "ac3", "eac3", "flac", "alac"],
        containers: ["mp4", "mov"],
        hdr: false
    )
}

public struct PlaybackRequest: Encodable, Sendable {
    public var caps: PlaybackCaps
    public var start: Double
    public var audioIdx: Int?
    public var maxBitrate: Int?
    /// "transcode" after a failed direct play or remux.
    public var force: String?
    /// The session being replaced (seek outside the transcoded range); the server stops it.
    public var prevSession: String?

    public init(caps: PlaybackCaps = .apple, start: Double, audioIdx: Int? = nil, maxBitrate: Int? = nil,
                force: String? = nil, prevSession: String? = nil) {
        self.caps = caps
        self.start = start
        self.audioIdx = audioIdx
        self.maxBitrate = maxBitrate
        self.force = force
        self.prevSession = prevSession
    }
}

/// A part of the video that can be skipped.
public struct Segment: Codable, Sendable, Hashable {
    public enum Kind: String, Codable, Sendable {
        case intro, recap, credits, preview, unknown

        public init(from decoder: Decoder) throws {
            self = Kind(rawValue: try decoder.singleValueContainer().decode(String.self)) ?? .unknown
        }
    }

    public var type: Kind
    public var start: Double
    public var end: Double

    public init(type: Kind, start: Double, end: Double) {
        self.type = type
        self.start = start
        self.end = end
    }
}

public struct SubtitleTrack: Codable, Sendable, Hashable, Identifiable {
    public var idx: Int
    public var lang: String?
    public var title: String?
    public var forced: Bool?
    public var textBased: Bool?
    public var url: String
    public var id: Int { idx }
}

/// The server's answer to "play this": where the stream is and how it was decided.
public struct PlaybackInfo: Decodable, Sendable {
    public enum Mode: String, Decodable, Sendable {
        case direct, remux, transcode, unknown

        public init(from decoder: Decoder) throws {
            self = Mode(rawValue: try decoder.singleValueContainer().decode(String.self)) ?? .unknown
        }
    }

    public struct PlayItem: Decodable, Sendable, Hashable {
        public var id: Int
        public var title: String
        public var kind: Item.Kind
        public var seriesTitle: String?
        public var seriesId: Int?
        public var seasonNum: Int?
        public var episodeNum: Int?
    }

    /// Relative to the server, with the stream token already in the query.
    public var url: String
    public var mode: Mode
    public var reasons: [String]?
    /// HLS session id (remux / transcode); stopped when playback ends.
    public var session: String?
    /// Where the HLS stream starts in the file: player time 0 is this position.
    public var startOffset: Double?
    public var duration: Double?
    public var creditsStart: Double?
    public var segments: [Segment]?
    public var subtitles: [SubtitleTrack]?
    public var audioIdx: Int?
    public var streams: [MediaStream]?
    public var streamToken: String?
    public var item: PlayItem
    public var encoderLabel: String?

    /// Where the closing credits start: marked credits, then the server's guess.
    public var creditsAt: Double? {
        segments?.first { $0.type == .credits }?.start ?? creditsStart
    }
}

public struct ProgressResult: Decodable, Sendable {
    public var ok: Bool?
    /// The server counted the item as watched (near the end or `ended`).
    public var played: Bool?
}

extension KinoraClient {
    public func playback(_ id: Int, _ request: PlaybackRequest) async throws -> PlaybackInfo {
        try await post("/api/items/\(id)/playback", request)
    }

    /// Position in seconds from the start of the file.
    @discardableResult
    public func reportProgress(_ id: Int, position: Double, ended: Bool = false) async throws -> ProgressResult {
        try await post("/api/items/\(id)/progress", ProgressBody(position: position, ended: ended))
    }

    /// Stops a remux / transcode session on the server.
    public func stopStream(_ session: String) async throws {
        try await delete("/api/hls/\(session)")
    }

    /// The episode after this one, if any.
    public func nextEpisode(after id: Int) async throws -> Item? {
        try await get("/api/items/\(id)/next", as: NextBody.self).next
    }
}

struct ProgressBody: Encodable, Sendable {
    var position: Double
    var ended: Bool
}

struct NextBody: Decodable, Sendable {
    var next: Item?
}
