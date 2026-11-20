import SwiftUI
import KinoraKit

struct SearchView: View {
    @Environment(AppModel.self) private var model
    @State private var query = ""
    @State private var results: [Item]?
    @State private var error: String?

    @Environment(\.metrics) private var metrics

    var body: some View {
        ScrollView {
            if let results {
                if results.isEmpty {
                    ContentUnavailableView.search(text: query).padding(.top, 60)
                    askLink
                } else {
                    LazyVGrid(columns: metrics.posterGrid, spacing: metrics.regular ? 28 : 20) {
                        ForEach(results) { item in
                            NavigationLink(value: item) { PosterCard(item: item, fillsColumn: true) }
                                .buttonStyle(.plain)
                        }
                    }
                    .padding(.horizontal, metrics.regular ? metrics.gutter : 16)
                    .padding(.vertical, 16)
                    askLink.padding(.bottom, 24)
                }
            } else if let error {
                LoadingState(error: error) { Task { await search(query) } }
            } else {
                ContentUnavailableView(L("Filmek és sorozatok"), systemImage: "magnifyingglass",
                                       description: Text(L("Keress címre.")))
                    .padding(.top, 60)
            }
        }
        .scrollDismissesKeyboard(.immediately)
        .kinoraScreen()
        .navigationTitle(L("Keresés"))
        .searchable(text: $query, placement: .navigationBarDrawer(displayMode: .always), prompt: L("Cím"))
        .toolbar { ToolbarItem(placement: .topBarTrailing) { AccountMenu() } }
        // Typing restarts the task: a short pause before asking the server.
        .task(id: query) {
            let q = query.trimmingCharacters(in: .whitespaces)
            guard !q.isEmpty else { results = nil; error = nil; return }
            try? await Task.sleep(for: .milliseconds(300))
            guard !Task.isCancelled else { return }
            await search(q)
        }
    }

    /// Not in the library: the same title in Felfedezés, where it can be requested.
    @ViewBuilder private var askLink: some View {
        if model.canRequest {
            Button {
                model.discoverQuery = query.trimmingCharacters(in: .whitespaces)
                model.tab = .discover
            } label: {
                Label(L("Nincs meg? Kérd meg a Felfedezésben"), systemImage: "sparkles")
                    .font(.subheadline.weight(.semibold))
                    .frame(minHeight: 44)
                    .contentShape(.rect)
            }
            .foregroundStyle(Theme.amber)
            .frame(maxWidth: .infinity)
        }
    }

    private func search(_ q: String) async {
        do {
            let r = try await model.api { try await $0.search(q) }
            error = nil
            results = r
        } catch is CancellationError {
        } catch {
            results = nil
            self.error = error.localizedDescription
        }
    }
}
