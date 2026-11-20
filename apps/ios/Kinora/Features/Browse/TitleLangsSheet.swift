import SwiftUI
import KinoraKit

/// The audio and subtitle language this profile starts a film or series with. Kept on the server, so every
/// device and the web start the same way; a choice made in the player changes it too.
struct TitleLangsSheet: View {
    @Environment(AppModel.self) private var model
    @Environment(\.dismiss) private var dismiss
    /// A film, a series, or an episode (it stands for its series).
    var item: Item

    @State private var langs: TitleLangs?
    @State private var audio = ""
    @State private var subtitle = ""
    @State private var error: String?
    @State private var saving = false

    private var isSeries: Bool { item.kind != .movie }

    var body: some View {
        NavigationStack {
            Form {
                if let langs {
                    Section {
                        Picker(L("Hang"), selection: $audio) {
                            Text(L("A profil beállítása szerint")).tag("")
                            ForEach(choices(langs.audio ?? [], current: langs.audioLang), id: \.self) { Text(name($0)).tag($0) }
                        }
                        Picker(L("Felirat"), selection: $subtitle) {
                            Text(L("A profil beállítása szerint")).tag("")
                            Text(L("Kikapcsolva")).tag(TitleLangs.off)
                            ForEach(choices(langs.subs ?? [], current: langs.subLang == TitleLangs.off ? "" : langs.subLang), id: \.self) {
                                Text(name($0)).tag($0)
                            }
                        }
                    } footer: {
                        Text(isSeries
                             ? L("A sorozat minden része ezzel a hanggal és felirattal indul ennél a profilnál. Ha lejátszás közben választasz mást, a következő részek is úgy indulnak.")
                             : L("Ezzel a hanggal és felirattal indul a film ennél a profilnál. Ha lejátszás közben választasz mást, azt is megjegyzi."))
                    }
                    if let error {
                        Section { Text(error).foregroundStyle(Theme.danger) }
                    }
                } else {
                    LoadingState(error: error) { Task { await load() } }
                        .listRowBackground(Color.clear)
                }
            }
            .scrollContentBackground(.hidden)
            .kinoraScreen()
            .navigationTitle(L("Hang és felirat"))
            .navigationBarTitleDisplayMode(.inline)
            .toolbar {
                ToolbarItem(placement: .cancellationAction) { Button(L("Mégse")) { dismiss() } }
                ToolbarItem(placement: .confirmationAction) {
                    Button(L("Mentés")) { Task { await save() } }
                        .disabled(langs == nil || saving)
                }
            }
        }
        .presentationDetents([.medium, .large])
        .task { await load() }
    }

    /// The file's languages, with the saved one kept even when no file has it any more.
    private func choices(_ list: [String], current: String) -> [String] {
        current.isEmpty || list.contains(current) ? list : [current] + list
    }

    private func name(_ code: String) -> String { TrackLabel.language(code) ?? code.uppercased() }

    private func load() async {
        do {
            let l = try await model.api { [id = item.id] client in try await client.titleLangs(id) }
            audio = l.audioLang
            subtitle = l.subLang
            langs = l
            error = nil
        } catch is CancellationError {
        } catch {
            self.error = error.localizedDescription
        }
    }

    private func save() async {
        saving = true
        defer { saving = false }
        do {
            let a = audio, s = subtitle
            try await model.api { [id = item.id] client in try await client.setTitleLangs(id, audio: a, subtitle: s) }
            Haptic.success.play()
            dismiss()
        } catch {
            Haptic.error.play()
            self.error = error.localizedDescription
        }
    }
}
