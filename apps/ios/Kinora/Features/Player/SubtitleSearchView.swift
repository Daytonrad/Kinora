import SwiftUI
import KinoraKit

/// Subtitle search on OpenSubtitles through the server, from the player and the film page.
/// A downloaded file stays on the server and shows as a track of the item from then on.
struct SubtitleSearchView: View {
    @Environment(AppModel.self) private var model
    @Environment(\.dismiss) private var dismiss
    var item: Item
    /// Gets the new track after a download (the player switches to it).
    var onDownloaded: ((MediaStream) -> Void)?

    @State private var langs: [String] = []
    @State private var results: [SubtitleResult] = []
    @State private var enabled = true
    @State private var loading = true
    @State private var error: String?
    @State private var busy: Int?
    @State private var notice: String?

    /// Languages offered in the filter besides the profile's own.
    private static let common = ["hun", "eng", "ger", "fre", "spa", "ita", "por", "pol", "cze", "slo", "rum", "rus", "dut", "swe"]

    var body: some View {
        NavigationStack {
            List {
                if enabled {
                    Section {
                        ScrollView(.horizontal, showsIndicators: false) {
                            HStack(spacing: 8) {
                                ForEach(langChoices, id: \.self) { code in
                                    let on = langs.contains(code)
                                    Button {
                                        Haptic.select.play()
                                        toggle(code)
                                    } label: {
                                        Text(Self.langName(code))
                                            .font(.subheadline.weight(on ? .semibold : .regular))
                                            .padding(.horizontal, 12).padding(.vertical, 6)
                                            .background(on ? Color.accentColor.opacity(0.25) : Color.secondary.opacity(0.15), in: .capsule)
                                    }
                                    .buttonStyle(.plain)
                                    .accessibilityAddTraits(on ? .isSelected : [])
                                }
                            }
                            .padding(.vertical, 2)
                        }
                    } header: {
                        Text(L("Keresett nyelvek"))
                    }
                }
                content
            }
            .navigationTitle(L("Felirat keresése"))
            .navigationBarTitleDisplayMode(.inline)
            .toolbar {
                ToolbarItem(placement: .cancellationAction) {
                    Button(L("Bezárás")) { dismiss() }
                }
            }
            .refreshable { await search() }
            .task { await search() }
            .alert(L("Felirat"), isPresented: Binding(get: { notice != nil }, set: { if !$0 { notice = nil } })) {
                Button("OK", role: .cancel) {}
            } message: {
                Text(notice ?? "")
            }
        }
    }

    @ViewBuilder private var content: some View {
        if loading && results.isEmpty {
            HStack(spacing: 10) {
                ProgressView()
                Text(L("Keresés az OpenSubtitles-ön…")).foregroundStyle(.secondary)
            }
        } else if let error {
            Text(error).foregroundStyle(.red)
        } else if !enabled {
            Text(model.me?.user.isAdmin == true
                 ? L("A feliratkereséshez adj meg egy OpenSubtitles API-kulcsot: Admin → Metaadatok.")
                 : L("A feliratkeresés nincs bekapcsolva ezen a szerveren. Szólj az adminnak."))
                .foregroundStyle(.secondary)
        } else if results.isEmpty {
            Text(L("Ezeken a nyelveken nincs találat.")).foregroundStyle(.secondary)
        } else {
            Section {
                ForEach(results) { r in row(r) }
            } footer: {
                Text(L("Feliratok: OpenSubtitles.com"))
            }
        }
    }

    private func row(_ r: SubtitleResult) -> some View {
        HStack(alignment: .top, spacing: 12) {
            Text((r.lang ?? "?").uppercased())
                .font(.caption.weight(.bold).monospaced())
                .frame(width: 40, height: 24)
                .background(Color.secondary.opacity(0.18), in: .rect(cornerRadius: 6))
            VStack(alignment: .leading, spacing: 4) {
                Text(r.name).font(.subheadline).lineLimit(3)
                let flags = [
                    r.hashMatch == true ? L("Pontos egyezés") : nil,
                    r.trusted == true ? L("Megbízható feltöltő") : nil,
                    r.hearingImpaired == true ? L("Hallássérülteknek") : nil,
                    r.forced == true ? L("kényszerített") : nil,
                    r.machineMade == true ? L("Gépi fordítás") : nil,
                ].compactMap { $0 }
                if !flags.isEmpty {
                    Text(flags.joined(separator: " · "))
                        .font(.caption.weight(.semibold))
                        .foregroundStyle(r.hashMatch == true ? Color.green : r.machineMade == true ? Color.orange : Color.secondary)
                }
                Text([L("{0} letöltés", r.downloads ?? 0), r.uploader].compactMap { $0 }.filter { !$0.isEmpty }.joined(separator: " · "))
                    .font(.caption).foregroundStyle(.secondary)
            }
            Spacer(minLength: 4)
            if r.have == true {
                Label(L("Megvan"), systemImage: "checkmark")
                    .labelStyle(.iconOnly)
                    .foregroundStyle(.green)
                    .frame(width: 44, height: 32)
                    .accessibilityLabel(L("Megvan"))
            } else if busy == r.fileId {
                ProgressView().frame(width: 44, height: 32)
            } else {
                Button {
                    Task { await download(r) }
                } label: {
                    Image(systemName: "arrow.down.circle.fill").font(.title2)
                        .frame(width: 44, height: 32)
                }
                .buttonStyle(.borderless)
                .disabled(busy != nil)
                .accessibilityLabel(L("Letöltés"))
            }
        }
        .padding(.vertical, 2)
    }

    private var langChoices: [String] {
        var out = langs
        for c in Self.common where !out.contains(c) { out.append(c) }
        return out
    }

    private static func langName(_ code: String) -> String {
        TrackLabel.language(code) ?? code.uppercased()
    }

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
        busy = r.fileId
        defer { busy = nil }
        do {
            let res = try await model.api { [id = item.id] in try await $0.downloadSubtitle(id, r) }
            Haptic.success.play()
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
            Haptic.error.play()
            notice = error.localizedDescription
        }
    }
}
