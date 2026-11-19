import SwiftUI
import KinoraKit

/// Subtitle search on OpenSubtitles through the server, sized for the remote: language pills
/// on top, one row per result; pressing a row downloads it.
struct TVSubtitleSearchView: View {
    @Environment(AppModel.self) private var model
    @Environment(\.dismiss) private var dismiss
    var item: Item
    /// Gets the new track after a download (the player switches to it and the sheet closes).
    var onDownloaded: ((MediaStream) -> Void)?

    @State private var langs: [String] = []
    @State private var results: [SubtitleResult] = []
    @State private var enabled = true
    @State private var loading = true
    @State private var error: String?
    @State private var busy: Int?
    @State private var notice: String?

    private static let common = ["hun", "eng", "ger", "fre", "spa", "ita", "por", "pol", "cze", "rum", "rus", "dut"]

    var body: some View {
        ScrollView {
            VStack(alignment: .leading, spacing: 36) {
                Text(L("Felirat keresése")).font(Theme.display(64))
                Text(item.kind == .episode ? [item.seriesTitle, item.episodeLabel].compactMap { $0 }.joined(separator: " · ") : item.title)
                    .font(.title3).foregroundStyle(Theme.plaster)
                if enabled {
                    TVHeading(text: L("Keresett nyelvek"))
                    TVButtonRow {
                        ForEach(langChoices, id: \.self) { code in
                            let on = langs.contains(code)
                            Button { toggle(code) } label: {
                                if on { Label(langName(code), systemImage: "checkmark") } else { Text(langName(code)) }
                            }
                            .buttonStyle(TVPillStyle(prominent: on))
                        }
                    }
                    .focusSection()
                }
                content
            }
            .padding(80)
        }
        .background(Theme.panel.ignoresSafeArea())
        .task { await search() }
        .alert(L("Felirat"), isPresented: Binding(get: { notice != nil }, set: { if !$0 { notice = nil } })) {
            Button("OK", role: .cancel) {}
        } message: {
            Text(notice ?? "")
        }
    }

    @ViewBuilder private var content: some View {
        if loading && results.isEmpty {
            HStack(spacing: 20) {
                ProgressView()
                Text(L("Keresés az OpenSubtitles-ön…")).foregroundStyle(Theme.muted)
            }
        } else if let error {
            TVErrorText(message: error)
        } else if !enabled {
            Text(model.me?.user.isAdmin == true
                 ? L("A feliratkereséshez adj meg egy OpenSubtitles API-kulcsot: Admin → Metaadatok.")
                 : L("A feliratkeresés nincs bekapcsolva ezen a szerveren. Szólj az adminnak."))
                .font(.callout).foregroundStyle(Theme.plaster)
        } else if results.isEmpty {
            Text(L("Ezeken a nyelveken nincs találat.")).font(.callout).foregroundStyle(Theme.muted)
        } else {
            LazyVStack(alignment: .leading, spacing: 18) {
                ForEach(results) { r in row(r) }
            }
            .focusSection()
            Text(L("Feliratok: OpenSubtitles.com")).font(.caption).foregroundStyle(Theme.muted)
        }
    }

    private func row(_ r: SubtitleResult) -> some View {
        Button { Task { await download(r) } } label: {
            HStack(alignment: .center, spacing: 28) {
                Text((r.lang ?? "?").uppercased())
                    .font(.headline.monospaced())
                    .frame(width: 90)
                VStack(alignment: .leading, spacing: 6) {
                    Text(r.name).font(.headline).lineLimit(2)
                    let flags = [
                        r.hashMatch == true ? L("Pontos egyezés") : nil,
                        r.trusted == true ? L("Megbízható feltöltő") : nil,
                        r.hearingImpaired == true ? L("Hallássérülteknek") : nil,
                        r.forced == true ? L("kényszerített") : nil,
                        r.machineMade == true ? L("Gépi fordítás") : nil,
                        L("{0} letöltés", r.downloads ?? 0),
                    ].compactMap { $0 }
                    Text(flags.joined(separator: " · ")).font(.caption).opacity(0.8)
                }
                Spacer(minLength: 20)
                if busy == r.fileId {
                    ProgressView()
                } else if r.have == true {
                    Label(L("Megvan"), systemImage: "checkmark.circle.fill").font(.callout)
                } else {
                    Image(systemName: "arrow.down.circle").font(.title2)
                }
            }
        }
        .buttonStyle(TVRowStyle())
        .disabled(r.have == true || (busy != nil && busy != r.fileId))
    }

    private var langChoices: [String] {
        var out = langs
        for c in Self.common where !out.contains(c) { out.append(c) }
        return out
    }

    private func langName(_ code: String) -> String { TrackLabel.language(code) ?? code.uppercased() }

    private func toggle(_ code: String) {
        if let i = langs.firstIndex(of: code) {
            guard langs.count > 1 else { return }
            langs.remove(at: i)
        } else {
            langs.append(code)
        }
        Task { await search() }
    }

    private func search() async {
        loading = true
        error = nil
        defer { loading = false }
        do {
            let asked = langs
            let res = try await model.api { [id = item.id] in try await $0.searchSubtitles(id, langs: asked.isEmpty ? nil : asked) }
            guard asked == langs else { return }
            enabled = res.enabled
            if langs.isEmpty { langs = res.langs ?? [] }
            results = res.results
        } catch is CancellationError {
        } catch {
            self.error = error.localizedDescription
        }
    }

    private func download(_ r: SubtitleResult) async {
        guard busy == nil, r.have != true else { return }
        busy = r.fileId
        defer { busy = nil }
        do {
            let res = try await model.api { [id = item.id] in try await $0.downloadSubtitle(id, r) }
            if let i = results.firstIndex(where: { $0.fileId == r.fileId }) { results[i].have = true }
            if let onDownloaded {
                onDownloaded(res.stream)
                dismiss()
            } else if let left = res.remaining, left >= 0 {
                notice = L("Felirat letöltve. Mára még {0} letöltés maradt.", left)
            } else {
                notice = L("Felirat letöltve.")
            }
        } catch {
            notice = error.localizedDescription
        }
    }
}
