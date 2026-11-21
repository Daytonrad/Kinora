import SwiftUI
import KinoraKit

/// A film or series from TMDB: what it is, where it streams, and the request (or the way to ask for it).
struct DiscoverDetailSheet: View {
    enum SeasonMode: String, Hashable { case all, future, seasons }

    @Environment(AppModel.self) private var model
    @Environment(\.dismiss) private var dismiss
    var item: DiscoverItem
    /// The title is in the library: open it there.
    var open: (Item) -> Void
    /// A request was sent or taken back.
    var changed: () -> Void

    @State private var detail: Detail?
    @State private var error: String?
    @State private var libraries: [RequestLibraries.Lib]?
    @State private var library: Int?
    @State private var seasonMode = SeasonMode.all
    @State private var chosen = Set<Int>()
    @State private var busy = false
    @State private var actionError: String?
    @State private var sent: String?
    @State private var confirmCancel = false
    /// Other sites' ratings and the box office, loaded after the details.
    @State private var scores: Scores?
    @Environment(\.horizontalSizeClass) private var hSize

    /// The film and series details in one shape.
    struct Detail {
        var id: Int
        var title: String
        var originalTitle: String?
        var facts: String
        /// TMDB's score, shown in the facts until the ratings strip arrives.
        var rating: Double?
        var overview: String?
        var poster: String?
        var watch: WhereToWatch?
        var itemId: Int?
        var request: AcqRequest?
        var followed: Bool
        var seasons: [SeasonState]
        /// The profile's watch history of it (also after the file is gone).
        var watched: Watched?
    }

    var body: some View {
        NavigationStack {
            ScrollView {
                if let detail {
                    content(detail)
                        .frame(maxWidth: 640, alignment: .leading)
                        .padding(20)
                        .frame(maxWidth: .infinity)
                } else {
                    LoadingState(error: error) { Task { await load() } }
                }
            }
            .kinoraScreen()
            .navigationTitle(item.title)
            .navigationBarTitleDisplayMode(.inline)
            .toolbar {
                ToolbarItem(placement: .cancellationAction) { Button(L("Bezárás")) { dismiss() } }
            }
            .safeAreaInset(edge: .bottom) { if let detail { actions(detail) } }
            .confirmationDialog(L("Visszavonod a kérést?"), isPresented: $confirmCancel, titleVisibility: .visible) {
                Button(L("Visszavonás"), role: .destructive) { Task { await cancel() } }
            }
        }
        .presentationDetents([.large])
        .presentationBackground(Theme.black)
        .task { if detail == nil { await load() } }
        .task { await loadScores() }
    }

    // MARK: - content

    @ViewBuilder private func content(_ d: Detail) -> some View {
        VStack(alignment: .leading, spacing: 18) {
            HStack(alignment: .top, spacing: 16) {
                RemotePoster(url: d.poster, title: d.title)
                    .frame(width: 120)
                    .clipShape(.rect(cornerRadius: 8))
                VStack(alignment: .leading, spacing: 6) {
                    Text(d.title).font(Theme.display(28)).foregroundStyle(Theme.plasterBright)
                        .fixedSize(horizontal: false, vertical: true)
                    if let o = d.originalTitle, !o.isEmpty, o != d.title {
                        Text(o).font(.subheadline).foregroundStyle(Theme.muted)
                    }
                    let facts = facts(d)
                    if !facts.isEmpty {
                        Text(facts).font(.subheadline).foregroundStyle(Theme.plaster)
                    }
                }
            }

            if let s = scores, !s.ratings.isEmpty {
                ScoresStrip(ratings: s.ratings, style: .compact, columns: hSize == .regular ? 4 : 2)
            }

            if let w = d.watched {
                SeenBlock(watched: w, series: item.isSeries, seasons: d.seasons)
            }

            status(d)

            if let o = d.overview, !o.isEmpty {
                Text(o).font(.body).foregroundStyle(Theme.plaster).fixedSize(horizontal: false, vertical: true)
            }

            if d.itemId == nil && d.request == nil {
                if item.isSeries && !d.seasons.isEmpty { seasonPicker(d.seasons) }
                libraryPicker
            }

            if let s = scores, s.hasBoxOffice, !item.isSeries {
                VStack(alignment: .leading, spacing: 10) {
                    Text(L("Költségvetés és bevétel")).font(.headline)
                    BoxOfficeView(scores: s, style: .compact)
                }
            }

            if let w = d.watch { WatchBlock(watch: w) }
        }
    }

    /// The facts line, with TMDB's star while no ratings strip stands in for it.
    private func facts(_ d: Detail) -> String {
        let star = (d.rating ?? 0) > 0 && scores?.ratings.isEmpty != false ? String(format: "★ %.1f", d.rating ?? 0) : nil
        return Self.join([d.facts.isEmpty ? nil : d.facts, star])
    }

    private func loadScores() async {
        guard scores == nil else { return }
        let found = try? await model.api { [item] in try await $0.discoverScores(item.id, series: item.isSeries) }
        withAnimation(.easeOut(duration: 0.25)) { scores = found }
    }

    @ViewBuilder private func status(_ d: Detail) -> some View {
        if let sent {
            Label(sent, systemImage: "checkmark.circle.fill")
                .font(.callout.weight(.semibold)).foregroundStyle(Theme.amber)
                .fixedSize(horizontal: false, vertical: true)
        }
        if let r = d.request {
            VStack(alignment: .leading, spacing: 6) {
                HStack(spacing: 8) {
                    Text(r.status.label)
                        .font(.caption.weight(.bold))
                        .padding(.horizontal, 8).padding(.vertical, 4)
                        .background(Theme.raised, in: .capsule)
                    if r.isSeries { Text(seasonsLabel(r.seasons)).font(.caption).foregroundStyle(Theme.muted) }
                    if let by = r.userName, !by.isEmpty, r.userId != model.me?.user.id {
                        Text(L("kérte: {0}", by)).font(.caption).foregroundStyle(Theme.muted)
                    }
                }
                if r.status == .downloading, let p = r.progress, p > 0 {
                    ProgressView(value: p).tint(Theme.amber)
                    Text("\(Int(p * 100))%").font(.caption).foregroundStyle(Theme.muted)
                }
                if let n = r.note, !n.isEmpty {
                    Text(Language.shared.server(n)).font(.caption).foregroundStyle(Theme.muted)
                }
            }
        } else if d.itemId != nil {
            Label(item.isSeries ? L("Van belőle a könyvtárban.") : L("Ez a film már megvan a könyvtárban."), systemImage: "checkmark.seal.fill")
                .font(.callout).foregroundStyle(Theme.amber)
        } else if d.followed {
            Label(L("Ezt a sorozatot már figyeli a szerver."), systemImage: "eye")
                .font(.callout).foregroundStyle(Theme.plaster)
        }
    }

    private func seasonPicker(_ seasons: [SeasonState]) -> some View {
        VStack(alignment: .leading, spacing: 10) {
            Text(L("Mit töltsön le?")).font(.headline)
            Picker(L("Mit töltsön le?"), selection: $seasonMode) {
                Text(L("Minden évad")).tag(SeasonMode.all)
                Text(L("Csak az új részek")).tag(SeasonMode.future)
                Text(L("Kiválasztott évadok")).tag(SeasonMode.seasons)
            }
            .pickerStyle(.segmented)
            if seasonMode == .seasons {
                VStack(spacing: 0) {
                    ForEach(seasons) { s in
                        Toggle(isOn: Binding(get: { chosen.contains(s.number) },
                                             set: { on in Haptic.select.play(); if on { chosen.insert(s.number) } else { chosen.remove(s.number) } })) {
                            VStack(alignment: .leading, spacing: 2) {
                                Text(L("{0}. évad", s.number))
                                Text([s.episodeCount.map { L("{0} rész", $0) }, (s.have ?? 0) > 0 ? L("{0} megvan", s.have ?? 0) : nil]
                                    .compactMap { $0 }.joined(separator: " · "))
                                    .font(.caption).foregroundStyle(Theme.muted)
                            }
                        }
                        .tint(Theme.amber)
                        .padding(.vertical, 8)
                        if s.id != seasons.last?.id { Divider().overlay(Theme.rule) }
                    }
                }
                .padding(.horizontal, 14)
                .background(Theme.lacquer, in: .rect(cornerRadius: 10))
            }
        }
    }

    @ViewBuilder private var libraryPicker: some View {
        if let libraries {
            if libraries.isEmpty {
                Text(item.isSeries ? L("Még nincs sorozatkönyvtár, ezért most nem lehet sorozatot kérni.")
                                   : L("Még nincs filmkönyvtár, ezért most nem lehet filmet kérni."))
                    .font(.callout).foregroundStyle(Theme.danger)
            } else if libraries.count > 1 {
                VStack(alignment: .leading, spacing: 8) {
                    Text(L("Melyik könyvtárba kerüljön?")).font(.headline)
                    Picker(L("Könyvtár"), selection: $library) {
                        ForEach(libraries) { Text($0.name).tag(Optional($0.id)) }
                    }
                    .pickerStyle(.menu)
                    .tint(Theme.plasterBright)
                    .frame(maxWidth: .infinity, alignment: .leading)
                    .kinoraField()
                }
            }
        }
    }

    // MARK: - actions

    @ViewBuilder private func actions(_ d: Detail) -> some View {
        VStack(spacing: 8) {
            ErrorText(message: actionError)
            if let id = d.itemId {
                Button {
                    var it = Item(id: id, kind: item.isSeries ? .series : .movie, title: d.title)
                    it.year = item.year
                    open(it)
                } label: { Label(L("Megnézem"), systemImage: "play.fill") }
                .buttonStyle(PrimaryButtonStyle())
            } else if let r = d.request {
                if r.cancellable && r.userId == model.me?.user.id {
                    Button(role: .destructive) { confirmCancel = true } label: { Text(L("Visszavonás")) }
                        .buttonStyle(SecondaryButtonStyle())
                        .disabled(busy)
                } else if r.removable && r.userId == model.me?.user.id {
                    // it ended without the film: clear it away, and the title can be asked for again
                    Button(role: .destructive) { Task { await cancel() } } label: { Label(L("Eltávolítás"), systemImage: "trash") }
                        .buttonStyle(SecondaryButtonStyle())
                        .disabled(busy)
                }
            } else {
                Button { Task { await request(d) } } label: {
                    if busy { ProgressView().tint(Theme.amberInk) } else { Label(L("Kérem"), systemImage: "plus") }
                }
                .buttonStyle(PrimaryButtonStyle())
                .disabled(busy || !canSend)
            }
        }
        .padding(.horizontal, 20)
        .padding(.top, 10)
        .padding(.bottom, 8)
        .frame(maxWidth: 680)
        .frame(maxWidth: .infinity)
        .background(Theme.black.opacity(0.96))
    }

    private var canSend: Bool {
        guard let libraries, !libraries.isEmpty else { return false }
        if item.isSeries && seasonMode == .seasons && chosen.isEmpty { return false }
        return libraries.count == 1 || library != nil
    }

    private func load() async {
        do {
            let d: Detail = try await model.api { [item] client in
                if item.isSeries {
                    let s = try await client.discoverSeries(item.id)
                    let seasons = s.seasons ?? []
                    let facts = [s.year.map(String.init), seasons.isEmpty ? nil : L("{0} évad", seasons.count),
                                 (s.genres ?? []).prefix(3).joined(separator: ", "), s.ended == true ? L("befejeződött") : nil]
                    return Detail(id: s.id, title: s.title, originalTitle: s.originalTitle, facts: Self.join(facts), rating: nil, overview: s.overview,
                                  poster: s.poster, watch: s.watch, itemId: s.itemId, request: s.request,
                                  followed: s.followed == true, seasons: seasons, watched: s.watched)
                }
                let m = try await client.discoverMovie(item.id)
                let runtime = (m.runtime ?? 0) > 0 ? L("{0} perc", m.runtime ?? 0) : nil
                let facts: [String?] = [m.year.map(String.init), runtime, (m.genres ?? []).prefix(3).joined(separator: ", ")]
                return Detail(id: m.id, title: m.title, originalTitle: m.originalTitle, facts: Self.join(facts), rating: m.rating,
                              overview: m.overview,
                              poster: m.poster, watch: m.watch, itemId: m.itemId, request: m.request, followed: false, seasons: [], watched: m.watched)
            }
            if d.itemId == nil && d.request == nil && libraries == nil {
                let libs = try await model.api { try await $0.requestLibraries() }
                let list = item.isSeries ? libs.shows : libs.movies
                libraries = list
                if list.count == 1 { library = list[0].id }
            }
            error = nil
            detail = d
        } catch is CancellationError {
        } catch {
            self.error = error.localizedDescription
        }
    }

    private nonisolated static func join(_ parts: [String?]) -> String {
        parts.compactMap { $0 }.filter { !$0.isEmpty }.joined(separator: " · ")
    }

    private func request(_ d: Detail) async {
        busy = true
        actionError = nil
        defer { busy = false }
        do {
            let lib = library
            let r: AcqRequest
            if item.isSeries {
                let seasons = seasonMode == .seasons ? chosen.sorted().map(String.init).joined(separator: ",") : seasonMode.rawValue
                r = try await model.api { try await $0.requestSeries(d.id, seasons: seasons, library: lib) }
            } else {
                r = try await model.api { try await $0.requestMovie(d.id, library: lib) }
            }
            Haptic.success.play()
            sent = r.status == .pending ? L("Elküldve: {0}. Az admin jóváhagyása után indul a letöltés.", d.title)
                                        : L("Kérve: {0}. A szerver már keresi.", d.title)
            detail?.request = r
            changed()
        } catch {
            Haptic.error.play()
            actionError = error.localizedDescription
        }
    }

    private func cancel() async {
        guard let r = detail?.request else { return }
        busy = true
        actionError = nil
        defer { busy = false }
        do {
            try await model.api { try await $0.cancelRequest(r.id) }
            Haptic.warning.play()
            sent = nil
            changed()
            dismiss()
        } catch {
            actionError = error.localizedDescription
        }
    }
}

/// Where a title streams in the server's region (TMDB, data by JustWatch).
struct WatchBlock: View {
    var watch: WhereToWatch

    var body: some View {
        VStack(alignment: .leading, spacing: 10) {
            Text(L("Hol nézhető? ({0})", watch.region)).font(.headline)
            let stream = watch.stream ?? []
            if stream.isEmpty {
                Text(L("Most egyik előfizetéses szolgáltatónál sem érhető el.")).font(.callout).foregroundStyle(Theme.muted)
            } else {
                providers(stream)
            }
            let other = ((watch.rent ?? []) + (watch.buy ?? [])).reduce(into: [WatchProvider]()) { acc, p in
                if !acc.contains(where: { $0.id == p.id }) && !stream.contains(where: { $0.id == p.id }) { acc.append(p) }
            }
            if !other.isEmpty {
                Text(L("Kölcsönözhető vagy megvásárolható")).font(.subheadline).foregroundStyle(Theme.muted)
                providers(other)
            }
            Text(L("Streaming-adatok: JustWatch, a TMDB-n keresztül")).font(.caption2).foregroundStyle(Theme.muted)
        }
    }

    private func providers(_ list: [WatchProvider]) -> some View {
        ScrollView(.horizontal, showsIndicators: false) {
            HStack(spacing: 10) {
                ForEach(list) { p in
                    AsyncImage(url: p.logo.flatMap(URL.init(string:))) { img in
                        img.resizable().scaledToFill()
                    } placeholder: {
                        Text(String(p.name.prefix(2))).font(.caption.weight(.bold)).foregroundStyle(Theme.plaster)
                    }
                    .frame(width: 40, height: 40)
                    .background(Theme.raised)
                    .clipShape(.rect(cornerRadius: 9))
                    .accessibilityLabel(p.name)
                }
            }
        }
    }
}

/// "You have seen this": when, and for a series how far into each season.
struct SeenBlock: View {
    var watched: Watched
    var series: Bool
    var seasons: [SeasonState]

    private var title: String {
        guard series else { return L("Láttad") }
        return watched.complete(seasons) ? L("Végignézted") : L("{0} részt láttál", watched.episodes ?? 0)
    }

    private var seen: [(season: SeasonState, n: Int, of: Int)] {
        seasons.compactMap { s in
            let n = watched.episodes(inSeason: s.number)
            guard n > 0 else { return nil }
            let of = max(s.episodeCount ?? n, 1)
            return (s, min(n, of), of)
        }
    }

    var body: some View {
        VStack(alignment: .leading, spacing: 12) {
            HStack(spacing: 12) {
                Image(systemName: "checkmark")
                    .font(.system(size: 15, weight: .bold))
                    .foregroundStyle(Theme.amberInk)
                    .frame(width: 34, height: 34)
                    .background(Theme.amber, in: .circle)
                    .accessibilityHidden(true)
                VStack(alignment: .leading, spacing: 2) {
                    Text(title).font(.headline).foregroundStyle(Theme.plasterBright)
                    if let d = watched.date {
                        Text(L("utoljára {0}", d.formatted(Date.FormatStyle(date: .abbreviated, time: .omitted).locale(Language.shared.locale))))
                            .font(.caption).foregroundStyle(Theme.muted)
                    }
                }
            }
            if series && !seen.isEmpty {
                Grid(alignment: .leading, horizontalSpacing: 12, verticalSpacing: 8) {
                    ForEach(seen, id: \.season.number) { row in
                        GridRow {
                            Text(L("{0}. évad", row.season.number)).font(.subheadline).foregroundStyle(Theme.plaster)
                            ProgressView(value: Double(row.n), total: Double(row.of))
                                .tint(Theme.amber)
                                .accessibilityHidden(true)
                            Text("\(row.n) / \(row.of)").font(.caption.monospacedDigit()).foregroundStyle(Theme.muted)
                        }
                        .accessibilityElement(children: .ignore)
                        .accessibilityLabel(L("{0}. évad", row.season.number) + ", " + "\(row.n) / \(row.of)")
                    }
                }
            }
        }
        .padding(14)
        .frame(maxWidth: .infinity, alignment: .leading)
        .background(Theme.amber.opacity(0.07), in: .rect(cornerRadius: 10))
        .overlay(RoundedRectangle(cornerRadius: 10).strokeBorder(Theme.amber.opacity(0.45), lineWidth: 1))
        .accessibilityElement(children: .contain)
    }
}
