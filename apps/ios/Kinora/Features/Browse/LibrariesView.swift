import SwiftUI
import KinoraKit

struct LibrariesView: View {
    @Environment(AppModel.self) private var model
    @State private var libraries: [Library]?
    @State private var error: String?

    var body: some View {
        Group {
            if let libraries {
                if libraries.isEmpty {
                    ContentUnavailableView(L("Nincs könyvtár"), systemImage: "square.grid.2x2",
                                           description: Text(L("Az adminisztrátor a webes felületen adhat hozzá könyvtárat.")))
                } else {
                    List(libraries) { lib in
                        NavigationLink(value: lib) {
                            Label {
                                VStack(alignment: .leading, spacing: 2) {
                                    Text(lib.name).font(.headline)
                                    if let n = lib.count { Text(L("{0} cím", n)).font(.caption).foregroundStyle(Theme.muted) }
                                }
                            } icon: {
                                Image(systemName: icon(lib.kind)).foregroundStyle(Theme.amber)
                            }
                            .padding(.vertical, 6)
                        }
                        .listRowBackground(Theme.panel)
                    }
                    .scrollContentBackground(.hidden)
                }
            } else {
                LoadingState(error: error) { Task { await load() } }
            }
        }
        .refreshable { await load() }
        .task { if libraries == nil { await load() } }
        .kinoraScreen()
        .navigationTitle(L("Könyvtárak"))
        .toolbar { ToolbarItem(placement: .topBarTrailing) { AccountMenu() } }
    }

    private func icon(_ kind: String) -> String {
        switch kind {
        case "movies": "film"
        case "shows": "tv"
        default: "rectangle.stack"
        }
    }

    private func load() async {
        error = nil
        do { libraries = try await model.api { try await $0.libraries() } }
        catch is CancellationError {}
        catch { if libraries == nil { self.error = error.localizedDescription } }
    }
}

/// One library as a poster grid, loaded 60 at a time.
struct LibraryView: View {
    @Environment(AppModel.self) private var model
    var library: Library
    @AppStorage("kinora.librarySort") private var sort: ItemSort = .title
    @State private var items: [Item] = []
    @State private var total = 0
    @State private var loaded = false
    @State private var loadingMore = false
    @State private var error: String?

    @Environment(\.metrics) private var metrics

    var body: some View {
        ScrollView {
            if loaded {
                LazyVGrid(columns: metrics.posterGrid, spacing: metrics.regular ? 28 : 20) {
                    ForEach(items) { item in
                        NavigationLink(value: item) { PosterCard(item: item, fillsColumn: true) }
                            .buttonStyle(.plain)
                            .onAppear { if item.id == items.last?.id { Task { await loadMore() } } }
                    }
                }
                .padding(.horizontal, metrics.regular ? metrics.gutter : 16)
                .padding(.vertical, 16)
                if loadingMore { ProgressView().padding() }
                if items.isEmpty {
                    ContentUnavailableView(L("Üres könyvtár"), systemImage: "film.stack").padding(.top, 60)
                }
            } else {
                LoadingState(error: error) { Task { await reload() } }
            }
        }
        .refreshable { await reload() }
        .task(id: sort) { await reload() }
        .kinoraScreen()
        .navigationTitle(library.name)
        .toolbar {
            ToolbarItem(placement: .topBarTrailing) {
                Menu {
                    Picker(L("Rendezés"), selection: $sort) {
                        ForEach(ItemSort.allCases) { Text($0.label).tag($0) }
                    }
                } label: {
                    Label(L("Rendezés"), systemImage: "arrow.up.arrow.down")
                }
            }
        }
    }

    private func reload() async {
        error = nil
        do {
            let page = try await model.api { [library, sort] in try await $0.items(library: library.id, sort: sort) }
            items = page.items
            total = page.total
            loaded = true
        } catch is CancellationError {
        } catch {
            if !loaded { self.error = error.localizedDescription }
        }
    }

    private func loadMore() async {
        guard !loadingMore, items.count < total else { return }
        loadingMore = true
        defer { loadingMore = false }
        let offset = items.count
        if let page = try? await model.api({ [library, sort] in try await $0.items(library: library.id, sort: sort, offset: offset) }) {
            let known = Set(items.map(\.id))
            items += page.items.filter { !known.contains($0.id) }
            total = page.total
        }
    }
}
