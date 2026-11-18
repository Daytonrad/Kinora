import HomeKit
import Observation

/// Runs a Home app scene when playback starts, pauses or stops ("Movie night": lights off).
/// The scenes are picked in Settings and kept on this device only.
@MainActor @Observable
final class HomeScenes: NSObject, HMHomeManagerDelegate {
    static let shared = HomeScenes()

    enum Moment: String, CaseIterable, Identifiable {
        case start, pause, stop
        var id: String { rawValue }
        var label: String {
            switch self {
            case .start: "Lejátszás indul"
            case .pause: "Szünet"
            case .stop: "Lejátszás vége"
            }
        }
    }

    struct Scene: Hashable, Identifiable {
        let id: UUID
        let name: String
        let home: String
    }

    /// Every scene of every home, once the Home data has loaded.
    private(set) var scenes: [Scene] = []
    private(set) var loaded = false
    /// The user turned down Home access in iOS Settings.
    private(set) var denied = false

    private var manager: HMHomeManager?
    private var pauseTask: Task<Void, Never>?
    private var stopTask: Task<Void, Never>?
    /// Set once the start scene ran, so a pause or stop scene only follows a real start.
    private var dimmed = false

    private static func key(_ m: Moment) -> String { "kinora.homeScene.\(m.rawValue)" }

    func scene(for moment: Moment) -> UUID? {
        UserDefaults.standard.string(forKey: Self.key(moment)).flatMap(UUID.init)
    }

    func setScene(_ id: UUID?, for moment: Moment) {
        UserDefaults.standard.set(id?.uuidString, forKey: Self.key(moment))
        if id != nil { connect() }
    }

    private var anyScene: Bool { Moment.allCases.contains { scene(for: $0) != nil } }

    /// Reads the homes; asks for Home access the first time. Only called once the user
    /// opens the setting or has already picked a scene, so nobody is asked out of the blue.
    func connect() {
        guard manager == nil else { return }
        let m = HMHomeManager()
        m.delegate = self
        manager = m
    }

    /// At launch: get the home data ready if scenes are set, so the first play is quick.
    func warmUp() {
        if anyScene { connect() }
    }

    // MARK: - playback events

    func playing() {
        pauseTask?.cancel()
        stopTask?.cancel()
        // Resuming after the pause scene: run the start scene again; mid-play ticks do nothing.
        guard !dimmed || pausedSceneRan else { return }
        pausedSceneRan = false
        dimmed = true
        run(.start)
    }

    private var pausedSceneRan = false

    /// A few seconds' grace: seeking and buffering also stop the picture briefly.
    func paused() {
        guard dimmed, !pausedSceneRan else { return }
        pauseTask?.cancel()
        pauseTask = Task {
            try? await Task.sleep(for: .seconds(4))
            guard !Task.isCancelled else { return }
            pausedSceneRan = true
            run(.pause)
            pauseTask = nil
        }
    }

    /// The player closed. Waits a moment: the next episode opens a new player straight away.
    func stopped() {
        pauseTask?.cancel()
        pauseTask = nil
        guard dimmed else { return }
        stopTask = Task {
            try? await Task.sleep(for: .seconds(3))
            guard !Task.isCancelled else { return }
            dimmed = false
            pausedSceneRan = false
            run(.stop)
        }
    }

    private func run(_ moment: Moment) {
        guard let id = scene(for: moment) else { return }
        connect()
        guard let manager, let (home, set) = find(id, in: manager) else { return }
        home.executeActionSet(set) { _ in }
    }

    private func find(_ id: UUID, in manager: HMHomeManager) -> (HMHome, HMActionSet)? {
        for home in manager.homes {
            if let set = home.actionSets.first(where: { $0.uniqueIdentifier == id }) { return (home, set) }
        }
        return nil
    }

    // MARK: - HMHomeManagerDelegate

    nonisolated func homeManagerDidUpdateHomes(_ manager: HMHomeManager) {
        Task { @MainActor in self.refresh(manager) }
    }

    nonisolated func homeManager(_ manager: HMHomeManager, didUpdate status: HMHomeManagerAuthorizationStatus) {
        Task { @MainActor in self.refresh(manager) }
    }

    private func refresh(_ manager: HMHomeManager) {
        denied = manager.authorizationStatus.contains(.determined) && !manager.authorizationStatus.contains(.authorized)
        scenes = manager.homes.flatMap { home in
            home.actionSets.map { Scene(id: $0.uniqueIdentifier, name: $0.name, home: home.name) }
        }
        .sorted { ($0.home, $0.name) < ($1.home, $1.name) }
        loaded = true
    }
}
