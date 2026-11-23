import SwiftUI
import KinoraKit

/// Titles saved on the device and the downloads still in progress.
struct DownloadsView: View {
    @Environment(AppModel.self) private var model
    private var store: DownloadStore { .shared }

    var body: some View {
        Group {
            if store.visible.isEmpty && store.visiblePhases.isEmpty {
                ContentUnavailableView(L("Nincs letöltés"), systemImage: "arrow.down.circle",
                                       description: Text(L("Filmet vagy részt a címlapján a letöltés gombbal menthetsz offline nézéshez.")))
            } else {
                List {
                    if !store.visiblePhases.isEmpty {
                        Section(L("Folyamatban")) {
                            ForEach(store.visiblePhases.keys.sorted(), id: \.self) { id in
                                if let phase = store.phases[id] {
                                    ActiveRow(title: store.pendingTitle(id), phase: phase) { Haptic.warning.play(); store.cancel(id) }
                                }
                            }
                        }
                        .listRowBackground(Theme.panel)
                    }
                    if !store.visible.isEmpty {
                        Section {
                            ForEach(store.visible) { d in
                                Button { model.play(d.item) } label: { SavedRow(download: d) }
                                    .buttonStyle(.plain)
                                    .swipeActions {
                                        Button(L("Törlés"), systemImage: "trash", role: .destructive) { Haptic.warning.play(); store.delete(d.id) }
                                    }
                                    .contextMenu {
                                        Button(L("Lejátszás"), systemImage: "play.fill") { model.play(d.item) }
                                        Button(L("Törlés az eszközről"), systemImage: "trash", role: .destructive) { Haptic.warning.play(); store.delete(d.id) }
                                    }
                            }
                        } header: {
                            Text(L("Az eszközön"))
                        } footer: {
                            Text(L("{0}. Törléshez húzd balra a sort.", usage))
                        }
                        .listRowBackground(Theme.panel)
                    }
                }
                .scrollContentBackground(.hidden)
            }
        }
        .kinoraScreen()
        .navigationTitle(L("Letöltések"))
        .toolbar { ToolbarItem(placement: .topBarTrailing) { AccountMenu() } }
    }

    private var usage: String {
        let used = ByteCountFormatter.string(fromByteCount: store.usedBytes, countStyle: .file)
        let limit = AppSettings.downloadLimitGB
        return limit > 0 ? "\(used) / \(limit) GB" : L("Összesen {0}", used)
    }
}

private struct SavedRow: View {
    var download: SavedDownload

    var body: some View {
        HStack(spacing: 12) {
            ArtworkView(item: download.item, kind: download.item.kind == .episode ? .thumb : .backdrop)
                .frame(width: 112, height: 63)
                .clipShape(.rect(cornerRadius: 6))
            VStack(alignment: .leading, spacing: 3) {
                if let s = download.item.seriesTitle { Text(s).font(.caption).foregroundStyle(Theme.muted) }
                Text(download.item.title).font(.subheadline.weight(.semibold)).lineLimit(2)
                Text([download.item.episodeLabel, download.quality,
                      ByteCountFormatter.string(fromByteCount: download.size, countStyle: .file)]
                        .compactMap { $0 }.joined(separator: " · "))
                    .font(.caption).foregroundStyle(Theme.muted)
            }
            Spacer(minLength: 0)
            Image(systemName: "play.circle.fill").font(.title2).foregroundStyle(Theme.amber)
        }
        .padding(.vertical, 4)
        .contentShape(.rect)
        .accessibilityElement(children: .combine)
        .accessibilityHint(L("Lejátszás az eszközről"))
    }
}

private struct ActiveRow: View {
    var title: String
    var phase: DownloadStore.Phase
    var cancel: () -> Void

    private var failed: Bool {
        if case .failed = phase { return true }
        return false
    }

    var body: some View {
        HStack(spacing: 12) {
            VStack(alignment: .leading, spacing: 6) {
                Text(title).font(.subheadline.weight(.semibold)).lineLimit(1)
                switch phase {
                case .preparing(let p):
                    ProgressView(value: p).tint(Theme.muted)
                    Text(L("Előkészítés a szerveren · {0}%", Int(p * 100))).font(.caption).foregroundStyle(Theme.muted)
                case .transferring(let p):
                    ProgressView(value: p).tint(Theme.amber)
                    Text(p == 0 && AppSettings.downloadsWifiOnly && NetworkStatus.shared.isExpensive
                         ? L("Wi-Fi-re vár") : L("Letöltés · {0}%", Int(p * 100)))
                        .font(.caption).foregroundStyle(Theme.muted)
                case .interrupted(let p):
                    ProgressView(value: p).tint(Theme.muted)
                    Text(L("Megszakadt · {0}% · folytatódik, ha lesz hálózat", Int(p * 100)))
                        .font(.caption).foregroundStyle(Theme.muted)
                case .failed(let m):
                    Text(m).font(.caption).foregroundStyle(Theme.danger)
                }
            }
            Button(action: cancel) {
                Image(systemName: "xmark.circle.fill").font(.title3).foregroundStyle(Theme.muted)
            }
            .buttonStyle(.plain)
            .frame(minWidth: 44, minHeight: 44)
            .contentShape(.rect)
            .accessibilityLabel(failed ? L("Eltávolítás a listából") : L("Letöltés megszakítása"))
        }
        .padding(.vertical, 4)
    }
}

/// Download button of a movie or episode: pick a quality, then follow the progress.
struct DownloadButton: View {
    @Environment(AppModel.self) private var model
    var item: Item
    @State private var error: String?
    @State private var asking = false
    private var store: DownloadStore { .shared }

    static let qualities: [(String, String)] = [
        ("1080p", "1080p (~6 Mbps)"), ("720p", "720p (~3 Mbps)"), ("480p", "480p (~1,2 Mbps)"), ("360p", "360p (~600 kbps)"),
    ]

    var body: some View {
        Button {
            if store.download(for: item.id) == nil, store.phases[item.id] == nil {
                if let room = store.roomError() { error = room } else { Haptic.tap.play(); asking = true }
            }
            else { model.tab = .downloads }
        } label: {
            icon
                .font(.title3)
                .frame(width: 50, height: 50)
                .background(Theme.panel, in: .rect(cornerRadius: 10))
        }
        .accessibilityLabel(label)
        // Done in the background while this page is open: a short "ready".
        .sensoryFeedback(.success, trigger: store.download(for: item.id) != nil) { _, done in done }
        .sensoryFeedback(.error, trigger: error) { _, e in e != nil }
        .confirmationDialog(L("Letöltés offline nézéshez"), isPresented: $asking, titleVisibility: .visible) {
            ForEach(Self.qualities, id: \.0) { q in
                Button(q.1) { start(q.0) }
            }
        } message: {
            Text(L("A szerver előbb átalakítja a fájlt, utána a letöltés a háttérben is folytatódik."))
        }
        .alert(L("Nem sikerült a letöltés"), isPresented: Binding(get: { error != nil }, set: { if !$0 { error = nil } })) {
            Button("OK", role: .cancel) {}
        } message: {
            Text(error ?? "")
        }
    }

    @ViewBuilder private var icon: some View {
        if store.download(for: item.id) != nil {
            Image(systemName: "arrow.down.circle.fill").foregroundStyle(Theme.amber)
        } else if let phase = store.phases[item.id] {
            switch phase {
            case .preparing(let p), .transferring(let p), .interrupted(let p):
                ProgressView(value: max(p, 0.02)).progressViewStyle(.circular).tint(Theme.amber)
            case .failed:
                Image(systemName: "exclamationmark.circle").foregroundStyle(Theme.danger)
            }
        } else {
            Image(systemName: "arrow.down.circle").foregroundStyle(Theme.plaster)
        }
    }

    private var label: String {
        if store.download(for: item.id) != nil { return L("Letöltve") }
        if store.phases[item.id] != nil { return L("Letöltés folyamatban") }
        return L("Letöltés")
    }

    private func start(_ quality: String) {
        Haptic.firm.play()
        Task {
            do { try await model.api { try await DownloadStore.shared.start(item, quality: quality, client: $0) } }
            catch { self.error = error.localizedDescription }
        }
    }
}
