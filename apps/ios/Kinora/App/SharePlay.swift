import Combine
import GroupActivities
import SwiftUI
import KinoraKit

/// SharePlay rides on the watch party: the activity carries the room code, everyone in the
/// FaceTime call joins that room, and the server keeps the players together as it always
/// does (chat, reactions and the host's controls included). So everyone needs an account
/// on the same Kinora server; SharePlay only saves typing the code.
struct WatchTogether: GroupActivity {
    var code: String
    var serverId: String
    var title: String

    var metadata: GroupActivityMetadata {
        var m = GroupActivityMetadata()
        m.type = .watchTogether
        m.title = title
        m.subtitle = L("Kinora watch party · {0}", code)
        return m
    }
}

@MainActor @Observable
final class SharePlay {
    static let shared = SharePlay()

    weak var model: AppModel?
    /// The room code of the SharePlay session this device is in.
    private(set) var sharedCode: String?

    private var session: GroupSession<WatchTogether>?
    @ObservationIgnored private let observer = GroupStateObserver()
    /// A FaceTime call is going on, so a new party can be offered to it right away.
    var callActive: Bool { observer.isEligibleForGroupSession }
    private var watchers: [Task<Void, Never>] = []

    private init() {}

    /// Waits for sessions the whole time the app runs: started here, or by someone in the call.
    func listen() {
        Task {
            for await s in WatchTogether.sessions() { configure(s) }
        }
    }

    private func configure(_ s: GroupSession<WatchTogether>) {
        leave()
        session = s
        watchers = [
            Task { [weak self] in
                for await activity in s.$activity.values { self?.arrived(activity) }
            },
            Task { [weak self] in
                for await state in s.$state.values {
                    if case .invalidated = state, self?.session === s { self?.ended() }
                }
            },
        ]
        s.join()
    }

    private func arrived(_ a: WatchTogether) {
        sharedCode = a.code
        model?.sharePlayRoom(a)
    }

    private func ended() {
        watchers.forEach { $0.cancel() }
        watchers = []
        session = nil
        sharedCode = nil
    }

    /// Offers the room to the FaceTime call; the system asks whether to SharePlay it or not.
    /// Returns false when there is no call or the user chose to watch alone.
    @discardableResult
    func share(code: String, serverId: String, title: String) async -> Bool {
        guard sharedCode != code else { return true }
        let activity = WatchTogether(code: code, serverId: serverId, title: title)
        // A session already running (another title): the new room replaces it for everyone.
        if let session, session.state == .joined {
            session.activity = activity
            return true
        }
        switch await activity.prepareForActivation() {
        case .activationPreferred:
            do { return try await activity.activate() } catch { return false }
        default:
            return false
        }
    }

    /// This device leaves the shared session (the others stay in the call and the room).
    func leave() {
        session?.leave()
        ended()
    }

    /// The player left the room: a SharePlay session for that room ends here too.
    func left(room code: String) {
        if sharedCode == code { leave() }
    }
}

/// The SharePlay button in the party panel, shown during a FaceTime call.
struct SharePlayButton: View {
    var party: PartySession
    var title: String
    @Environment(AppModel.self) private var model
    @StateObject private var group = GroupStateObserver()

    var body: some View {
        if group.isEligibleForGroupSession {
            let shared = SharePlay.shared.sharedCode == party.code
            Button {
                Haptic.tap.play()
                guard let serverId = model.server?.id else { return }
                Task {
                    if await SharePlay.shared.share(code: party.code, serverId: serverId, title: title) {
                        Haptic.success.play()
                    }
                }
            } label: {
                Image(systemName: "shareplay").frame(width: 36, height: 36).contentShape(.rect)
            }
            .foregroundStyle(shared ? Theme.amber : .primary)
            .disabled(shared)
            .accessibilityLabel(shared ? L("SharePlay folyamatban") : L("Megosztás SharePlay-jel a FaceTime-hívásban"))
        }
    }
}

