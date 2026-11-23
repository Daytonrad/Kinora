import ActivityKit
import Foundation

// Shared by the app (which starts and updates the activities) and the widget extension
// (which draws them on the lock screen and in the Dynamic Island).

/// Downloads on their way to the device.
struct DownloadsActivityAttributes: ActivityAttributes {
    struct ContentState: Codable, Hashable {
        /// Downloads still running (preparing on the server or transferring).
        var active: Int
        /// Finished since the activity started.
        var done: Int
        /// Failed since the activity started.
        var failed: Int
        /// The download in front (the one transferring, else the first preparing).
        var title: String
        /// 0…1 across the running downloads.
        var fraction: Double
        /// Every running download is still being prepared by the server.
        var preparing: Bool
        /// When `fraction` was measured.
        var measuredAt: Date = .now
        /// Estimated finish of every running transfer, from the measured speed; nil while unknown.
        /// The app is suspended in the background and sends nothing, so the widget runs the bar
        /// and the remaining time on its own towards this date.
        var eta: Date? = nil

        /// The span the progress bar runs over on its own, placed so it shows `fraction` at `measuredAt`.
        var timeline: ClosedRange<Date>? {
            guard let eta, active > 0, !preparing, eta > measuredAt, fraction < 1 else { return nil }
            let f = min(max(fraction, 0), 0.99)
            let start = measuredAt.addingTimeInterval(-eta.timeIntervalSince(measuredAt) * f / (1 - f))
            return start...eta
        }
    }
}

/// A watch party room this device is in.
struct PartyActivityAttributes: ActivityAttributes {
    var code: String

    struct ContentState: Codable, Hashable {
        var title: String
        var members: Int
        var playing: Bool
        /// Seconds into the title at `at`.
        var position: Double
        var duration: Double
        var at: Date

        /// The span the progress bar runs over while playing (it moves on its own).
        var timeline: ClosedRange<Date>? {
            guard playing, duration > 0 else { return nil }
            let start = at.addingTimeInterval(-position)
            return start...start.addingTimeInterval(max(duration, position + 1))
        }

        var fraction: Double { duration > 0 ? min(1, max(0, position / duration)) : 0 }
    }
}

/// One requested title the server is fetching, started and driven by the server through push.
/// The state is sealed with the pairing's key so the relay and Apple only see ciphertext; the
/// widget opens it with the key in the shared keychain (`PushCrypto.liveState`).
struct RequestActivityAttributes: ActivityAttributes {
    /// The relay's id of the server, picks the key.
    var s: String
    /// The server's reference of the request.
    var ref: String

    struct ContentState: Codable, Hashable {
        /// base64url(nonce || ciphertext || tag) of a `PushLiveState`.
        var e: String
    }
}
