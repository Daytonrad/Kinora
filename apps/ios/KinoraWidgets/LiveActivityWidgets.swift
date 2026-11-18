import ActivityKit
import SwiftUI
import WidgetKit
import KinoraKit

// MARK: - downloads

struct DownloadsLiveActivity: Widget {
    var body: some WidgetConfiguration {
        ActivityConfiguration(for: DownloadsActivityAttributes.self) { context in
            DownloadsLockView(state: context.state)
                .padding(16)
                .activityBackgroundTint(Palette.black.opacity(0.9))
                .activitySystemActionForegroundColor(Palette.amber)
        } dynamicIsland: { context in
            let s = context.state
            return DynamicIsland {
                DynamicIslandExpandedRegion(.leading) {
                    Image(systemName: s.active > 0 ? "arrow.down.circle.fill" : "checkmark.circle.fill")
                        .font(.title2)
                        .foregroundStyle(Palette.amber)
                }
                DynamicIslandExpandedRegion(.trailing) {
                    Text(s.fraction, format: .percent.precision(.fractionLength(0)))
                        .font(.headline.monospacedDigit())
                        .foregroundStyle(Palette.bright)
                }
                DynamicIslandExpandedRegion(.bottom) {
                    VStack(alignment: .leading, spacing: 6) {
                        Text(DownloadsLockView.headline(s)).font(.subheadline.weight(.semibold))
                        if s.active > 0 {
                            Text(s.title).font(.caption).foregroundStyle(Palette.muted).lineLimit(1)
                            ProgressView(value: s.fraction).tint(Palette.amber)
                        }
                    }
                }
            } compactLeading: {
                Image(systemName: "arrow.down.circle.fill").foregroundStyle(Palette.amber)
            } compactTrailing: {
                if s.active > 0 {
                    ProgressView(value: s.fraction).progressViewStyle(.circular).tint(Palette.amber)
                } else {
                    Image(systemName: "checkmark").foregroundStyle(Palette.amber)
                }
            } minimal: {
                Image(systemName: s.active > 0 ? "arrow.down" : "checkmark").foregroundStyle(Palette.amber)
            }
            .widgetURL(URL(string: "kinora://downloads"))
        }
    }
}

struct DownloadsLockView: View {
    var state: DownloadsActivityAttributes.ContentState

    static func headline(_ s: DownloadsActivityAttributes.ContentState) -> String {
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
                Text(Self.headline(state))
                    .font(.headline)
                    .foregroundStyle(Palette.bright)
                if state.active > 0 {
                    Text(state.title).font(.subheadline).foregroundStyle(Palette.plaster).lineLimit(1)
                    ProgressView(value: state.fraction).tint(Palette.amber)
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
