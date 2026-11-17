import AVKit
import SwiftUI
import KinoraKit

/// Full-screen player for the Siri Remote.
///
/// - click or play/pause: pause and resume; left/right: ±10 s (repeats add up);
///   up/down or a swipe: show the controls; Menu/Back: hide the controls, then leave.
/// - Intro skip and the next episode take the focus when they appear, so a click acts on them.
///
/// The same commands come from an iPhone (Control Center remote, lock screen) through
/// PlayerModel's Now Playing info and remote command handlers.
struct TVPlayerView: View {
    @Bindable var player: PlayerModel

    enum Focus: Hashable { case timeline, skip, next, dismissNext, audio, subtitles, speed, chapters, retry, transcode, close }

    @FocusState private var focus: Focus?
    @State private var chrome = true
    @State private var hideTask: Task<Void, Never>?
    @State private var seekFlash: Double?
    @State private var flashTask: Task<Void, Never>?

    var body: some View {
        ZStack {
            Color.black.ignoresSafeArea()
            TVPlayerSurface(layer: player.layer).ignoresSafeArea()

            VStack {
                Spacer()
                LinearGradient(colors: [.clear, .black.opacity(0.85)], startPoint: .top, endPoint: .bottom)
                    .frame(height: 560)
            }
            .ignoresSafeArea()
            .opacity(chrome && player.error == nil ? 1 : 0)
            .animation(.easeOut(duration: 0.25), value: chrome)
            .allowsHitTesting(false)

            if let caption = player.caption {
                TVCaption(text: caption, raised: chrome || prompt, style: AppSettings.subtitleStyle)
            }

            if player.loading || player.buffering {
                ProgressView().scaleEffect(1.6)
            }

            if let message = player.error {
                errorPanel(message)
            } else {
                controls
            }

            if let s = seekFlash {
                Text(s > 0 ? "+\(Int(s)) mp" : "−\(Int(-s)) mp")
                    .font(.system(size: 44, weight: .bold).monospacedDigit())
                    .padding(.horizontal, 36).padding(.vertical, 18)
                    .background(.black.opacity(0.55), in: .capsule)
                    .transition(.opacity)
            }
        }
        .onPlayPauseCommand { player.togglePlay(); show() }
        .onExitCommand(perform: exit)
        .onAppear {
            player.presented = true
            focus = .timeline
            scheduleHide()
        }
        .onDisappear { player.presented = false }
        .onChange(of: player.isPlaying) { _, playing in
            if playing { scheduleHide() } else { show(autoHide: false) }
        }
        .onChange(of: player.currentSegment?.start) { _, start in
            if start != nil { focus = .skip } else if focus == .skip { focus = .timeline }
        }
        .onChange(of: player.showNext) { _, on in
            if on { show(autoHide: false); focus = .next } else if focus == .next || focus == .dismissNext { focus = .timeline }
        }
        .onChange(of: player.error) { _, e in if e != nil { focus = .retry } }
        .persistentSystemOverlays(.hidden)
    }

    private var prompt: Bool { player.showNext || player.currentSegment != nil }

    // MARK: - controls

    private var controls: some View {
        VStack(alignment: .leading, spacing: 30) {
            Spacer()
            HStack(alignment: .bottom, spacing: 30) {
                if chrome { titleBlock.transition(.opacity) }
                Spacer()
                promptButtons
            }
            TVTimeline(player: player, focused: focus == .timeline)
                .opacity(chrome ? 1 : 0)
                .focusable()
                .focused($focus, equals: .timeline)
                .onMoveCommand { dir in
                    switch dir {
                    case .left: seek(-10)
                    case .right: seek(10)
                    default: show()
                    }
                }
                .onTapGesture { player.togglePlay(); show() }
            if chrome {
                menuRow.transition(.opacity)
            }
        }
        .padding(.horizontal, 90)
        .padding(.bottom, 60)
        .animation(.easeOut(duration: 0.25), value: chrome)
    }

    private var titleBlock: some View {
        VStack(alignment: .leading, spacing: 8) {
            if player.item.kind == .episode {
                Text([player.item.seriesTitle, player.item.episodeLabel].compactMap { $0 }.joined(separator: " · "))
                    .font(.callout.weight(.semibold)).foregroundStyle(Theme.amber)
            }
            Text(player.item.title).font(Theme.display(54)).lineLimit(1)
            if let c = player.currentChapter?.title, !c.isEmpty {
                Text(c).font(.callout).foregroundStyle(Theme.plaster)
            }
        }
    }

    @ViewBuilder private var promptButtons: some View {
        if player.showNext, let next = player.next {
            VStack(alignment: .trailing, spacing: 16) {
                Text("Következő rész" + (player.nextCountdown.map { " · \($0) mp" } ?? ""))
                    .font(.callout.weight(.semibold)).foregroundStyle(Theme.amber)
                HStack(spacing: 20) {
                    Button("Bezárás") { player.dismissNext(); focus = .timeline }
                        .buttonStyle(TVPillStyle())
                        .focused($focus, equals: .dismissNext)
                    Button { player.playNext() } label: {
                        Label([next.episodeLabel, next.title].compactMap { $0 }.joined(separator: " · "), systemImage: "forward.end.fill")
                            .lineLimit(1)
                    }
                    .buttonStyle(TVPillStyle(prominent: true))
                    .focused($focus, equals: .next)
                }
            }
            .focusSection()
        } else if let seg = player.currentSegment {
            Button { player.skipSegment(); focus = .timeline } label: {
                Label(Self.skipLabel(seg.type), systemImage: "forward.fill")
            }
            .buttonStyle(TVPillStyle(prominent: true))
            .focused($focus, equals: .skip)
        }
    }

    private var menuRow: some View {
        let audio = player.info?.audioTracks ?? []
        let subs = player.info?.textSubtitles ?? []
        return HStack(spacing: 24) {
            if !subs.isEmpty {
                Menu {
                    Button { player.selectSubtitle(nil) } label: { check("Ki", player.subtitle == nil) }
                    ForEach(subs) { s in
                        Button { player.selectSubtitle(s) } label: { check(TrackLabel.subtitle(s), player.subtitle == s) }
                    }
                    if player.subtitle != nil {
                        Section("Időzítés: \(Self.delayLabel(player.subtitleDelay))") {
                            Button("Korábban (−0,5 mp)", systemImage: "minus") { player.shiftSubtitles(by: -0.5) }
                            Button("Később (+0,5 mp)", systemImage: "plus") { player.shiftSubtitles(by: 0.5) }
                            if player.subtitleDelay != 0 {
                                Button("Visszaállítás", systemImage: "arrow.counterclockwise") { player.shiftSubtitles(by: 0) }
                            }
                        }
                    }
                } label: {
                    Label(player.subtitle.map { TrackLabel.subtitle($0) } ?? "Felirat", systemImage: "captions.bubble")
                }
                .focused($focus, equals: .subtitles)
            }
            if audio.count > 1, !player.isOffline {
                Menu {
                    ForEach(audio, id: \.typeIndex) { a in
                        Button { player.selectAudio(a.typeIndex) } label: { check(TrackLabel.audio(a), a.typeIndex == player.audioIdx) }
                    }
                } label: {
                    Label(audio.first { $0.typeIndex == player.audioIdx }.map { TrackLabel.audio($0) } ?? "Hang",
                          systemImage: "speaker.wave.2")
                }
                .focused($focus, equals: .audio)
            }
            if !player.chapters.isEmpty {
                Menu {
                    ForEach(Array(player.chapters.enumerated()), id: \.offset) { i, c in
                        Button { player.seek(to: c.start) } label: {
                            check("\(c.title ?? "\(i + 1). fejezet") · \(Self.clock(c.start))", c == player.currentChapter)
                        }
                    }
                } label: { Label("Fejezetek", systemImage: "list.bullet") }
                .focused($focus, equals: .chapters)
            }
            Menu {
                ForEach(PlaybackSpeeds.all, id: \.self) { r in
                    Button { player.setSpeed(r) } label: { check(PlaybackSpeeds.label(r), r == player.speed) }
                }
                if !player.isOffline {
                    Section("Minőség") {
                        Button { player.setQuality(nil) } label: { check("Automatikus", player.qualityCap == nil) }
                        ForEach(SettingChoices.bitrates, id: \.0) { b in
                            Button { player.setQuality(b.0) } label: { check(b.1, player.qualityCap == b.0) }
                        }
                    }
                }
            } label: { Label(PlaybackSpeeds.label(player.speed), systemImage: "gauge.with.dots.needle.67percent") }
            .focused($focus, equals: .speed)
        }
        .font(.callout)
        .focusSection()
        .onChange(of: focus) { _, f in
            // Menus and buttons keep the controls up while they have the focus.
            // An open menu takes the focus away from all of them (nil): no hiding then either.
            if f == .timeline || f == .skip { scheduleHide() } else { hideTask?.cancel() }
        }
    }

    @ViewBuilder private func check(_ title: String, _ on: Bool) -> some View {
        if on { Label(title, systemImage: "checkmark") } else { Text(title) }
    }

    private func errorPanel(_ message: String) -> some View {
        VStack(spacing: 30) {
            Image(systemName: "exclamationmark.triangle").font(.system(size: 70)).foregroundStyle(Theme.amber)
            Text(message).font(.title3).multilineTextAlignment(.center).frame(maxWidth: 1100)
            if let reasons = player.info?.reasons, !reasons.isEmpty {
                Text(reasons.joined(separator: " · ")).font(.callout).foregroundStyle(Theme.muted)
            }
            HStack(spacing: 24) {
                Button { player.retry() } label: { Label("Újra", systemImage: "arrow.clockwise") }
                    .buttonStyle(TVPillStyle(prominent: true))
                    .focused($focus, equals: .retry)
                if !player.triedTranscode && !player.isOffline {
                    Button("Lejátszás átkódolással") { player.retry(transcode: true) }
                        .buttonStyle(TVPillStyle())
                        .focused($focus, equals: .transcode)
                }
                Button("Bezárás") { player.close() }
                    .buttonStyle(TVPillStyle())
                    .focused($focus, equals: .close)
            }
        }
        .padding(60)
        .background(Theme.panel.opacity(0.92), in: .rect(cornerRadius: 30))
    }

    // MARK: - behaviour

    private func exit() {
        if player.error == nil, chrome, player.isPlaying {
            hide()
        } else {
            player.close()
        }
    }

    private func seek(_ delta: Double) {
        player.seek(by: delta)
        let total = (seekFlash.map { ($0 > 0) == (delta > 0) ? $0 : 0 } ?? 0) + delta
        withAnimation(.easeOut(duration: 0.15)) { seekFlash = total }
        flashTask?.cancel()
        flashTask = Task {
            try? await Task.sleep(for: .seconds(0.9))
            guard !Task.isCancelled else { return }
            withAnimation(.easeOut(duration: 0.25)) { seekFlash = nil }
        }
        show()
    }

    private func show(autoHide: Bool = true) {
        chrome = true
        if autoHide { scheduleHide() } else { hideTask?.cancel() }
    }

    private func hide() {
        hideTask?.cancel()
        chrome = false
        if focus != .skip && focus != .next && focus != .dismissNext { focus = .timeline }
    }

    private func scheduleHide() {
        hideTask?.cancel()
        hideTask = Task {
            try? await Task.sleep(for: .seconds(5))
            guard !Task.isCancelled, player.isPlaying, focus == .timeline || focus == .skip else { return }
            hide()
        }
    }

    // MARK: - labels

    static func clock(_ seconds: Double) -> String {
        let s = max(0, Int(seconds.isFinite ? seconds : 0))
        let h = s / 3600, m = (s % 3600) / 60, sec = s % 60
        return h > 0 ? String(format: "%d:%02d:%02d", h, m, sec) : String(format: "%d:%02d", m, sec)
    }

    static func delayLabel(_ d: Double) -> String {
        d == 0 ? "eredeti" : String(format: "%+.1f mp", d).replacingOccurrences(of: ".", with: ",")
    }

    static func skipLabel(_ kind: Segment.Kind) -> String {
        switch kind {
        case .recap: "Összefoglaló átugrása"
        case .credits: "Stáblista átugrása"
        default: "Intro átugrása"
        }
    }
}

/// Progress bar with elapsed and remaining time; thicker while it has the focus.
private struct TVTimeline: View {
    var player: PlayerModel
    var focused: Bool

    var body: some View {
        let d = player.duration
        let p = d > 0 ? min(1, max(0, player.position / d)) : 0
        VStack(spacing: 14) {
            GeometryReader { g in
                ZStack(alignment: .leading) {
                    Capsule().fill(.white.opacity(0.25))
                    Capsule().fill(Theme.amber).frame(width: g.size.width * p)
                    ForEach(Array(player.chapters.dropFirst().enumerated()), id: \.offset) { _, c in
                        if d > 0 {
                            Rectangle().fill(.black.opacity(0.6)).frame(width: 3)
                                .offset(x: g.size.width * c.start / d)
                        }
                    }
                    Circle().fill(Theme.plasterBright)
                        .frame(width: focused ? 26 : 0, height: focused ? 26 : 0)
                        .offset(x: g.size.width * p - (focused ? 13 : 0))
                }
            }
            .frame(height: focused ? 14 : 8)
            HStack {
                Image(systemName: player.isPlaying ? "play.fill" : "pause.fill").foregroundStyle(Theme.plaster)
                Text(TVPlayerView.clock(player.position))
                Spacer()
                Text("−" + TVPlayerView.clock(d - player.position))
            }
            .font(.callout.monospacedDigit())
            .foregroundStyle(Theme.plaster)
        }
        .animation(.easeOut(duration: 0.15), value: focused)
    }
}

private struct TVCaption: View {
    var text: String
    var raised: Bool
    var style: SubtitleStyle

    var body: some View {
        VStack {
            Spacer()
            Text(text)
                .font(.system(size: style.points * 2, weight: .semibold))
                .foregroundStyle(style.color == .yellow ? Color(red: 1, green: 0.88, blue: 0.3) : .white)
                .multilineTextAlignment(.center)
                .shadow(color: .black.opacity(style.background == .none ? 0 : 0.9), radius: 4, x: 0, y: 2)
                .padding(.horizontal, style.background == .box ? 22 : 0)
                .padding(.vertical, style.background == .box ? 8 : 0)
                .background(style.background == .box ? Color.black.opacity(0.6) : .clear, in: .rect(cornerRadius: 10))
                .padding(.horizontal, 160)
                .padding(.bottom, raised ? 300 : 70)
                .animation(.easeOut(duration: 0.25), value: raised)
        }
        .allowsHitTesting(false)
    }
}

/// Hosts the player's AVPlayerLayer.
private struct TVPlayerSurface: UIViewRepresentable {
    var layer: AVPlayerLayer

    func makeUIView(context: Context) -> SurfaceView {
        let v = SurfaceView()
        v.backgroundColor = .black
        v.host(layer)
        return v
    }

    func updateUIView(_ view: SurfaceView, context: Context) { view.host(layer) }

    final class SurfaceView: UIView {
        private weak var hosted: AVPlayerLayer?

        func host(_ l: AVPlayerLayer) {
            guard hosted !== l else { return }
            hosted?.removeFromSuperlayer()
            layer.addSublayer(l)
            hosted = l
            setNeedsLayout()
        }

        override func layoutSubviews() {
            super.layoutSubviews()
            CATransaction.begin()
            CATransaction.setDisableActions(true)
            hosted?.frame = bounds
            CATransaction.commit()
        }
    }
}
