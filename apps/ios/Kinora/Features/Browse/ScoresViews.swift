import KinoraKit
import SwiftUI

// Other sites' ratings and the box office of a film or series, below the title on the detail
// page and in the Felfedezés sheet; on Apple TV in the detail header. Same rules as the web
// client (scoreStrip / boxOfficeBlock in app.js): plaster figures and hairline meters, no
// brand colours, the amber stays for focus and live states.

/// Where the views sit: they size themselves for it.
enum ScoresStyle {
    /// iPhone / iPad detail page.
    case page
    /// The Felfedezés sheet: smaller, no footnote.
    case compact
    /// Apple TV detail header, read from the sofa.
    case tv

    fileprivate var figure: CGFloat {
        switch self {
        case .page: 26
        case .compact: 20
        case .tv: 46
        }
    }

    fileprivate var label: Font {
        switch self {
        case .page, .compact: .caption2.weight(.semibold)
        case .tv: .caption.weight(.semibold)
        }
    }

    fileprivate var small: Font {
        switch self {
        case .page, .compact: .caption
        case .tv: .callout
        }
    }

    fileprivate var padding: CGFloat {
        switch self {
        case .page: 12
        case .compact: 10
        case .tv: 22
        }
    }

    fileprivate var meter: CGFloat { self == .tv ? 4 : 2 }
}

enum ScoreFormat {
    private static var locale: Locale { Language.shared.locale }

    /// "8,7" for a 10-point site, "83" for a 100-point one.
    static func figure(_ s: Score) -> String {
        if s.max == 100 { return Int(s.value.rounded()).formatted(.number.locale(locale)) }
        return s.value.formatted(.number.precision(.fractionLength(1)).locale(locale))
    }

    /// The scale after the figure.
    static func scale(_ s: Score) -> String {
        if s.source == "rt" { return "%" }
        return s.max == 100 ? "/100" : "/10"
    }

    /// "2,1 M" votes.
    static func compact(_ n: Int64) -> String {
        Double(n).formatted(.number.notation(.compactName).precision(.fractionLength(0...1)).locale(locale))
    }

    /// "$63 000 000", or "$63M" when compact.
    static func usd(_ v: Int64, compact: Bool) -> String {
        let style = FloatingPointFormatStyle<Double>.Currency(code: "USD", locale: locale).presentation(.narrow)
        if compact {
            if #available(iOS 18, tvOS 18, *) {
                return Double(v).formatted(style.notation(.compactName).precision(.fractionLength(0...1)))
            }
            return "$" + Double(v).formatted(.number.notation(.compactName).precision(.fractionLength(0...1)).locale(locale))
        }
        return Double(v).formatted(style.precision(.fractionLength(0)))
    }

    static func number(_ v: Double, digits: Int) -> String {
        v.formatted(.number.precision(.fractionLength(digits)).locale(locale))
    }

    /// "Made 7.4× its budget" / "Grossed 60% of its budget".
    static func ratio(_ r: Double) -> String {
        r >= 1 ? L("{0}× a költségvetéshez képest", number(r, digits: r >= 10 ? 0 : 1))
            : L("A költségvetés {0}%-át hozta be", number(r * 100, digits: 0))
    }
}

/// The rating cells: site, figure on its scale, a hairline meter and the votes. On iOS a cell
/// with a link opens the site.
struct ScoresStrip: View {
    var ratings: [Score]
    var style: ScoresStyle = .page
    /// Cells in a row; the phone wraps four sites into two rows (three still fit in one).
    var columns: Int = 4

    var body: some View {
        let cols = max(1, min(ratings.count == 3 ? max(columns, 3) : columns, ratings.count))
        Grid(horizontalSpacing: style == .tv ? 24 : 8, verticalSpacing: style == .tv ? 24 : 8) {
            ForEach(Array(stride(from: 0, to: ratings.count, by: cols)), id: \.self) { start in
                GridRow {
                    ForEach(ratings[start ..< min(start + cols, ratings.count)]) { s in
                        cell(s)
                    }
                    // Keep the last row's cells as wide as the ones above.
                    ForEach(0 ..< (cols - min(cols, ratings.count - start)), id: \.self) { _ in
                        Color.clear.gridCellUnsizedAxes([.horizontal, .vertical])
                    }
                }
            }
        }
        .accessibilityElement(children: .contain)
        .accessibilityLabel(L("Értékelések"))
    }

    @ViewBuilder private func cell(_ s: Score) -> some View {
        #if os(tvOS)
        ScoreCell(score: s, style: style)
        #else
        if let link = s.url.flatMap(URL.init(string:)) {
            Link(destination: link) { ScoreCell(score: s, style: style, linked: true) }
                .buttonStyle(.plain)
                .accessibilityHint(L("{0} megnyitása", s.siteName))
        } else {
            ScoreCell(score: s, style: style)
        }
        #endif
    }
}

private struct ScoreCell: View {
    var score: Score
    var style: ScoresStyle
    var linked = false

    var body: some View {
        VStack(alignment: .leading, spacing: style == .tv ? 10 : 6) {
            HStack(spacing: 4) {
                Text(score.siteName.uppercased())
                    .font(style.label)
                    .tracking(0.6)
                    .foregroundStyle(Theme.plaster)
                    .lineLimit(1)
                    .minimumScaleFactor(0.6)
                if linked {
                    Image(systemName: "arrow.up.right").font(.system(size: 8, weight: .bold)).foregroundStyle(Theme.muted)
                }
            }
            HStack(alignment: .firstTextBaseline, spacing: 2) {
                Text(ScoreFormat.figure(score))
                    .font(.system(size: style.figure, weight: .bold).monospacedDigit())
                    .foregroundStyle(Theme.plasterBright)
                Text(ScoreFormat.scale(score))
                    .font(.system(size: style.figure * 0.5, weight: .semibold).monospacedDigit())
                    .foregroundStyle(Theme.muted)
            }
            Meter(fraction: score.fraction, height: style.meter)
            Text(sub)
                .font(style.small)
                .foregroundStyle(Theme.muted)
                .lineLimit(1)
                .minimumScaleFactor(0.8)
        }
        .padding(style.padding)
        .frame(maxWidth: .infinity, alignment: .leading)
        .background(Theme.black.opacity(0.35), in: .rect(cornerRadius: 6))
        .overlay(RoundedRectangle(cornerRadius: 6).stroke(Theme.rule))
        .contentShape(.rect(cornerRadius: 6))
        .accessibilityElement(children: .ignore)
        .accessibilityLabel("\(score.siteName): \(ScoreFormat.figure(score))\(ScoreFormat.scale(score)), \(sub)")
    }

    /// "2,1 M szavazat" or "kritikusok"; a blank line keeps the cells level.
    private var sub: String {
        if let v = score.votes, v > 0 { return L("{0} szavazat", ScoreFormat.compact(v)) }
        return score.isCritics ? L("kritikusok") : " "
    }
}

/// A hairline 0…1 bar.
private struct Meter: View {
    var fraction: Double
    var height: CGFloat
    var tint: Color = Theme.plaster

    var body: some View {
        GeometryReader { g in
            ZStack(alignment: .leading) {
                Rectangle().fill(Theme.rule)
                Rectangle().fill(tint).frame(width: g.size.width * fraction)
            }
        }
        .frame(height: height)
        .clipShape(.rect(cornerRadius: min(height / 2, 2)))
        .accessibilityHidden(true)
    }
}

/// Budget and worldwide gross side by side as bars on one scale, with what the gross made of
/// the budget. Films only; nothing when TMDB knows neither.
struct BoxOfficeView: View {
    var scores: Scores
    var style: ScoresStyle = .page

    var body: some View {
        let budget = scores.budget ?? 0, revenue = scores.revenue ?? 0
        let top = Double(max(budget, revenue, 1))
        VStack(alignment: .leading, spacing: style == .tv ? 18 : 10) {
            if budget > 0 { row(L("Költségvetés"), hint: nil, value: budget, of: top, tint: Theme.muted) }
            if revenue > 0 { row(L("Bevétel"), hint: L("világszerte"), value: revenue, of: top, tint: Theme.plaster) }
            if let r = scores.ratio {
                Text(ScoreFormat.ratio(r))
                    .font(style == .tv ? .callout.weight(.semibold) : .subheadline.weight(.semibold))
                    .foregroundStyle(Theme.plasterBright)
                    .padding(.top, 2)
            }
            if style != .compact {
                Text(L("TMDB adatai, USD. A bevétel mozis jegybevétel, nem nyereség."))
                    .font(style.small)
                    .foregroundStyle(Theme.muted)
                    .fixedSize(horizontal: false, vertical: true)
            }
        }
    }

    private func row(_ label: String, hint: String?, value: Int64, of top: Double, tint: Color) -> some View {
        VStack(alignment: .leading, spacing: style == .tv ? 8 : 5) {
            HStack(alignment: .firstTextBaseline, spacing: 8) {
                Text(label).font(style.small.weight(.semibold)).foregroundStyle(Theme.plaster)
                if let hint { Text(hint).font(style.small).foregroundStyle(Theme.muted) }
                Spacer(minLength: 8)
                Text(ScoreFormat.usd(value, compact: style == .compact))
                    .font(.system(size: style.figure * 0.72, weight: .bold).monospacedDigit())
                    .foregroundStyle(Theme.plasterBright)
            }
            Meter(fraction: Double(value) / top, height: style == .tv ? 8 : 6, tint: tint)
        }
        .accessibilityElement(children: .combine)
    }
}

/// What the strip says when nothing but TMDB is there and the viewer could fix that.
struct OmdbHint: View {
    var style: ScoresStyle = .page

    var body: some View {
        Text(L("IMDb, Rotten Tomatoes és Metacritic: adj meg OMDb-kulcsot az Admin → Metaadatok oldalon."))
            .font(style.small)
            .foregroundStyle(Theme.muted)
            .fixedSize(horizontal: false, vertical: true)
    }
}
