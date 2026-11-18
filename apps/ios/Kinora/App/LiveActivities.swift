import ActivityKit
import Foundation

/// Starts, updates and ends the app's Live Activities. The user can turn them off in the
/// system Settings (Kinora → Élő tevékenységek); every call then does nothing.
@MainActor
enum LiveActivities {
    private static var enabled: Bool { ActivityAuthorizationInfo().areActivitiesEnabled }

    // MARK: - downloads

    private static var downloads: Activity<DownloadsActivityAttributes>?
    private static var lastDownloads: DownloadsActivityAttributes.ContentState?
    private static var lastDownloadsUpdate = Date.distantPast
    private static var done = 0
    private static var failed = 0

    /// One download finished or failed; counted into the current activity.
    static func downloadEnded(failed didFail: Bool) {
        guard downloads != nil else { return }
        if didFail { failed += 1 } else { done += 1 }
    }

    /// The running downloads changed: `active` is how many run, `fraction` their overall progress.
    static func downloadsChanged(active: Int, title: String, fraction: Double, preparing: Bool) {
        guard active > 0 else { return endDownloads() }
        let state = DownloadsActivityAttributes.ContentState(
            active: active, done: done, failed: failed, title: title,
            fraction: fraction, preparing: preparing)
        guard let current = downloads else {
            guard enabled else { return }
            done = 0
            failed = 0
            var fresh = state
            fresh.done = 0
            fresh.failed = 0
            downloads = try? Activity.request(attributes: DownloadsActivityAttributes(),
                                              content: .init(state: fresh, staleDate: nil))
            lastDownloads = fresh
            lastDownloadsUpdate = .now
            return
        }
        // Progress moves many times a second; the lock screen needs a few steps, not every byte.
        let structural = lastDownloads.map {
            $0.active != state.active || $0.done != state.done || $0.failed != state.failed || $0.title != state.title
        } ?? true
        guard structural || Date.now.timeIntervalSince(lastDownloadsUpdate) > 2,
              state != lastDownloads else { return }
        lastDownloads = state
        lastDownloadsUpdate = .now
        // Suspended in the background the app sends nothing until the next download ends.
        update(current.id, DownloadsActivityAttributes.self, .init(state: state, staleDate: .now.addingTimeInterval(15 * 60)))
    }

    private static func endDownloads() {
        guard let current = downloads else { return }
        downloads = nil
        let final = DownloadsActivityAttributes.ContentState(
            active: 0, done: done, failed: failed, title: lastDownloads?.title ?? "",
            fraction: 1, preparing: false)
        lastDownloads = nil
        end(current.id, DownloadsActivityAttributes.self, .init(state: final, staleDate: nil), .after(.now.addingTimeInterval(10 * 60)))
    }

    // MARK: - watch party

    private static var party: Activity<PartyActivityAttributes>?
    private static var lastParty: PartyActivityAttributes.ContentState?

    static func partyChanged(code: String, _ state: PartyActivityAttributes.ContentState) {
        if let current = party, current.attributes.code != code { endParty() }
        guard let current = party else {
            guard enabled else { return }
            party = try? Activity.request(attributes: PartyActivityAttributes(code: code),
                                          content: .init(state: state, staleDate: nil))
            lastParty = state
            return
        }
        // While playing the bar runs on its own; only a real change needs an update.
        if let last = lastParty, last.title == state.title, last.members == state.members,
           last.playing == state.playing,
           abs((last.position + (state.playing ? state.at.timeIntervalSince(last.at) : 0)) - state.position) < 5 {
            return
        }
        lastParty = state
        update(current.id, PartyActivityAttributes.self, .init(state: state, staleDate: nil))
    }

    static func endParty() {
        guard let current = party else { return }
        party = nil
        lastParty = nil
        end(current.id, PartyActivityAttributes.self, nil, .immediate)
    }

    /// Activities a previous run left behind (the app was killed) are ended on launch.
    static func cleanUp() {
        for a in Activity<PartyActivityAttributes>.activities where a.id != party?.id {
            end(a.id, PartyActivityAttributes.self, nil, .immediate)
        }
        for a in Activity<DownloadsActivityAttributes>.activities where a.id != downloads?.id {
            end(a.id, DownloadsActivityAttributes.self, nil, .immediate)
        }
    }

    // Activities are not Sendable: the async calls look them up by id off the main actor.

    private nonisolated static func update<A: ActivityAttributes>(_ id: String, _: A.Type, _ content: sending ActivityContent<A.ContentState>) {
        Task.detached {
            await Activity<A>.activities.first { $0.id == id }?.update(content)
        }
    }

    private nonisolated static func end<A: ActivityAttributes>(_ id: String, _: A.Type, _ content: sending ActivityContent<A.ContentState>?,
                                                   _ policy: ActivityUIDismissalPolicy) {
        Task.detached {
            await Activity<A>.activities.first { $0.id == id }?.end(content, dismissalPolicy: policy)
        }
    }
}
