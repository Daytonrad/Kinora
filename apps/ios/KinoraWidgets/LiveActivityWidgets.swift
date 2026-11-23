import ActivityKit
import SwiftUI
import WidgetKit
import KinoraKit

// MARK: - downloads

struct DownloadsLiveActivity: Widget {
    var body: some WidgetConfiguration {
        ActivityConfiguration(for: DownloadsActivityAttributes.self) { context in
            DownloadsLockView(state: context.state, stale: context.isStale)
                .padding(16)
                .activityBackgroundTint(Palette.black.opacity(0.9))
                .activitySystemActionForegroundColor(Palette.amber)
        } dynamicIsland: { context in
            let s = context.state
            let stale = context.isStale
            return DynamicIsland {
                DynamicIslandExpandedRegion(.leading) {
                    Image(systemName: s.active > 0 ? "arrow.down.circle.fill" : "checkmark.circle.fill")
                        .font(.title2)
                        .foregroundStyle(Palette.amber)
                }
                DynamicIslandExpandedRegion(.trailing) {
                    DownloadsStatus(state: s, stale: stale)
                        .font(.headline.monospacedDigit())
                        .foregroundStyle(Palette.bright)
                }
                DynamicIslandExpandedRegion(.bottom) {
                    VStack(alignment: .leading, spacing: 6) {
                        Text(DownloadsLockView.headline(s, stale: stale)).font(.subheadline.weight(.semibold))
                        if s.active > 0 {
                            Text(s.title).font(.caption).foregroundStyle(Palette.muted).lineLimit(1)
                            DownloadsProgress(state: s, stale: stale)
                        }
                    }
                }
            } compactLeading: {
                Image(systemName: "arrow.down.circle.fill").foregroundStyle(Palette.amber)
            } compactTrailing: {
                if s.active > 0 {
                    // A ring that fills on its own: a percentage would freeze while the app is suspended.
                    DownloadsProgress(state: s, stale: stale, circular: true)
                } else {
                    Image(systemName: "checkmark").foregroundStyle(Palette.amber)
                }
            } minimal: {
                if s.active > 0 {
                    DownloadsProgress(state: s, stale: stale, circular: true)
                } else {
                    Image(systemName: "checkmark").foregroundStyle(Palette.amber)
                }
            }
            .widgetURL(URL(string: "kinora://downloads"))
        }
    }
}

struct DownloadsLockView: View {
    var state: DownloadsActivityAttributes.ContentState
    var stale = false

    /// A percentage only means something once bytes are moving; while the server prepares it would sit at 0%.
    static func showsPercent(_ s: DownloadsActivityAttributes.ContentState) -> Bool {
        s.active > 0 && !s.preparing
    }

    static func percent(_ s: DownloadsActivityAttributes.ContentState) -> String {
        min(max(s.fraction, 0), 1).formatted(.percent.precision(.fractionLength(0)))
    }

    /// Past the estimated finish with no word from the (suspended) app: almost there, not "done".
    static func finishing(_ s: DownloadsActivityAttributes.ContentState, stale: Bool) -> Bool {
        stale && s.active > 0 && s.eta != nil
    }

    static func headline(_ s: DownloadsActivityAttributes.ContentState, stale: Bool = false) -> String {
        if finishing(s, stale: stale) { return L("Befejezés…") }
        if s.active == 0 {
            if s.done == 0 { return s.failed > 0 ? L("A letöltés nem sikerült") : L("Letöltések") }
            return s.done == 1 ? L("Letöltve, nézhető net nélkül") : L("{0} cím letöltve", s.done)
        }
        if s.preparing { return s.active == 1 ? L("A szerver készíti a fájlt") : L("{0} letöltés készül", s.active) }
        return s.active == 1 ? L("Letöltés") : L("{0} letöltés folyik", s.active)
    }

    var body: some View {
        HStack(spacing: 14) {
            Image(systemName: state.active > 0 ? "arrow.down.circle.fill" : "checkmark.circle.fill")
                .font(.system(size: 34))
                .foregroundStyle(Palette.amber)
            VStack(alignment: .leading, spacing: 4) {
                HStack(alignment: .firstTextBaseline) {
                    Text(Self.headline(state, stale: stale))
                        .font(.headline)
                        .foregroundStyle(Palette.bright)
                    Spacer(minLength: 8)
                    DownloadsStatus(state: state, stale: stale)
                        .font(.headline.monospacedDigit())
                        .foregroundStyle(Palette.amber)
                }
                if state.active > 0 {
                    Text(state.title).font(.subheadline).foregroundStyle(Palette.plaster).lineLimit(1)
                    DownloadsProgress(state: state, stale: stale)
                }
                if state.done > 0 && state.active > 0 || state.failed > 0 {
                    Text(footnote).font(.caption).foregroundStyle(Palette.muted)
                }
            }
        }
    }

    private var footnote: String {
        [state.done > 0 ? L("{0} kész", state.done) : nil, state.failed > 0 ? L("{0} sikertelen", state.failed) : nil]
            .compactMap { $0 }.joined(separator: " · ")
    }
}

/// The figure beside the headline: the remaining time counting down on its own once the speed is
/// known (an estimate, hence "kb."), the measured percentage until then, nothing while preparing.
struct DownloadsStatus: View {
    var state: DownloadsActivityAttributes.ContentState
    var stale: Bool

    var body: some View {
        if DownloadsLockView.finishing(state, stale: stale) || !DownloadsLockView.showsPercent(state) {
            EmptyView()
        } else if let eta = state.timeline?.upperBound, eta > .now {
            // A timer text takes all the width it is offered: give it a fixed one, or the view can't be laid out.
            HStack(spacing: 4) {
                Text(L("kb."))
                Text(timerInterval: Date.now...eta, countsDown: true)
                    .multilineTextAlignment(.trailing)
                    .frame(width: 52, alignment: .trailing)
            }
        } else {
            Text(DownloadsLockView.percent(state))
        }
    }
}

/// The download bar. With an estimate it runs on its own towards the estimated finish (the app is
/// suspended in the background and cannot move it); the next real figure corrects it.
struct DownloadsProgress: View {
    var state: DownloadsActivityAttributes.ContentState
    var stale: Bool
    var circular = false

    var body: some View {
        Group {
            if DownloadsLockView.finishing(state, stale: stale) {
                styled(ProgressView(value: 0.98))
            } else if let span = state.timeline {
                styled(ProgressView(timerInterval: span, countsDown: false) { EmptyView() } currentValueLabel: { EmptyView() })
            } else {
                styled(ProgressView(value: min(max(state.fraction, 0), 1)))
            }
        }
        .tint(Palette.amber)
    }

    @ViewBuilder private func styled(_ view: some View) -> some View {
        if circular { view.progressViewStyle(.circular) } else { view }
    }
}

// MARK: - watch party

struct PartyLiveActivity: Widget {
    var body: some WidgetConfiguration {
        ActivityConfiguration(for: PartyActivityAttributes.self) { context in
            PartyLockView(code: context.attributes.code, state: context.state)
                .padding(16)
                .activityBackgroundTint(Palette.black.opacity(0.9))
                .activitySystemActionForegroundColor(Palette.amber)
        } dynamicIsland: { context in
            let s = context.state
            return DynamicIsland {
                DynamicIslandExpandedRegion(.leading) {
                    Label("\(s.members)", systemImage: "person.2.fill")
                        .font(.headline)
                        .foregroundStyle(Palette.amber)
                }
                DynamicIslandExpandedRegion(.trailing) {
                    Text(context.attributes.code)
                        .font(.headline.monospaced())
                        .foregroundStyle(Palette.bright)
                }
                DynamicIslandExpandedRegion(.bottom) {
                    VStack(alignment: .leading, spacing: 6) {
                        Text(s.title).font(.subheadline.weight(.semibold)).lineLimit(1)
                        PartyProgress(state: s)
                    }
                }
            } compactLeading: {
                Image(systemName: "person.2.fill").foregroundStyle(Palette.amber)
            } compactTrailing: {
                Text("\(s.members)").font(.caption.monospacedDigit()).foregroundStyle(Palette.bright)
            } minimal: {
                Image(systemName: "person.2.fill").foregroundStyle(Palette.amber)
            }
        }
    }
}

struct PartyLockView: View {
    var code: String
    var state: PartyActivityAttributes.ContentState

    var body: some View {
        VStack(alignment: .leading, spacing: 8) {
            HStack {
                Label(L("Watch party"), systemImage: "person.2.fill")
                    .font(.caption.weight(.semibold))
                    .foregroundStyle(Palette.amber)
                Spacer()
                Text(state.members == 1 ? L("csak te") : L("{0} néző", state.members))
                    .font(.caption)
                    .foregroundStyle(Palette.muted)
                Text(code)
                    .font(.caption.monospaced().weight(.semibold))
                    .foregroundStyle(Palette.plaster)
            }
            HStack(spacing: 8) {
                Image(systemName: state.playing ? "play.fill" : "pause.fill")
                    .foregroundStyle(Palette.bright)
                Text(state.title)
                    .font(.headline)
                    .foregroundStyle(Palette.bright)
                    .lineLimit(1)
            }
            PartyProgress(state: state)
        }
    }
}

/// Runs on its own while the room plays; stands still while it is paused.
struct PartyProgress: View {
    var state: PartyActivityAttributes.ContentState

    var body: some View {
        if let span = state.timeline {
            ProgressView(timerInterval: span, countsDown: false) { EmptyView() } currentValueLabel: { EmptyView() }
                .tint(Palette.amber)
        } else {
            ProgressView(value: state.fraction).tint(Palette.amber)
        }
    }
}

// MARK: - requests (driven by the server through push)

struct RequestLiveActivity: Widget {
    var body: some WidgetConfiguration {
        ActivityConfiguration(for: RequestActivityAttributes.self) { context in
            let s = RequestLive.open(context)
            RequestLockView(state: s, stale: context.isStale)
                .padding(16)
                .activityBackgroundTint(Palette.black.opacity(0.9))
                .activitySystemActionForegroundColor(Palette.amber)
                .widgetURL(RequestLive.link(s))
        } dynamicIsland: { context in
            let s = RequestLive.open(context)
            let stale = context.isStale
            return DynamicIsland {
                DynamicIslandExpandedRegion(.leading) {
                    Image(systemName: RequestLive.icon(s))
                        .font(.title2)
                        .foregroundStyle(Palette.amber)
                }
                DynamicIslandExpandedRegion(.trailing) {
                    RequestStatus(state: s, stale: stale)
                        .font(.headline.monospacedDigit())
                        .foregroundStyle(Palette.bright)
                }
                DynamicIslandExpandedRegion(.bottom) {
                    VStack(alignment: .leading, spacing: 6) {
                        Text(RequestLive.headline(s, stale: stale)).font(.subheadline.weight(.semibold))
                        if let s {
                            Text(s.t).font(.caption).foregroundStyle(Palette.muted).lineLimit(1)
                            if s.running { RequestProgress(state: s, stale: stale) }
                        }
                    }
                }
            } compactLeading: {
                Image(systemName: "tray.and.arrow.down.fill").foregroundStyle(Palette.amber)
            } compactTrailing: {
                if let s, s.running {
                    RequestProgress(state: s, stale: stale, circular: true)
                } else {
                    Image(systemName: RequestLive.icon(s)).foregroundStyle(Palette.amber)
                }
            } minimal: {
                if let s, s.running {
                    RequestProgress(state: s, stale: stale, circular: true)
                } else {
                    Image(systemName: RequestLive.icon(s)).foregroundStyle(Palette.amber)
                }
            }
            .widgetURL(RequestLive.link(s))
        }
    }
}

enum RequestLive {
    /// Opens the sealed state with the key the app keeps for that server; nil when it can't (unpaired since).
    static func open(_ context: ActivityViewContext<RequestActivityAttributes>) -> PushLiveState? {
        guard let key = PushStore.shared().key(relayServer: context.attributes.s) else { return nil }
        return try? PushCrypto.liveState(context.state.e, key: key)
    }

    static func icon(_ s: PushLiveState?) -> String {
        switch s?.st {
        case "available": "checkmark.circle.fill"
        case "failed", "canceled": "xmark.circle.fill"
        case "importing": "gearshape.2.fill"
        default: "tray.and.arrow.down.fill"
        }
    }

    static func headline(_ s: PushLiveState?, stale: Bool) -> String {
        guard let s else { return L("Kérés beszerzése") }
        switch s.st {
        case "available": return s.k == "series" ? L("Megérkezett a sorozat") : L("Megérkezett a film")
        case "failed": return L("Nem sikerült beszerezni")
        case "canceled": return L("A beszerzés leállt")
        case "importing": return L("Hozzáadás a könyvtárhoz…")
        default: return stale && s.finishes != nil ? L("Befejezés…") : L("Letöltés a szerverre")
        }
    }

    static func link(_ s: PushLiveState?) -> URL? {
        guard let s, s.st == "available", let item = s.item else { return nil }
        return KinoraLink.item(Int(item)).url
    }
}

struct RequestLockView: View {
    var state: PushLiveState?
    var stale = false

    var body: some View {
        HStack(spacing: 14) {
            Image(systemName: RequestLive.icon(state))
                .font(.system(size: 34))
                .foregroundStyle(Palette.amber)
            VStack(alignment: .leading, spacing: 4) {
                HStack(alignment: .firstTextBaseline) {
                    Text(RequestLive.headline(state, stale: stale))
                        .font(.headline)
                        .foregroundStyle(Palette.bright)
                    Spacer(minLength: 8)
                    RequestStatus(state: state, stale: stale)
                        .font(.headline.monospacedDigit())
                        .foregroundStyle(Palette.amber)
                }
                if let state {
                    Text(state.t).font(.subheadline).foregroundStyle(Palette.plaster).lineLimit(1)
                    if state.running {
                        RequestProgress(state: state, stale: stale)
                    } else if state.st == "available" {
                        Text(L("Már nézheted.")).font(.caption).foregroundStyle(Palette.muted)
                    }
                } else {
                    Text(L("Nyisd meg az appot a részletekért.")).font(.caption).foregroundStyle(Palette.muted)
                }
            }
        }
    }
}

/// Remaining time counting down on its own once the server knows the speed, the percentage until then.
struct RequestStatus: View {
    var state: PushLiveState?
    var stale: Bool

    var body: some View {
        if let s = state, s.st == "downloading", !stale || s.finishes == nil {
            if let eta = s.timeline?.upperBound, eta > .now {
                HStack(spacing: 4) {
                    Text(L("kb."))
                    Text(timerInterval: Date.now...eta, countsDown: true)
                        .multilineTextAlignment(.trailing)
                        .frame(width: 52, alignment: .trailing)
                }
            } else {
                Text(min(max(s.p, 0), 1).formatted(.percent.precision(.fractionLength(0))))
            }
        } else {
            EmptyView()
        }
    }
}

struct RequestProgress: View {
    var state: PushLiveState
    var stale: Bool
    var circular = false

    var body: some View {
        Group {
            if state.st == "importing" || stale && state.finishes != nil {
                styled(ProgressView(value: 0.98))
            } else if let span = state.timeline {
                styled(ProgressView(timerInterval: span, countsDown: false) { EmptyView() } currentValueLabel: { EmptyView() })
            } else {
                styled(ProgressView(value: min(max(state.p, 0), 1)))
            }
        }
        .tint(Palette.amber)
    }

    @ViewBuilder private func styled(_ view: some View) -> some View {
        if circular { view.progressViewStyle(.circular) } else { view }
    }
}
