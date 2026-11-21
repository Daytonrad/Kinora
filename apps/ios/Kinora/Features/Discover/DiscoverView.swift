import SwiftUI
import KinoraKit

// Felfedezés: search TMDB or browse its lists, and ask for a film or series the server does not have
// yet. Shown only when the server allows requests from this profile (`Me.canRequest`). iPhone and
// iPad only; the Apple TV app leaves it out.

enum DiscoverRoute: Hashable { case requests }

/// The Felfedezés tab with its own stack, so a title the library has can be opened from the sheet.
struct DiscoverTab: View {
    @State private var path = NavigationPath()

    var body: some View {
        NavigationStack(path: $path) {
            DiscoverView { path.append($0) }
                .browseDestinations()
                .navigationDestination(for: DiscoverRoute.self) { _ in RequestsView { path.append($0) } }
        }
    }
}

struct DiscoverView: View {
    enum Kind: Hashable { case movies, series }

    @Environment(AppModel.self) private var model
    @Environment(\.metrics) private var metrics
    /// Opens a library item (already in the library).
    var open: (Item) -> Void

    @State private var kind = Kind.movies
    @State private var query = ""
    @State private var shelves: [Kind: [(DiscoverList, [DiscoverItem])]] = [:]
    /// What the four big streaming services have, per kind (after the first list, like the web page).
    @State private var services: [Kind: [(WatchProvider, [DiscoverItem])]] = [:]
    @State private var options: DiscoverOptions?
    @State private var shelfError: String?
    @State private var results: (movies: [DiscoverItem], series: [DiscoverItem])?
    @State private var searchError: String?
    @State private var selected: DiscoverItem?

    private var searching: Bool { !query.trimmingCharacters(in: .whitespaces).isEmpty }

    var body: some View {
        ScrollView {
            if searching { searchBody } else { browseBody }
        }
        .scrollDismissesKeyboard(.immediately)
        .kinoraScreen()
        .navigationTitle(L("Felfedezés"))
        .searchable(text: $query, placement: .navigationBarDrawer(displayMode: .always), prompt: L("Film vagy sorozat címe…"))
        .toolbar {
            ToolbarItem(placement: .topBarLeading) {
                NavigationLink(value: DiscoverRoute.requests) {
                    Label(L("Kéréseim"), systemImage: "tray.full")
                }
            }
            ToolbarItem(placement: .topBarTrailing) { AccountMenu() }
        }
        .onAppear(perform: takeHandedQuery)
        .onChange(of: model.discoverQuery) { takeHandedQuery() }
        .task(id: kind) { if shelves[kind] == nil { await loadShelves() } }
        .task(id: query) {
            let q = query.trimmingCharacters(in: .whitespaces)
            guard !q.isEmpty else { results = nil; searchError = nil; return }
            try? await Task.sleep(for: .milliseconds(350))
            guard !Task.isCancelled else { return }
            await search(q)
        }
        .refreshable {
            if searching { await search(query.trimmingCharacters(in: .whitespaces)) } else { await loadShelves() }
        }
        .sheet(item: $selected) { item in
            DiscoverDetailSheet(item: item, open: { lib in selected = nil; open(lib) }, changed: refresh)
        }
    }

    // MARK: - browsing

    @ViewBuilder private var browseBody: some View {
        VStack(alignment: .leading, spacing: 24) {
            Picker(L("Filmek vagy sorozatok"), selection: $kind) {
                Text(L("Filmek")).tag(Kind.movies)
                Text(L("Sorozatok")).tag(Kind.series)
            }
            .pickerStyle(.segmented)
            .padding(.horizontal, metrics.gutter)
            .onChange(of: kind) { Haptic.select.play() }

            if let rows = shelves[kind] {
                if rows.allSatisfy({ $0.1.isEmpty }) {
                    ContentUnavailableView(L("Felfedezés"), systemImage: "sparkles",
                                           description: Text(L("Most nem érhetők el a listák. Próbálj rákeresni egy címre.")))
                        .padding(.top, 40)
                } else {
                    Text(L("Keress rá egy címre vagy böngéssz, és kérd meg: amint jóváhagyják, a szerver letölti és beteszi a könyvtárba."))
                        .font(.callout).foregroundStyle(Theme.muted)
                        .fixedSize(horizontal: false, vertical: true)
                        .padding(.horizontal, metrics.gutter)
                    ForEach(Array(rows.enumerated()), id: \.element.0) { i, row in
                        if !row.1.isEmpty { shelf(row.0.title, row.1) }
                        if i == 0 {
                            ForEach(services[kind] ?? [], id: \.0.id) { p, items in
                                if !items.isEmpty { shelf(L("{0} – most népszerű", BigProvider.label(for: p)), items, logo: p.logo) }
                            }
                        }
                    }
                }
            } else {
                LoadingState(error: shelfError) { Task { await loadShelves() } }
            }
        }
        .padding(.vertical, 12)
    }

    private func shelf(_ title: String, _ items: [DiscoverItem], logo: String? = nil) -> some View {
        VStack(alignment: .leading, spacing: 12) {
            HStack(spacing: 10) {
                if let logo, let url = URL(string: logo) {
                    AsyncImage(url: url) { $0.resizable().scaledToFit() } placeholder: { Theme.raised }
                        .frame(width: 30, height: 30)
                        .clipShape(.rect(cornerRadius: 6))
                        .accessibilityHidden(true)
                }
                Text(title)
                    .font(Theme.display(metrics.sectionTitle))
                    .accessibilityAddTraits(.isHeader)
            }
            .padding(.horizontal, metrics.gutter)
            ScrollView(.horizontal, showsIndicators: false) {
                LazyHStack(alignment: .top, spacing: metrics.spacing) {
                    ForEach(items, id: \.key) { it in
                        Button { selected = it } label: { DiscoverCard(item: it) }
                            .buttonStyle(.plain)
                            .frame(width: metrics.poster)
                    }
                }
                .padding(.horizontal, metrics.gutter)
            }
        }
    }

    private func loadShelves() async {
        let k = kind
        let lists = k == .movies ? DiscoverList.movies : DiscoverList.series
        do {
            // The first list's error (acquisition switched off, no metadata key) is shown; another
            // list that fails stays empty.
            let first = try await model.api { try await $0.discover(lists[0]) }
            var rows: [(DiscoverList, [DiscoverItem])] = [(lists[0], first)]
            if let client = model.client {
                let rest = await withTaskGroup(of: (Int, [DiscoverItem]).self) { group in
                    for (i, l) in lists.enumerated().dropFirst() {
                        group.addTask { (i, (try? await client.discover(l)) ?? []) }
                    }
                    var got: [Int: [DiscoverItem]] = [:]
                    for await (i, items) in group { got[i] = items }
                    return got
                }
                for i in 1..<lists.count { rows.append((lists[i], rest[i] ?? [])) }
            }
            shelfError = nil
            shelves[k] = rows
            await loadServices(k)
        } catch is CancellationError {
        } catch {
            shelfError = error.localizedDescription
        }
    }

    /// The four big services' shelves; a service the region lacks, or one that fails, is left out.
    private func loadServices(_ k: Kind) async {
        guard let client = model.client else { return }
        if options == nil { options = try? await client.discoverOptions() }
        let provs = options?.bigProviders ?? []
        guard !provs.isEmpty else { services[k] = []; return }
        let series = k == .series
        let got = await withTaskGroup(of: (Int, [DiscoverItem]).self) { group in
            for (i, p) in provs.enumerated() {
                group.addTask { (i, (try? await client.discoverBrowse(provider: p.id, series: series).results) ?? []) }
            }
            var out: [Int: [DiscoverItem]] = [:]
            for await (i, items) in group { out[i] = items }
            return out
        }
        services[k] = provs.enumerated().map { ($0.element, got[$0.offset] ?? []) }
    }

    // MARK: - search

    @ViewBuilder private var searchBody: some View {
        if let results {
            if results.movies.isEmpty && results.series.isEmpty {
                ContentUnavailableView.search(text: query).padding(.top, 60)
            } else {
                VStack(alignment: .leading, spacing: 28) {
                    if !results.movies.isEmpty { grid(L("Filmek"), results.movies) }
                    if !results.series.isEmpty { grid(L("Sorozatok"), results.series) }
                }
                .padding(.vertical, 16)
            }
        } else if let searchError {
            LoadingState(error: searchError) { Task { await search(query) } }
        } else {
            ProgressView().controlSize(.large).padding(.top, 80).frame(maxWidth: .infinity)
        }
    }

    private func grid(_ title: String, _ items: [DiscoverItem]) -> some View {
        VStack(alignment: .leading, spacing: 12) {
            Text(title).font(Theme.display(metrics.sectionTitle)).accessibilityAddTraits(.isHeader)
            LazyVGrid(columns: metrics.posterGrid, spacing: metrics.regular ? 28 : 20) {
                ForEach(items, id: \.key) { it in
                    Button { selected = it } label: { DiscoverCard(item: it) }.buttonStyle(.plain)
                }
            }
        }
        .padding(.horizontal, metrics.regular ? metrics.gutter : 16)
    }

    private func search(_ q: String) async {
        guard !q.isEmpty else { return }
        do {
            let r = try await model.api { client in
                async let mv = client.discoverSearch(q, series: false)
                async let tv = client.discoverSearch(q, series: true)
                return try await (movies: mv, series: tv)
            }
            searchError = nil
            results = r
        } catch is CancellationError {
        } catch {
            results = nil
            searchError = error.localizedDescription
        }
    }

    /// A title the library search did not find arrives in the search field.
    private func takeHandedQuery() {
        guard !model.discoverQuery.isEmpty else { return }
        query = model.discoverQuery
        model.discoverQuery = ""
    }

    /// After a request or a cancel: the cards show the new state.
    private func refresh() {
        Task {
            if searching { await search(query.trimmingCharacters(in: .whitespaces)) }
            shelves[kind] = nil
            await loadShelves()
        }
    }
}

// MARK: - card

/// A TMDB poster with the title and a badge for what the server knows about it.
struct DiscoverCard: View {
    var item: DiscoverItem

    var body: some View {
        VStack(alignment: .leading, spacing: 6) {
            RemotePoster(url: item.poster, title: item.title)
                .overlay(alignment: .topLeading) { badge.padding(6) }
                .overlay(alignment: .bottomTrailing) { seenMark.padding(6) }
                .clipShape(.rect(cornerRadius: 8))
                .pointerLift(cornerRadius: 8)
            Text(item.title).font(.subheadline.weight(.semibold)).lineLimit(1)
            Text(item.year.map(String.init) ?? " ")
                .font(.caption).foregroundStyle(Theme.muted).lineLimit(1)
        }
        .accessibilityElement(children: .combine)
        .accessibilityValue([badgeText, seenText].compactMap { $0 }.joined(separator: ", "))
    }

    private var seenText: String? {
        guard let w = item.watched else { return nil }
        return item.isSeries ? L("{0} részt láttál", w.episodes ?? 0) : L("Láttad")
    }

    /// Seen by this profile: an amber tick (series: with the episodes watched).
    @ViewBuilder private var seenMark: some View {
        if let w = item.watched {
            HStack(spacing: 2) {
                Image(systemName: "checkmark").font(.system(size: 10, weight: .heavy))
                if item.isSeries, let n = w.episodes, n > 0 {
                    Text("\(n)").font(.caption2.weight(.bold).monospacedDigit())
                }
            }
            .foregroundStyle(Theme.amberInk)
            .padding(.horizontal, item.isSeries ? 7 : 0)
            .frame(minWidth: 22, minHeight: 22)
            .background(Theme.amber, in: .capsule)
            .shadow(color: .black.opacity(0.45), radius: 3, y: 1)
            .accessibilityHidden(true)
        }
    }

    private var badgeText: String? {
        if let s = AcqRequest.Status(state: item.request) { return s.label }
        if item.itemId != nil { return item.isSeries ? L("Van belőle") : L("Könyvtárban") }
        if item.followed == true { return L("Figyelve") }
        return nil
    }

    @ViewBuilder private var badge: some View {
        if let t = badgeText {
            let st = AcqRequest.Status(state: item.request)
            let warn = st == .denied || st == .failed
            Text(t)
                .font(.caption2.weight(.bold))
                .lineLimit(1)
                .padding(.horizontal, 7).padding(.vertical, 3)
                .foregroundStyle(item.itemId != nil && item.request == nil ? Theme.amberInk : Theme.plasterBright)
                .background(item.itemId != nil && item.request == nil ? Theme.amber : (warn ? Theme.danger.opacity(0.9) : Theme.black.opacity(0.78)),
                            in: .capsule)
        }
    }
}

/// A 2:3 poster from a public URL (TMDB), with the title on a placeholder while loading or missing.
struct RemotePoster: View {
    var url: String?
    var title: String

    var body: some View {
        Rectangle()
            .fill(LinearGradient(colors: [Theme.raised, Theme.panel], startPoint: .topLeading, endPoint: .bottomTrailing))
            .aspectRatio(2 / 3, contentMode: .fit)
            .overlay {
                AsyncImage(url: url.flatMap(URL.init(string:)), transaction: Transaction(animation: .easeOut(duration: 0.2))) { phase in
                    if let img = phase.image {
                        img.resizable().scaledToFill()
                    } else {
                        Text(title)
                            .font(Theme.display(18, weight: .bold))
                            .foregroundStyle(Theme.plaster.opacity(0.85))
                            .multilineTextAlignment(.center)
                            .lineLimit(3)
                            .minimumScaleFactor(0.6)
                            .padding(10)
                    }
                }
            }
            .clipped()
            .accessibilityHidden(true)
    }
}
