import SwiftUI
import KinoraKit

/// Movie, series, season or episode page.
struct ItemDetailView: View {
    @Environment(AppModel.self) private var model
    @Environment(\.verticalSizeClass) private var vSize
    @Environment(\.metrics) private var metrics
    /// What the previous screen knew; shown at once while the full detail loads.
    var item: Item

    @State private var detail: ItemDetail?
    @State private var userData = UserData()
    @State private var error: String?
    @State private var season: Int?
    @State private var episodes: [Int: [Item]] = [:]
    @State private var overviewExpanded = false
    @State private var fetchingSeason: Int?
    @State private var askSeasonQuality = false
    @State private var episodesError: String?
    /// A short message after an action: a season download, a failed toggle.
    @State private var notice: Notice?

    private struct Notice {
        var title: String
        var message: String
    }

    private var current: Item { detail?.item ?? item }

    var body: some View {
        ScrollView {
            VStack(alignment: .leading, spacing: 20) {
                header
                actions
                overview
                if current.kind == .series || current.kind == .season { episodesSection }
                if let similar = detail?.similar, !similar.isEmpty { similarSection(similar) }
                if let error, detail == nil {
                    LoadingState(error: error) { Task { await load() } }
                }
            }
            .padding(.bottom, 32)
        }
        .refreshable { await load() }
        // Runs again on returning from an episode, so watched marks stay current.
        .task { await load() }
        .onChange(of: model.playbackGeneration) { Task { await load() } }
        .task(id: season) { await loadEpisodes() }
        .confirmationDialog("Évad letöltése", isPresented: $askSeasonQuality, titleVisibility: .visible) {
            ForEach(DownloadButton.qualities, id: \.0) { q in
                Button(q.1) { downloadSeason(q.0) }
            }
        } message: {
            Text("Az évad még nem letöltött részei sorban kerülnek az eszközre.")
        }
        .alert(notice?.title ?? "", isPresented: Binding(get: { notice != nil }, set: { if !$0 { notice = nil } })) {
            Button("OK", role: .cancel) {}
        } message: {
            Text(notice?.message ?? "")
        }
        .kinoraScreen()
        .navigationTitle(current.title)
        .navigationBarTitleDisplayMode(.inline)
    }

    // MARK: - sections

    private var header: some View {
        VStack(alignment: .leading, spacing: 0) {
            ArtworkView(item: current, kind: current.kind == .episode ? .thumb : .backdrop)
                .heroFrame(metrics.heroRatio(16 / 9), compactHeight: vSize == .compact)
                .overlay {
                    LinearGradient(colors: [.clear, Theme.black], startPoint: .center, endPoint: .bottom)
                }

            HStack(alignment: .bottom, spacing: 16) {
                if current.kind != .episode {
                    ArtworkView(item: current, kind: .poster)
                        .frame(width: metrics.regular ? 168 : 104, height: metrics.regular ? 252 : 156)
                        .clipShape(.rect(cornerRadius: 8))
                        .shadow(radius: 10)
                }
                VStack(alignment: .leading, spacing: 6) {
                    if current.kind == .episode || current.kind == .season, let s = current.seriesTitle {
                        Text(s).font(.subheadline.weight(.semibold)).foregroundStyle(Theme.amber)
                    }
                    if let label = current.episodeLabel {
                        Text(label).font(.caption.weight(.semibold)).foregroundStyle(Theme.muted)
                    }
                    Text(current.title)
                        .font(Theme.display(metrics.regular ? 48 : 32, weight: .bold))
                        .lineLimit(3)
                        .minimumScaleFactor(0.7)
                    let meta = Format.meta(current)
                    if !meta.isEmpty { Text(meta).font(.footnote).foregroundStyle(Theme.plaster) }
                    if let g = current.genres, !g.isEmpty {
                        Text(g.joined(separator: ", ")).font(.footnote).foregroundStyle(Theme.muted)
                    }
                }
            }
            .padding(.horizontal, metrics.gutter)
            .padding(.top, current.kind == .episode ? 0 : (metrics.regular ? -110 : -60))
        }
    }

    private var playTarget: Item? {
        switch current.kind {
        case .movie, .episode: current
        default: detail?.playNext
        }
    }

    private var actions: some View {
        VStack(alignment: .leading, spacing: 10) {
            HStack(spacing: 12) {
                Button { if let t = playTarget { model.play(t) } } label: {
                    Label(playLabel, systemImage: "play.fill").frame(maxWidth: .infinity)
                }
                .buttonStyle(PrimaryButtonStyle())
                .disabled(playTarget == nil)

                if current.kind == .movie || current.kind == .episode {
                    DownloadButton(item: current)
                }
                IconToggle(on: userData.favorite, onIcon: "heart.fill", offIcon: "heart",
                           label: userData.favorite ? "Kedvenc – eltávolítás" : "Kedvencekhez") {
                    await setFavorite(!userData.favorite)
                }
                IconToggle(on: userData.played, onIcon: "checkmark.circle.fill", offIcon: "checkmark.circle",
                           label: userData.played ? "Megnézve – visszavonás" : "Megnézettnek jelölés") {
                    await setPlayed(!userData.played)
                }
            }
            if let t = playTarget, let p = t.progress {
                HStack(spacing: 10) {
                    ProgressView(value: p).tint(Theme.amber).frame(maxWidth: 120)
                    if let left = Format.runtime((t.runtimeSec ?? 0) - (t.userData?.position ?? 0)) {
                        Text("még \(left)").font(.caption).foregroundStyle(Theme.muted)
                    }
                    Spacer()
                    Button { model.play(t, from: 0) } label: {
                        Label("Lejátszás elölről", systemImage: "arrow.counterclockwise")
                            .padding(.vertical, 12)
                            .contentShape(.rect)
                    }
                    .font(.caption.weight(.semibold))
                    .foregroundStyle(Theme.plaster)
                }
            }
        }
        // On an iPad the play button and the toggles stay together, not stretched across the screen.
        .frame(maxWidth: metrics.regular ? 600 : nil, alignment: .leading)
        .padding(.horizontal, metrics.gutter)
    }

    private var playLabel: String {
        guard let t = playTarget else { return "Lejátszás" }
        if t.progress != nil { return "Folytatás" }
        if t.kind == .episode, current.kind != .episode, let l = t.episodeLabel { return "\(l) lejátszása" }
        return "Lejátszás"
    }

    @ViewBuilder private var overview: some View {
        if let o = current.overview, !o.isEmpty {
            // Short texts fit in four lines anyway: no "more" for them.
            let long = o.count > 240 || o.split(separator: "\n").count > 4
            Button {
                withAnimation { overviewExpanded.toggle() }
            } label: {
                VStack(alignment: .leading, spacing: 6) {
                    Text(o)
                        .foregroundStyle(Theme.plaster)
                        .lineLimit(overviewExpanded ? nil : 4)
                        .multilineTextAlignment(.leading)
                    if long {
                        Text(overviewExpanded ? "Kevesebb" : "Bővebben")
                            .font(.subheadline.weight(.semibold))
                            .foregroundStyle(Theme.amber)
                    }
                }
                .frame(maxWidth: .infinity, alignment: .leading)
                .contentShape(.rect)
            }
            .buttonStyle(.plain)
            .disabled(!long)
            .accessibilityRemoveTraits(long ? [] : .isButton)
            .frame(maxWidth: metrics.readableWidth, alignment: .leading)
            .padding(.horizontal, metrics.gutter)
        }
    }

    @ViewBuilder private var episodesSection: some View {
        let seasons = detail?.seasons ?? []
        VStack(alignment: .leading, spacing: 14) {
            if current.kind == .series {
                HStack {
                    Text("Évadok").font(Theme.display(metrics.sectionTitle, weight: .bold))
                    Spacer()
                    if seasons.count > 1 {
                        Picker("Évad", selection: $season) {
                            ForEach(seasons) { s in Text(s.title).tag(Optional(s.id)) }
                        }
                        .pickerStyle(.menu)
                    } else if let s = seasons.first {
                        Text(s.title).foregroundStyle(Theme.muted)
                    }
                }
            } else {
                Text("Részek").font(Theme.display(metrics.sectionTitle, weight: .bold))
            }

            if let season, let list = episodes[season] {
                if list.isEmpty {
                    Text("Ebben az évadban nincs rész.").foregroundStyle(Theme.muted)
                } else if !missing(list).isEmpty {
                    Button {
                        if let room = DownloadStore.shared.roomError() {
                            notice = Notice(title: "Évad letöltése", message: room)
                        } else {
                            askSeasonQuality = true
                        }
                    } label: {
                        Label("Évad letöltése (\(missing(list).count) rész)", systemImage: "arrow.down.circle")
                            .font(.subheadline.weight(.semibold))
                    }
                    .tint(Theme.amber)
                }
                // Two columns on an iPad, one on a phone.
                LazyVGrid(columns: episodeColumns, alignment: .leading, spacing: metrics.regular ? 22 : 14) {
                    ForEach(list) { ep in
                        NavigationLink(value: ep) { EpisodeRow(item: ep, isNext: ep.id == detail?.playNext?.id) }
                            .buttonStyle(.plain)
                            .contextMenu {
                                Button("Lejátszás", systemImage: "play.fill") { model.play(ep) }
                                if !missing([ep]).isEmpty {
                                    Button("Letöltés (720p)", systemImage: "arrow.down.circle") { download(ep) }
                                }
                            }
                    }
                }
            } else if let episodesError {
                VStack(alignment: .leading, spacing: 10) {
                    Text(episodesError).foregroundStyle(Theme.plaster)
                    Button("Újra") { Task { await loadEpisodes() } }.buttonStyle(.bordered)
                }
            } else if detail != nil, !(current.kind == .series && seasons.isEmpty) {
                ProgressView().frame(maxWidth: .infinity).padding()
            }
        }
        .padding(.horizontal, metrics.gutter)
    }

    private var episodeColumns: [GridItem] {
        Array(repeating: GridItem(.flexible(), spacing: 28, alignment: .top), count: metrics.regular ? 2 : 1)
    }

    private func similarSection(_ items: [Item]) -> some View {
        VStack(alignment: .leading, spacing: 12) {
            Text("Hasonlók").font(Theme.display(metrics.sectionTitle, weight: .bold)).padding(.horizontal, metrics.gutter)
            ScrollView(.horizontal, showsIndicators: false) {
                LazyHStack(alignment: .top, spacing: metrics.spacing) {
                    ForEach(items) { it in
                        NavigationLink(value: it) { PosterCard(item: it) }.buttonStyle(.plain)
                    }
                }
                .padding(.horizontal, metrics.gutter)
            }
        }
    }

    // MARK: - loading

    private func load() async {
        error = nil
        do {
            let d = try await model.api { [id = item.id] in try await $0.item(id) }
            detail = d
            Spotlight.index([d.item] + (d.similar ?? []))
            userData = d.item.userData ?? UserData()
            if current.kind == .season {
                season = d.item.id
            } else if season == nil, let seasons = d.seasons, !seasons.isEmpty {
                // Open the season the next episode is in.
                season = seasons.first { $0.id == d.playNext?.parentId }?.id ?? seasons.first?.id
            }
            // Refresh the open season in place; the others reload when picked.
            if let season, let list = try? await fetchEpisodes(season) { episodes = [season: list] }
        } catch is CancellationError {
        } catch {
            self.error = error.localizedDescription
        }
    }

    /// Episodes of the list that are neither saved nor downloading.
    private func missing(_ list: [Item]) -> [Item] {
        let store = DownloadStore.shared
        return list.filter { store.download(for: $0.id) == nil && store.phases[$0.id] == nil }
    }

    private func downloadSeason(_ quality: String) {
        guard let season, let list = episodes[season] else { return }
        let todo = missing(list)
        Task {
            do {
                let n = try await model.api { try await DownloadStore.shared.startAll(todo, quality: quality, client: $0) }
                if n < todo.count {
                    notice = Notice(title: "Évad letöltése",
                                    message: "\(todo.count) részből \(n) letöltése indult el. "
                                        + (DownloadStore.shared.roomError() ?? "A többit később próbáld újra."))
                }
            } catch {
                notice = Notice(title: "Évad letöltése", message: error.localizedDescription)
            }
        }
    }

    private func loadEpisodes() async {
        guard let season, episodes[season] == nil, fetchingSeason != season else { return }
        episodesError = nil
        do { episodes[season] = try await fetchEpisodes(season) }
        catch is CancellationError {}
        catch { if self.season == season { episodesError = error.localizedDescription } }
    }

    private func download(_ ep: Item) {
        if let room = DownloadStore.shared.roomError() {
            notice = Notice(title: "Nem sikerült a letöltés", message: room)
            return
        }
        Task {
            do { try await model.api { try await DownloadStore.shared.start(ep, quality: "720p", client: $0) } }
            catch { notice = Notice(title: "Nem sikerült a letöltés", message: error.localizedDescription) }
        }
    }

    private func fetchEpisodes(_ id: Int) async throws -> [Item] {
        fetchingSeason = id
        defer { if fetchingSeason == id { fetchingSeason = nil } }
        return try await model.api { try await $0.children(of: id) }
    }

    private func setFavorite(_ on: Bool) async {
        do { userData = try await model.api { [id = current.id] in try await $0.setFavorite(id, on) } }
        catch is CancellationError {}
        catch { notice = Notice(title: "Nem sikerült menteni", message: error.localizedDescription) }
    }

    private func setPlayed(_ on: Bool) async {
        let u: UserData
        do { u = try await model.api { [id = current.id] in try await $0.setPlayed(id, on) } }
        catch is CancellationError { return }
        catch {
            notice = Notice(title: "Nem sikerült menteni", message: error.localizedDescription)
            return
        }
        userData = u
        // Marking a series or season changes its episodes and the next one up.
        if current.kind == .series || current.kind == .season { await load() }
    }
}

private struct IconToggle: View {
    var on: Bool
    var onIcon: String
    var offIcon: String
    var label: String
    var action: () async -> Void
    @State private var busy = false

    var body: some View {
        Button {
            busy = true
            Task { await action(); busy = false }
        } label: {
            Image(systemName: on ? onIcon : offIcon)
                .font(.title3)
                .foregroundStyle(on ? Theme.amber : Theme.plaster)
                .frame(width: 50, height: 50)
                .background(Theme.panel, in: .rect(cornerRadius: 10))
        }
        .disabled(busy)
        .accessibilityLabel(label)
        .sensoryFeedback(.selection, trigger: on)
    }
}

private struct EpisodeRow: View {
    var item: Item
    var isNext: Bool
    @Environment(\.metrics) private var metrics

    var body: some View {
        HStack(alignment: .top, spacing: metrics.regular ? 16 : 12) {
            ArtworkView(item: item, kind: .thumb)
                .frame(width: metrics.regular ? 192 : 136, height: metrics.regular ? 108 : 76.5)
                .overlay(alignment: .bottom) { ProgressStripe(item: item) }
                .overlay(alignment: .topTrailing) { WatchedBadge(item: item) }
                .clipShape(.rect(cornerRadius: 6))
                .pointerLift(cornerRadius: 6)
            VStack(alignment: .leading, spacing: 3) {
                HStack(spacing: 6) {
                    if let n = item.episodeNum { Text("\(n).").foregroundStyle(Theme.muted) }
                    Text(item.title).fontWeight(.semibold).lineLimit(2)
                }
                .font(.subheadline)
                HStack(spacing: 6) {
                    if isNext { Text("Következő").foregroundStyle(Theme.amber).fontWeight(.semibold) }
                    if let r = Format.runtime(item.runtimeSec) { Text(r).foregroundStyle(Theme.muted) }
                }
                .font(.caption)
                if let o = item.overview, !o.isEmpty {
                    Text(o).font(.caption).foregroundStyle(Theme.muted).lineLimit(2)
                }
            }
            Spacer(minLength: 0)
        }
        .contentShape(.rect)
        .accessibilityElement(children: .combine)
    }
}
