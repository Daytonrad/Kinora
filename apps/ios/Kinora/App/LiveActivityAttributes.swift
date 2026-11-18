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
