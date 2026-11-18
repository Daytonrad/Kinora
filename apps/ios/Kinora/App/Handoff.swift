import Foundation
import KinoraKit

/// Handoff: what plays here shows up on the user's other iPhone or iPad (same Apple
/// account) in the app switcher and the Dock, and opens there at the same second.
/// A watch party hands off its room code instead, so the other device joins the room.
@MainActor
enum Handoff {
    static let activityType = (Bundle.main.bundleIdentifier ?? "io.github.scotylucas.kinora") + ".watching"

    /// Set by the app model when it switches servers; a handoff only plays on the same server.
    static var serverId: String?

    private static var activity: NSUserActivity?

    enum Key {
        static let server = "server", item = "item", position = "t", party = "party"
    }

    /// Called with every progress report (every 10 s while playing, on pause and seek).
    static func watching(_ itemId: Int, title: String, at position: Double, party: String?) {
        guard let serverId else { return stop() }
        let a = activity ?? NSUserActivity(activityType: activityType)
        a.title = title
        a.isEligibleForHandoff = true
        a.isEligibleForSearch = false
        a.isEligibleForPrediction = false
        var info: [String: Any] = [Key.server: serverId, Key.item: itemId, Key.position: position]
        if let party { info[Key.party] = party }
        a.userInfo = info
        a.needsSave = true
        if activity == nil {
            activity = a
            a.becomeCurrent()
        }
    }

    static func stop() {
        activity?.invalidate()
        activity = nil
    }

    /// What arrived from the other device.
    struct Incoming: Equatable {
        var serverId: String
        var itemId: Int
        var position: Double
        var party: String?
    }

    static func incoming(_ activity: NSUserActivity) -> Incoming? {
        guard activity.activityType == activityType, let info = activity.userInfo,
              let server = info[Key.server] as? String, let item = info[Key.item] as? Int else { return nil }
        return Incoming(serverId: server, itemId: item,
                        position: info[Key.position] as? Double ?? 0,
                        party: info[Key.party] as? String)
    }
}
