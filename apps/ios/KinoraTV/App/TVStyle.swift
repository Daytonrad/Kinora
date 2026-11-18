import SwiftUI
import KinoraKit

// Focus looks for the TV screens. A custom ButtonStyle gets no focus effect from tvOS,
// so these read `isFocused` and lift the focused control in the foyer palette.

/// Pill button: panel at rest (amber when prominent), light like the system buttons when focused.
struct TVPillStyle: ButtonStyle {
    var prominent = false

    func makeBody(configuration: Configuration) -> some View {
        PillBody(configuration: configuration, prominent: prominent)
    }

    private struct PillBody: View {
        @Environment(\.isFocused) private var focused
        @Environment(\.isEnabled) private var enabled
        let configuration: Configuration
        let prominent: Bool

        var body: some View {
            configuration.label
                .font(.headline)
                .labelStyle(.titleAndIcon)
                // A label never wraps or hyphenates ("Kedvencek-hez"): a row that runs out of room
                // moves whole buttons to the next line instead (TVButtonRow).
                .lineLimit(1)
                .fixedSize(horizontal: true, vertical: false)
                .padding(.horizontal, 34)
                .frame(minHeight: 66)
                .foregroundStyle(focused || prominent ? Theme.amberInk : Theme.plasterBright)
                .background(focused ? Theme.plasterBright : prominent ? Theme.amber : Theme.raised, in: .capsule)
                .opacity(enabled ? 1 : 0.45)
                .scaleEffect(focused ? (configuration.isPressed ? 1.02 : 1.08) : 1)
                .shadow(color: .black.opacity(focused ? 0.5 : 0), radius: 18, y: 10)
                .animation(.easeOut(duration: 0.15), value: focused)
        }
    }
}

/// Artwork card: grows, casts a shadow and gets an amber edge when focused.
struct TVCardStyle: ButtonStyle {
    func makeBody(configuration: Configuration) -> some View {
        CardBody(configuration: configuration)
    }

    private struct CardBody: View {
        @Environment(\.isFocused) private var focused
        let configuration: Configuration

        var body: some View {
            configuration.label
                .scaleEffect(focused ? (configuration.isPressed ? 1.04 : 1.1) : 1)
                .shadow(color: .black.opacity(focused ? 0.6 : 0), radius: 24, y: 14)
                .zIndex(focused ? 1 : 0)
                .animation(.easeOut(duration: 0.18), value: focused)
                .environment(\.tvCardFocused, focused)
        }
    }
}

/// Plain row in a list (settings, servers): a panel that turns amber when focused.
struct TVRowStyle: ButtonStyle {
    func makeBody(configuration: Configuration) -> some View {
        RowBody(configuration: configuration)
    }

    private struct RowBody: View {
        @Environment(\.isFocused) private var focused
        let configuration: Configuration

        var body: some View {
            configuration.label
                .labelStyle(TVRowLabelStyle())
                .padding(.horizontal, 30)
                .padding(.vertical, 22)
                .frame(maxWidth: .infinity, alignment: .leading)
                .foregroundStyle(focused ? Theme.amberInk : Theme.plasterBright)
                .background(focused ? Theme.amber : Theme.panel, in: .rect(cornerRadius: 16))
                .scaleEffect(focused ? (configuration.isPressed ? 0.99 : 1.03) : 1)
                .shadow(color: .black.opacity(focused ? 0.5 : 0), radius: 16, y: 8)
                .animation(.easeOut(duration: 0.15), value: focused)
                .environment(\.tvCardFocused, focused)
        }
    }
}

/// Row label: the icons sit in one fixed-width column, so the titles of a list start on the same line.
private struct TVRowLabelStyle: LabelStyle {
    func makeBody(configuration: Configuration) -> some View {
        HStack(spacing: 20) {
            configuration.icon.frame(width: 44)
            configuration.title
        }
    }
}

/// A row of pill buttons that wraps onto further lines when the buttons don't fit side by side
/// (Hungarian labels are long), instead of squeezing them until their titles break.
struct TVButtonRow: Layout {
    var spacing: CGFloat = 24
    var lineSpacing: CGFloat = 24

    func sizeThatFits(proposal: ProposedViewSize, subviews: Subviews, cache: inout ()) -> CGSize {
        let lines = lines(proposal.width ?? .infinity, subviews)
        let width = lines.map(\.width).max() ?? 0
        let height = lines.map(\.height).reduce(0, +) + lineSpacing * CGFloat(max(0, lines.count - 1))
        return CGSize(width: width, height: height)
    }

    func placeSubviews(in bounds: CGRect, proposal: ProposedViewSize, subviews: Subviews, cache: inout ()) {
        var y = bounds.minY
        for line in lines(bounds.width, subviews) {
            var x = bounds.minX
            for i in line.items {
                let size = subviews[i].sizeThatFits(.unspecified)
                subviews[i].place(at: CGPoint(x: x, y: y + (line.height - size.height) / 2), proposal: .unspecified)
                x += size.width + spacing
            }
            y += line.height + lineSpacing
        }
    }

    private struct Line { var items: [Int] = []; var width: CGFloat = 0; var height: CGFloat = 0 }

    private func lines(_ maxWidth: CGFloat, _ subviews: Subviews) -> [Line] {
        var lines: [Line] = []
        var line = Line()
        for i in subviews.indices {
            let size = subviews[i].sizeThatFits(.unspecified)
            if size.width == 0 && size.height == 0 { continue }
            let width = line.items.isEmpty ? size.width : line.width + spacing + size.width
            if !line.items.isEmpty && width > maxWidth {
                lines.append(line)
                line = Line(items: [i], width: size.width, height: size.height)
            } else {
                line.items.append(i)
                line.width = width
                line.height = max(line.height, size.height)
            }
        }
        if !line.items.isEmpty { lines.append(line) }
        return lines
    }
}

extension EnvironmentValues {
    /// The card or row this view is in has the focus (for secondary text colors).
    @Entry var tvCardFocused = false
}

/// Artwork with an amber ring while its card is focused.
struct TVArtwork: View {
    @Environment(\.tvCardFocused) private var focused
    var item: Item
    var kind: ImageKind
    var ratio: CGFloat

    var body: some View {
        ArtworkView(item: item, kind: kind)
            .aspectRatio(ratio, contentMode: .fit)
            .overlay(alignment: .bottom) { ProgressStripe(item: item).scaleEffect(y: 1.6, anchor: .bottom) }
            .overlay(alignment: .topTrailing) { WatchedBadge(item: item).scaleEffect(1.5, anchor: .topTrailing) }
            .clipShape(.rect(cornerRadius: 14))
            .overlay { RoundedRectangle(cornerRadius: 14).stroke(Theme.amber, lineWidth: focused ? 5 : 0) }
    }
}

/// Section heading on the TV screens.
struct TVHeading: View {
    var text: String
    var body: some View {
        Text(text)
            .font(.system(size: 34, weight: .bold))
            .foregroundStyle(Theme.plaster)
    }
}

/// Error line in the foyer danger color.
struct TVErrorText: View {
    var message: String?
    var body: some View {
        if let message {
            Text(message).font(.callout).foregroundStyle(Theme.danger)
                .fixedSize(horizontal: false, vertical: true)
                .onAppear { AccessibilityNotification.Announcement(message).post() }
                .onChange(of: message) { _, m in AccessibilityNotification.Announcement(m).post() }
        }
    }
}
