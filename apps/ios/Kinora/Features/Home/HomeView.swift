import SwiftUI
import KinoraKit

/// Signed in with a profile: home, libraries and search tabs.
struct HomeView: View {
    @Environment(AppModel.self) private var model

    var body: some View {
        @Bindable var model = model
        TabView(selection: $model.tab) {
            NavigationStack(path: $model.homePath) { HomeTab().browseDestinations() }
                .tabItem { Label("Kezdőlap", systemImage: "house") }
                .tag(AppModel.Tab.home)
            NavigationStack { LibrariesView().browseDestinations() }
                .tabItem { Label("Könyvtárak", systemImage: "square.grid.2x2") }
                .tag(AppModel.Tab.libraries)
            NavigationStack { SearchView().browseDestinations() }
                .tabItem { Label("Keresés", systemImage: "magnifyingglass") }
                .tag(AppModel.Tab.search)
        }
        // A widget link that arrived while signing in.
        .onAppear { model.handlePendingLink() }
        // A new profile sees its own home: rebuild everything when it changes.
        .id(model.me?.profile?.id)
        .sheet(isPresented: $model.settingsVisible) { SettingsView() }
        .fullScreenCover(isPresented: Binding(get: { model.playerVisible && model.player != nil },
                                              set: { model.playerVisible = $0 })) {
            if let p = model.player { PlayerView(player: p) }
        }
    }
}

struct HomeTab: View {
    @Environment(AppModel.self) private var model
    @State private var data: HomeData?
    @State private var error: String?

    var body: some View {
        ScrollView {
            if let data {
                LazyVStack(alignment: .leading, spacing: 28) {
                    if let hero = data.hero { HeroCard(item: hero) }
                    ForEach(data.rows) { row in
                        if !row.items.isEmpty { Shelf(row: row) }
                    }
                    if data.hero == nil && data.rows.allSatisfy({ $0.items.isEmpty }) {
                        ContentUnavailableView("Még üres a könyvtár",
                                               systemImage: "film.stack",
                                               description: Text("Az adminisztrátor a webes felületen adhat hozzá könyvtárat."))
                            .padding(.top, 60)
                    }
                }
                .padding(.bottom, 24)
            } else {
                LoadingState(error: error) { Task { await load() } }
            }
        }
        .refreshable { await load() }
        .task { if data == nil { await load() } }
        .onChange(of: model.playbackGeneration) { Task { await load() } }
        .kinoraScreen()
        .navigationTitle("Kezdőlap")
        .toolbar(.hidden, for: .navigationBar)
        .safeAreaInset(edge: .top) { header }
    }

    private var header: some View {
        HStack {
            BrandMark(size: 28)
            Spacer()
            AccountMenu()
        }
        .padding(.horizontal, 20)
        .padding(.vertical, 8)
        .background(Theme.black.opacity(0.92))
    }

    private func load() async {
        error = nil
        do { data = try await model.api { try await $0.home() } }
        catch is CancellationError {}
        catch { if data == nil { self.error = error.localizedDescription } }
    }
}

/// The featured title at the top of home.
private struct HeroCard: View {
    var item: Item
    @Environment(\.verticalSizeClass) private var vSize

    var body: some View {
        NavigationLink(value: item) {
            ZStack(alignment: .bottomLeading) {
                ArtworkView(item: item, kind: .backdrop)
                    .heroFrame(16 / 10, compactHeight: vSize == .compact)
                LinearGradient(colors: [.clear, Theme.black.opacity(0.95)], startPoint: .center, endPoint: .bottom)
                VStack(alignment: .leading, spacing: 6) {
                    Text(item.title)
                        .font(Theme.display(34, weight: .bold))
                        .lineLimit(2)
                    Text(Format.meta(item)).font(.footnote).foregroundStyle(Theme.plaster)
                    if let o = item.overview, !o.isEmpty {
                        Text(o).font(.footnote).foregroundStyle(Theme.muted).lineLimit(2)
                    }
                }
                .padding(20)
            }
            .clipShape(.rect(cornerRadius: 14))
            .padding(.horizontal, 16)
        }
        .buttonStyle(.plain)
    }
}

/// A titled horizontal row of cards.
private struct Shelf: View {
    var row: HomeRow

    var body: some View {
        VStack(alignment: .leading, spacing: 12) {
            Text(row.title)
                .font(Theme.display(22, weight: .bold))
                .padding(.horizontal, 20)
                .accessibilityAddTraits(.isHeader)
            ScrollView(.horizontal, showsIndicators: false) {
                LazyHStack(alignment: .top, spacing: 14) {
                    ForEach(row.items) { item in
                        NavigationLink(value: item) {
                            if row.style == .landscape { WideCard(item: item) } else { PosterCard(item: item) }
                        }
                        .buttonStyle(.plain)
                        .contextMenu {
                            if let why = row.reasons?[String(item.id)] { Text(why) }
                        }
                    }
                }
                .padding(.horizontal, 20)
            }
        }
    }
}
