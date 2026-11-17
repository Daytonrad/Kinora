import AVKit
import MediaPlayer
import KinoraKit

/// One playback: asks the server how to play, drives AVPlayer, reports progress,
/// offers intro skip and the next episode, and keeps going in picture in picture.
///
/// Remuxed and transcoded streams are HLS playlists that grow while the server works and
/// start at `offset` in the file. A seek past what is ready starts a new session there.
@MainActor @Observable
final class PlayerModel: NSObject {
    let player = AVPlayer()
    let layer: AVPlayerLayer

    private(set) var item: Item
    private(set) var info: PlaybackInfo?
    private(set) var error: String?
    private(set) var loading = true
    private(set) var isPlaying = false
    private(set) var buffering = false
    /// Seconds from the start of the file.
    private(set) var position: Double = 0
    private(set) var next: Item?
    private(set) var nextCountdown: Int?
    private(set) var pipActive = false
    private(set) var triedTranscode = false
    var nextDismissed = false
    var fill = false { didSet { layer.videoGravity = fill ? .resizeAspectFill : .resizeAspect } }

    /// The full-screen player is on screen (set by the view).
    var presented = true
    var onClose: (() -> Void)?
    var onPiPStart: (() -> Void)?
    var onPiPRestore: (() -> Void)?

    private let client: KinoraClient
    private let skipMode: String
    private let autoplayNext: Bool
    private let firstStart: Double

    private var pip: AVPictureInPictureController?
    private var offset: Double = 0
    private var session: String?
    private var force: String?
    private var pendingSeek: Double?
    private var ended = false
    private var closed = false
    private var restoring = false
    private var skipped = Set<Double>()
    private var lastReport = Date.distantPast
    private var countdown: Task<Void, Never>?
    private var timeObserver: Any?
    private var endObserver: NSObjectProtocol?
    private var statusObservation: NSKeyValueObservation?
    private var appObservers: [NSObjectProtocol] = []

    init(item: Item, start: Double, client: KinoraClient, profile: Profile?) {
        self.item = item
        self.firstStart = start
        self.client = client
        skipMode = profile?.skipIntro ?? "manual"
        autoplayNext = profile?.autoplayNext ?? true
        layer = AVPlayerLayer(player: player)
        layer.videoGravity = .resizeAspect
        super.init()
        player.allowsExternalPlayback = true
        player.usesExternalPlaybackWhileExternalScreenIsActive = true
        if AVPictureInPictureController.isPictureInPictureSupported(),
           let pip = AVPictureInPictureController(playerLayer: layer) {
            pip.delegate = self
            pip.canStartPictureInPictureAutomaticallyFromInline = true
            self.pip = pip
        }
    }

    // MARK: - derived

    var duration: Double { info?.duration ?? item.runtimeSec ?? 0 }
    var pipSupported: Bool { pip != nil }

    var title: String {
        guard item.kind == .episode else { return item.title }
        return [item.seriesTitle, item.episodeLabel, item.title].compactMap { $0 }.joined(separator: " · ")
    }

    /// The intro / recap / credits part being watched, unless the profile turned skipping off.
    var currentSegment: Segment? {
        guard skipMode != "off", let segs = info?.segments else { return nil }
        if showNext { return nil } // the next-episode card takes over at the credits
        return segs.first { [.intro, .recap, .credits].contains($0.type) && position >= $0.start && position < $0.end - 1 }
    }

    /// From the credits (or near the end) the next episode is offered.
    var showNext: Bool {
        guard next != nil, !nextDismissed, duration > 0 else { return false }
        if ended { return true }
        let at = info?.creditsAt ?? max(duration - 30, duration * 0.85)
        return position >= at
    }

    // MARK: - lifecycle

    func begin() async {
        try? AVAudioSession.sharedInstance().setCategory(.playback, mode: .moviePlayback)
        try? AVAudioSession.sharedInstance().setActive(true)
        observe()
        setUpRemoteCommands()
        await start(at: firstStart)
    }

    /// Stops everything and tells the server where we got to.
    func close() {
        guard !closed else { return }
        closed = true
        report(ended: ended)
        stopSession()
        countdown?.cancel()
        player.pause()
        player.replaceCurrentItem(with: nil)
        if let timeObserver { player.removeTimeObserver(timeObserver) }
        if let endObserver { NotificationCenter.default.removeObserver(endObserver) }
        appObservers.forEach { NotificationCenter.default.removeObserver($0) }
        appObservers = []
        statusObservation = nil
        pip?.stopPictureInPicture()
        tearDownRemoteCommands()
        try? AVAudioSession.sharedInstance().setActive(false, options: .notifyOthersOnDeactivation)
        onClose?()
    }

    func retry(transcode: Bool = false) {
        if transcode { force = "transcode"; triedTranscode = true }
        Task { await start(at: position) }
    }

    private func start(at t: Double) async {
        loading = true
        error = nil
        let prev = session
        session = nil
        do {
            let info = try await client.playback(item.id, PlaybackRequest(start: t, force: force, prevSession: prev))
            guard !closed else { return }
            guard let url = await client.url(info.url) else { throw KinoraError.network }
            self.info = info
            session = info.session
            offset = info.mode == .direct ? 0 : (info.startOffset ?? 0)
            position = t
            pendingSeek = info.mode == .direct && t > 0 ? t : nil
            let playerItem = AVPlayerItem(url: url)
            watch(playerItem)
            player.replaceCurrentItem(with: playerItem)
            player.play()
            if item.kind == .episode, next == nil {
                next = try? await client.nextEpisode(after: item.id)
            }
            updateNowPlaying()
        } catch is CancellationError {
        } catch {
            self.error = error.localizedDescription
            loading = false
        }
    }

    private func stopSession() {
        guard let s = session else { return }
        session = nil
        let c = client
        Task { try? await c.stopStream(s) }
    }

    // MARK: - controls

    func togglePlay() {
        if player.timeControlStatus == .paused {
            if ended { ended = false; seek(to: 0) }
            player.play()
        } else {
            player.pause()
            report()
        }
        updateNowPlaying()
    }

    func seek(by delta: Double) { seek(to: position + delta) }

    /// Seconds from the start of the file.
    func seek(to target: Double) {
        guard let info else { return }
        let t = max(0, duration > 2 ? min(target, duration - 2) : target)
        ended = false
        position = t
        if info.mode == .direct {
            player.seek(to: CMTime(seconds: t, preferredTimescale: 600), toleranceBefore: .zero, toleranceAfter: .zero)
        } else {
            let rel = t - offset
            if rel >= 0, let range = player.currentItem?.seekableTimeRanges.last?.timeRangeValue,
               rel <= range.end.seconds {
                player.seek(to: CMTime(seconds: rel, preferredTimescale: 600), toleranceBefore: .zero, toleranceAfter: .zero)
            } else {
                // Not transcoded yet (or behind the start of this session): new session from there.
                report()
                Task { await start(at: t) }
            }
        }
        updateNowPlaying()
    }

    func skipSegment() {
        guard let s = currentSegment else { return }
        skipped.insert(s.start)
        seek(to: s.end)
    }

    func dismissNext() {
        nextDismissed = true
        countdown?.cancel()
        nextCountdown = nil
    }

    func playNext() {
        guard let n = next else { return }
        countdown?.cancel()
        nextCountdown = nil
        report(ended: true)
        item = n
        next = nil
        info = nil
        nextDismissed = false
        ended = false
        skipped = []
        position = 0
        let resume = n.progress != nil ? (n.userData?.position ?? 0) : 0
        Task { await start(at: resume) }
    }

    func startPiP() { pip?.startPictureInPicture() }

    // MARK: - observing

    private func observe() {
        timeObserver = player.addPeriodicTimeObserver(forInterval: CMTime(value: 1, timescale: 2), queue: .main) { [weak self] _ in
            MainActor.assumeIsolated { self?.tick() }
        }
        endObserver = NotificationCenter.default.addObserver(forName: AVPlayerItem.didPlayToEndTimeNotification, object: nil, queue: .main) { [weak self] note in
            let finished = (note.object as AnyObject?).map(ObjectIdentifier.init)
            MainActor.assumeIsolated {
                guard let self, let current = self.player.currentItem, finished == ObjectIdentifier(current) else { return }
                self.didEnd()
            }
        }
        // Settings → "Hang folytatása a háttérben": without a layer AVPlayer keeps the sound
        // going in the background. Picture in picture, when it started, takes precedence.
        let center = NotificationCenter.default
        appObservers = [
            center.addObserver(forName: UIApplication.didEnterBackgroundNotification, object: nil, queue: .main) { [weak self] _ in
                MainActor.assumeIsolated {
                    guard let self, !self.pipActive, AppSettings.backgroundAudio else { return }
                    let wasPlaying = self.player.rate > 0
                    self.layer.player = nil
                    if wasPlaying { self.player.play() }
                }
            },
            center.addObserver(forName: UIApplication.willEnterForegroundNotification, object: nil, queue: .main) { [weak self] _ in
                MainActor.assumeIsolated {
                    guard let self, self.layer.player == nil else { return }
                    self.layer.player = self.player
                }
            },
        ]
    }

    private func watch(_ playerItem: AVPlayerItem) {
        statusObservation = playerItem.observe(\.status, options: [.new]) { [weak self] observed, _ in
            let status = observed.status
            let message = observed.error?.localizedDescription
            Task { @MainActor in self?.statusChanged(status, message) }
        }
    }

    private func statusChanged(_ status: AVPlayerItem.Status, _ message: String?) {
        switch status {
        case .readyToPlay:
            loading = false
            if let t = pendingSeek {
                pendingSeek = nil
                player.seek(to: CMTime(seconds: t, preferredTimescale: 600), toleranceBefore: .zero, toleranceAfter: .zero)
            } else if info?.mode != .direct, player.currentTime().seconds > 1 {
                // A growing (EVENT) playlist may open at its live edge: start from the top.
                player.seek(to: .zero)
            }
        case .failed:
            loading = false
            error = message ?? "A videó nem játszható le."
        default:
            break
        }
    }

    private func tick() {
        guard !closed else { return }
        let now = player.currentTime().seconds
        if now.isFinite, player.currentItem != nil, !loading { position = offset + now }
        isPlaying = player.timeControlStatus == .playing
        buffering = player.timeControlStatus == .waitingToPlayAtSpecifiedRate
        if isPlaying, Date.now.timeIntervalSince(lastReport) > 10 { report() }

        if skipMode == "auto", let s = currentSegment, s.type != .credits, !skipped.contains(s.start) {
            skipped.insert(s.start)
            seek(to: s.end)
        }
        updateCountdown()
    }

    private func didEnd() {
        ended = true
        isPlaying = false
        report(ended: true)
        if next != nil, !nextDismissed {
            if autoplayNext { playNext() }
        } else {
            close()
        }
    }

    private func updateCountdown() {
        if showNext, autoplayNext, countdown == nil {
            nextCountdown = 10
            countdown = Task { [weak self] in
                while let self, let n = self.nextCountdown, n > 0 {
                    try? await Task.sleep(for: .seconds(1))
                    if Task.isCancelled { return }
                    self.nextCountdown = n - 1
                }
                guard !Task.isCancelled else { return }
                self?.playNext()
            }
        } else if !showNext, let c = countdown {
            c.cancel()
            countdown = nil
            nextCountdown = nil
        }
    }

    /// Position report: every 10 s while playing, on pause, seek and close.
    private func report(ended: Bool = false) {
        lastReport = .now
        let p = position, id = item.id, c = client
        guard p >= 1 || ended else { return }
        Task { try? await c.reportProgress(id, position: p, ended: ended) }
    }

    // MARK: - lock screen and Control Center

    private func updateNowPlaying() {
        var info: [String: Any] = [
            MPMediaItemPropertyTitle: item.title,
            MPMediaItemPropertyPlaybackDuration: duration,
            MPNowPlayingInfoPropertyElapsedPlaybackTime: position,
            MPNowPlayingInfoPropertyPlaybackRate: player.rate,
            MPNowPlayingInfoPropertyMediaType: MPNowPlayingInfoMediaType.video.rawValue,
        ]
        if item.kind == .episode {
            info[MPMediaItemPropertyArtist] = [item.seriesTitle, item.episodeLabel].compactMap { $0 }.joined(separator: " · ")
        }
        MPNowPlayingInfoCenter.default().nowPlayingInfo = info
    }

    private func setUpRemoteCommands() {
        let center = MPRemoteCommandCenter.shared()
        center.playCommand.addTarget { [weak self] _ in
            MainActor.assumeIsolated { self?.player.play() }
            return .success
        }
        center.pauseCommand.addTarget { [weak self] _ in
            MainActor.assumeIsolated { self?.player.pause() }
            return .success
        }
        center.togglePlayPauseCommand.addTarget { [weak self] _ in
            MainActor.assumeIsolated { self?.togglePlay() }
            return .success
        }
        center.skipForwardCommand.preferredIntervals = [10]
        center.skipForwardCommand.addTarget { [weak self] _ in
            MainActor.assumeIsolated { self?.seek(by: 10) }
            return .success
        }
        center.skipBackwardCommand.preferredIntervals = [10]
        center.skipBackwardCommand.addTarget { [weak self] _ in
            MainActor.assumeIsolated { self?.seek(by: -10) }
            return .success
        }
        center.changePlaybackPositionCommand.addTarget { [weak self] event in
            guard let e = event as? MPChangePlaybackPositionCommandEvent else { return .commandFailed }
            let t = e.positionTime
            MainActor.assumeIsolated { self?.seek(to: t) }
            return .success
        }
    }

    private func tearDownRemoteCommands() {
        let center = MPRemoteCommandCenter.shared()
        for cmd in [center.playCommand, center.pauseCommand, center.togglePlayPauseCommand,
                    center.skipForwardCommand, center.skipBackwardCommand, center.changePlaybackPositionCommand] {
            cmd.removeTarget(nil)
        }
        MPNowPlayingInfoCenter.default().nowPlayingInfo = nil
    }
}

// MARK: - picture in picture

extension PlayerModel: AVPictureInPictureControllerDelegate {
    nonisolated func pictureInPictureControllerWillStartPictureInPicture(_ controller: AVPictureInPictureController) {
        MainActor.assumeIsolated {
            pipActive = true
            onPiPStart?()
        }
    }

    nonisolated func pictureInPictureController(_ controller: AVPictureInPictureController,
                                                restoreUserInterfaceForPictureInPictureStopWithCompletionHandler completionHandler: @escaping (Bool) -> Void) {
        MainActor.assumeIsolated {
            restoring = true
            onPiPRestore?()
        }
        completionHandler(true)
    }

    nonisolated func pictureInPictureControllerDidStopPictureInPicture(_ controller: AVPictureInPictureController) {
        MainActor.assumeIsolated {
            pipActive = false
            // Closed from the PiP window (not brought back): playback ends.
            if !presented && !restoring { close() }
            restoring = false
        }
    }
}
