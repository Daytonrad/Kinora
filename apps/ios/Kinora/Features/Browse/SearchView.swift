import SwiftUI
import KinoraKit

struct SearchView: View {
    @Environment(AppModel.self) private var model
    @State private var query = ""
    @State private var results: [Item]?
    @State private var error: String?

    private let columns = [GridItem(.adaptive(minimum: 104, maximum: 150), spacing: 14, alignment: .top)]

    var body: some View {
        ScrollView {
            if let results {
                if results.isEmpty {
                    ContentUnavailableView.search(text: query).padding(.top, 60)
                } else {
                    LazyVGrid(columns: columns, spacing: 20) {
                        ForEach(results) { item in
                            NavigationLink(value: item) { PosterCard(item: item, width: nil) }
                                .buttonStyle(.plain)
                        }
                    }
                    .padding(16)
                }
            } else if let error {
                LoadingState(error: error) { Task { await search(query) } }
            } else {
                ContentUnavailableView("Filmek és sorozatok", systemImage: "magnifyingglass",
                                       description: Text("Keress címre."))
                    .padding(.top, 60)
            }
        }
        .scrollDismissesKeyboard(.immediately)
        .kinoraScreen()
        .navigationTitle("Keresés")
        .searchable(text: $query, placement: .navigationBarDrawer(displayMode: .always), prompt: "Cím")
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
