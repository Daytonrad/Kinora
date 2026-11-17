import AVKit
import SwiftUI
import KinoraKit

/// Full-screen player: video, controls that hide while playing, skip and next-episode prompts.
struct PlayerView: View {
    let player: PlayerModel
    @State private var chrome = true
    @State private var scrub: Double?
    @State private var hideTask: Task<Void, Never>?

    var body: some View {
        ZStack {
            Color.black.ignoresSafeArea()
            PlayerSurface(layer: player.layer)
                .ignoresSafeArea()
                .contentShape(.rect)
                .onTapGesture { toggleChrome() }
                .onTapGesture(count: 2) { withAnimation { player.fill.toggle() } }
                .accessibilityHidden(true)

            if player.error == nil, player.loading || player.buffering {
                ProgressView().controlSize(.large).tint(.white).allowsHitTesting(false)
            }

            if let caption = player.caption, player.error == nil {
                CaptionView(text: caption, raised: chrome)
                    .animation(.easeInOut(duration: 0.2), value: chrome)
            }

            if let error = player.error {
                errorPanel(error)
            } else {
                if chrome { controls.transition(.opacity) }
                prompts
            }
        }
        .foregroundStyle(.white)
        .statusBarHidden(!chrome)
        .persistentSystemOverlays(chrome ? .automatic : .hidden)
        .preferredColorScheme(.dark)
        .onAppear { player.presented = true; scheduleHide() }
        .onDisappear {
            player.presented = false
            // Swiped away or closed; in picture in picture it keeps playing.
            if !player.pipActive { player.close() }
        }
        .onChange(of: player.isPlaying) { _, playing in
            if playing { scheduleHide() } else { show() }
        }
    }

    // MARK: - controls

    private var controls: some View {
        VStack(spacing: 0) {
            HStack(spacing: 6) {
                RoundIcon("xmark", label: "Bezárás") { player.close() }
                Text(player.title)
                    .font(.headline)
                    .lineLimit(1)
                    .frame(maxWidth: .infinity, alignment: .leading)
                    .padding(.leading, 6)
                if player.hasTrackChoice { tracksMenu }
                AirPlayButton().frame(width: 44, height: 44)
                if player.pipSupported {
                    RoundIcon("pip.enter", label: "Kép a képben") { player.startPiP() }
                }
            }
            .padding(.horizontal, 12)
            .padding(.top, 8)

            Spacer()

            HStack(spacing: 48) {
                RoundIcon("gobackward.10", size: 30, label: "Vissza 10 másodpercet") { player.seek(by: -10); scheduleHide() }
                RoundIcon(player.isPlaying ? "pause.fill" : "play.fill", size: 44,
                          label: player.isPlaying ? "Szünet" : "Lejátszás") { player.togglePlay() }
                RoundIcon("goforward.10", size: 30, label: "Előre 10 másodpercet") { player.seek(by: 10); scheduleHide() }
            }

            Spacer()

            VStack(spacing: 4) {
                Slider(value: Binding(get: { scrub ?? player.position },
                                      set: { scrub = $0 }),
                       in: 0...max(player.duration, 1)) { editing in
                    if editing {
                        hideTask?.cancel()
                    } else if let t = scrub {
                        player.seek(to: t)
                        scrub = nil
                        scheduleHide()
                    }
                }
                .tint(Theme.amber)
                .accessibilityLabel("Lejátszási pozíció")
                .accessibilityValue(Self.clock(scrub ?? player.position))
                HStack {
                    Text(Self.clock(scrub ?? player.position))
                    Spacer()
                    Text("-" + Self.clock(max(0, player.duration - (scrub ?? player.position))))
                }
                .font(.caption.monospacedDigit())
                .foregroundStyle(.white.opacity(0.8))
            }
            .padding(.horizontal, 20)
            .padding(.bottom, 12)
        }
        .background {
            LinearGradient(stops: [.init(color: .black.opacity(0.7), location: 0),
                                   .init(color: .clear, location: 0.25),
                                   .init(color: .clear, location: 0.7),
                                   .init(color: .black.opacity(0.8), location: 1)],
                           startPoint: .top, endPoint: .bottom)
                .ignoresSafeArea()
                .allowsHitTesting(false)
        }
    }

    /// Skip intro and next episode sit above the controls, also when those are hidden.
    @ViewBuilder private var prompts: some View {
        VStack {
            Spacer()
            HStack {
                Spacer()
                if player.showNext, let next = player.next {
                    NextCard(next: next, countdown: player.nextCountdown,
                             play: { player.playNext() }, dismiss: { player.dismissNext() })
                } else if let seg = player.currentSegment {
                    Button { player.skipSegment() } label: {
                        Label(Self.skipLabel(seg.type), systemImage: "forward.end.fill")
                            .font(.subheadline.weight(.semibold))
                    }
                    .buttonStyle(PrimaryButtonStyle())
                    .fixedSize()
                }
            }
            .padding(.horizontal, 20)
            // A fixed spot above the scrubber: a button that moves when the controls hide
            // is easy to miss.
            .padding(.bottom, 84)
        }
        .animation(.default, value: player.showNext)
        .animation(.default, value: player.currentSegment)
    }

    private func errorPanel(_ message: String) -> some View {
        VStack(spacing: 14) {
            Image(systemName: "exclamationmark.triangle").font(.largeTitle).foregroundStyle(Theme.amber)
            Text("Nem sikerült lejátszani").font(Theme.display(24, weight: .bold))
            Text(message).multilineTextAlignment(.center).foregroundStyle(Theme.plaster)
            if let reasons = player.info?.reasons, !reasons.isEmpty {
                Text(reasons.joined(separator: " · ")).font(.caption).foregroundStyle(Theme.muted)
                    .multilineTextAlignment(.center)
            }
            HStack {
                Button("Újra") { player.retry() }.buttonStyle(PrimaryButtonStyle())
                if !player.triedTranscode {
                    Button("Átkódolással") { player.retry(transcode: true) }.buttonStyle(SecondaryButtonStyle())
                }
                Button("Bezárás") { player.close() }.buttonStyle(SecondaryButtonStyle())
            }
            .fixedSize()
        }
        .padding(32)
        .frame(maxWidth: 520)
    }

    // MARK: - tracks

    private var tracksMenu: some View {
        let audio = player.info?.audioTracks ?? []
        let subs = player.info?.textSubtitles ?? []
        return Menu {
            if audio.count > 1 {
                Section("Hang") {
                    ForEach(audio, id: \.typeIndex) { a in
                        Button { player.selectAudio(a.typeIndex) } label: {
                            if a.typeIndex == player.audioIdx {
                                Label(TrackLabel.audio(a), systemImage: "checkmark")
                            } else {
                                Text(TrackLabel.audio(a))
                            }
                        }
                    }
                }
            }
            if !subs.isEmpty {
                Section("Felirat") {
                    Button { player.selectSubtitle(nil) } label: {
                        if player.subtitle == nil { Label("Ki", systemImage: "checkmark") } else { Text("Ki") }
                    }
                    ForEach(subs) { t in
                        Button { player.selectSubtitle(t) } label: {
                            if t == player.subtitle {
                                Label(TrackLabel.subtitle(t), systemImage: "checkmark")
                            } else {
                                Text(TrackLabel.subtitle(t))
                            }
                        }
                    }
                }
            }
        } label: {
            Image(systemName: player.subtitle == nil ? "captions.bubble" : "captions.bubble.fill")
                .font(.system(size: 20, weight: .semibold))
                .frame(width: 44, height: 44)
                .contentShape(.circle)
        }
        .menuActionDismissBehavior(.automatic)
        .accessibilityLabel("Hang és felirat")
        .simultaneousGesture(TapGesture().onEnded { show() })
    }

    // MARK: - chrome timing

    private func toggleChrome() {
        withAnimation(.easeInOut(duration: 0.2)) { chrome.toggle() }
        if chrome { scheduleHide() }
    }

    private func show() {
        hideTask?.cancel()
        withAnimation(.easeInOut(duration: 0.2)) { chrome = true }
    }

    private func scheduleHide() {
        hideTask?.cancel()
        hideTask = Task {
            try? await Task.sleep(for: .seconds(3.5))
            guard !Task.isCancelled, player.isPlaying, scrub == nil else { return }
            withAnimation(.easeInOut(duration: 0.3)) { chrome = false }
        }
    }

    // MARK: - text

    static func clock(_ seconds: Double) -> String {
        let s = Int(max(0, seconds.isFinite ? seconds : 0))
        let h = s / 3600, m = (s % 3600) / 60, sec = s % 60
        return h > 0 ? String(format: "%d:%02d:%02d", h, m, sec) : String(format: "%d:%02d", m, sec)
    }

    static func skipLabel(_ kind: Segment.Kind) -> String {
        switch kind {
        case .recap: "Összefoglaló átugrása"
        case .credits: "Stáblista átugrása"
        default: "Intro átugrása"
        }
    }
}

private struct NextCard: View {
    var next: Item
    var countdown: Int?
    var play: () -> Void
    var dismiss: () -> Void

    var body: some View {
        VStack(alignment: .leading, spacing: 10) {
            Text("Következő rész").font(.caption.weight(.semibold)).foregroundStyle(Theme.amber)
            Text(next.title).font(.headline).lineLimit(2)
            if let label = next.episodeLabel {
                Text(label).font(.caption).foregroundStyle(Theme.plaster)
            }
            HStack(spacing: 10) {
                Button(action: play) {
                    Label(countdown.map { "Lejátszás (\($0))" } ?? "Lejátszás", systemImage: "play.fill")
                        .font(.subheadline.weight(.semibold))
                        .contentTransition(.numericText())
                }
                .buttonStyle(PrimaryButtonStyle())
                Button("Mégse", action: dismiss).buttonStyle(SecondaryButtonStyle())
            }
            .fixedSize()
        }
        .padding(16)
        .frame(maxWidth: 320, alignment: .leading)
        .background(.ultraThinMaterial, in: .rect(cornerRadius: 14))
    }
}

private struct RoundIcon: View {
    var name: String
    var size: CGFloat
    var label: String
    var action: () -> Void

    init(_ name: String, size: CGFloat = 20, label: String, action: @escaping () -> Void) {
        self.name = name
        self.size = size
        self.label = label
        self.action = action
    }

    var body: some View {
        Button(action: action) {
            Image(systemName: name)
                .font(.system(size: size, weight: .semibold))
                .frame(width: max(44, size * 1.6), height: max(44, size * 1.6))
                .contentShape(.circle)
        }
        .buttonStyle(.plain)
        .accessibilityLabel(label)
    }
}

private struct CaptionView: View {
    var text: String
    var raised: Bool

    var body: some View {
        VStack {
            Spacer()
            Text(text)
                .font(.system(size: 20, weight: .semibold))
                .multilineTextAlignment(.center)
                .padding(.horizontal, 10)
                .padding(.vertical, 4)
                .background(.black.opacity(0.55), in: .rect(cornerRadius: 6))
                .shadow(color: .black, radius: 2)
                .padding(.horizontal, 24)
                .padding(.bottom, raised ? 110 : 28)
        }
        .allowsHitTesting(false)
        .accessibilityLabel(text)
    }
}

/// Hosts the player's own layer, so the video survives the view (picture in picture).
private struct PlayerSurface: UIViewRepresentable {
    let layer: AVPlayerLayer

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
            guard hosted !== l || l.superlayer !== layer else { return }
            hosted = l
            layer.addSublayer(l)
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

private struct AirPlayButton: UIViewRepresentable {
    func makeUIView(context: Context) -> AVRoutePickerView {
        let v = AVRoutePickerView()
        v.tintColor = .white
        v.activeTintColor = UIColor(Theme.amber)
        v.prioritizesVideoDevices = true
        return v
    }

    func updateUIView(_ uiView: AVRoutePickerView, context: Context) {}
}
