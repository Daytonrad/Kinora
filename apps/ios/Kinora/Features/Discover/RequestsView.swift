import SwiftUI
import KinoraKit

/// Kéréseim: what the user asked for and where it stands. Refreshes itself while a request is open.
struct RequestsView: View {
    @Environment(AppModel.self) private var model
    /// Opens a library item (a request that arrived).
    var open: (Item) -> Void

    @State private var list: [AcqRequest]?
    @State private var error: String?
    @State private var selected: DiscoverItem?
    @State private var cancelling: AcqRequest?

    var body: some View {
        List {
            if let list {
                if list.isEmpty {
                    ContentUnavailableView(L("Még nem kértél filmet."), systemImage: "tray",
                                           description: Text(L("A Felfedezésben kereshetsz rá egy címre, és kérheted.")))
                        .listRowBackground(Color.clear)
                } else {
                    Section {
                        ForEach(list) { r in
                            Button { selected = discoverItem(r) } label: { row(r) }
                                .buttonStyle(.plain)
                                .listRowBackground(Theme.lacquer)
                                .swipeActions {
                                    if r.cancellable {
                                        Button(L("Visszavonás"), role: .destructive) { cancelling = r }.tint(Theme.danger)
                                    } else if r.removable {
                                        Button(L("Eltávolítás"), role: .destructive) { Task { await cancel(r) } }.tint(Theme.danger)
                                    }
                                }
                                .contextMenu {
                                    if r.removable {
                                        Button(L("Eltávolítás"), systemImage: "trash", role: .destructive) { Task { await cancel(r) } }
                                    }
                                }
                        }
                    } footer: {
                        Text(L("A filmek, amiket kértél, és hogy hol tartanak.")).foregroundStyle(Theme.muted)
                    }
                }
            } else {
                LoadingState(error: error) { Task { await load() } }
                    .listRowBackground(Color.clear)
            }
        }
        .scrollContentBackground(.hidden)
        .kinoraScreen()
        .navigationTitle(L("Kéréseim"))
        .refreshable { await load() }
        .task {
            // While something is on its way the list follows it, like the web page.
            while !Task.isCancelled {
                await load()
                guard list?.contains(where: \.isOpen) == true else { break }
                try? await Task.sleep(for: .seconds(10))
            }
        }
        .sheet(item: $selected) { it in
            DiscoverDetailSheet(item: it, open: { lib in selected = nil; open(lib) }, changed: { Task { await load() } })
        }
        .confirmationDialog(L("Visszavonod a kérést?"), isPresented: Binding(get: { cancelling != nil }, set: { if !$0 { cancelling = nil } }),
                            titleVisibility: .visible, presenting: cancelling) { r in
            Button(L("Visszavonás"), role: .destructive) { Task { await cancel(r) } }
        }
    }

    private func row(_ r: AcqRequest) -> some View {
        HStack(spacing: 14) {
            RemotePoster(url: r.poster, title: r.title)
                .frame(width: 54)
                .clipShape(.rect(cornerRadius: 6))
            VStack(alignment: .leading, spacing: 4) {
                Text(r.title + (r.year.map { " (\($0))" } ?? ""))
                    .font(.headline).foregroundStyle(Theme.plasterBright).lineLimit(2)
                HStack(spacing: 6) {
                    Text(r.status.label)
                        .font(.caption.weight(.bold))
                        .padding(.horizontal, 7).padding(.vertical, 3)
                        .foregroundStyle(r.status == .available ? Theme.amberInk : Theme.plasterBright)
                        .background(r.status == .available ? Theme.amber : (r.status == .denied || r.status == .failed ? Theme.danger.opacity(0.85) : Theme.raised),
                                    in: .capsule)
                    if r.isSeries { Text(seasonsLabel(r.seasons)).font(.caption).foregroundStyle(Theme.muted).lineLimit(1) }
                }
                if r.status == .downloading, let p = r.progress, p > 0 {
                    ProgressView(value: p).tint(Theme.amber)
                }
                if let n = r.note, !n.isEmpty {
                    Text(Language.shared.server(n)).font(.caption).foregroundStyle(Theme.muted).lineLimit(2)
                }
                if let lib = r.libraryName, !lib.isEmpty {
                    Text(lib).font(.caption2).foregroundStyle(Theme.muted)
                }
            }
            Spacer(minLength: 0)
        }
        .padding(.vertical, 4)
        .contentShape(.rect)
        .accessibilityElement(children: .combine)
    }

    private func discoverItem(_ r: AcqRequest) -> DiscoverItem {
        var it = DiscoverItem(id: r.tmdbId, title: r.title, kind: r.isSeries ? "series" : "movie")
        it.year = r.year
        it.poster = r.poster
        return it
    }

    private func load() async {
        do {
            // a withdrawn request has nothing left to show
            list = try await model.api { try await $0.myRequests() }.filter { $0.status != .canceled }
            error = nil
        } catch is CancellationError {
        } catch {
            if list == nil { self.error = error.localizedDescription }
        }
    }

    private func cancel(_ r: AcqRequest) async {
        do {
            try await model.api { try await $0.cancelRequest(r.id) }
            Haptic.warning.play()
        } catch {
            self.error = error.localizedDescription
        }
        await load()
    }
}
