import SwiftUI
import KinoraKit

/// Settings → Otthon-jelenetek: which Home app scene runs at the start, pause and end of playback.
struct HomeScenesView: View {
    @State private var homes = HomeScenes.shared
    @State private var picks: [HomeScenes.Moment: UUID] = [:]

    var body: some View {
        Form {
            if homes.denied {
                Section {
                    Text(L("A Kinora nem férhet hozzá az Otthonhoz. Engedélyezd a Beállítások → Kinora → Otthon kapcsolóval."))
                        .foregroundStyle(Theme.muted)
                }
                .listRowBackground(Theme.panel)
            } else if !homes.loaded {
                Section { ProgressView().frame(maxWidth: .infinity) }
                    .listRowBackground(Theme.panel)
            } else if homes.scenes.isEmpty {
                Section {
                    Text(L("Még nincs jeleneted. Az Otthon appban hozz létre egyet (például „Mozi”: lámpák le, TV be), aztán itt kiválaszthatod."))
                        .foregroundStyle(Theme.muted)
                }
                .listRowBackground(Theme.panel)
            } else {
                Section {
                    ForEach(HomeScenes.Moment.allCases) { moment in
                        Picker(moment.label, selection: binding(moment)) {
                            Text(L("Semmi")).tag(UUID?.none)
                            ForEach(homes.scenes) { scene in
                                Text(multipleHomes ? "\(scene.name) · \(scene.home)" : scene.name).tag(UUID?.some(scene.id))
                            }
                        }
                    }
                } footer: {
                    Text(L("A szünet jelenete pár másodperc után fut le, hogy egy tekerés ne kapcsolja fel a lámpákat. Részek között, automatikus továbblépésnél nem fut semmi. Csak ezen a készüléken érvényes."))
                }
                .listRowBackground(Theme.panel)
            }
        }
        .scrollContentBackground(.hidden)
        .background(Theme.black)
        .navigationTitle(L("Otthon-jelenetek"))
        .navigationBarTitleDisplayMode(.inline)
        .onAppear {
            homes.connect()
            for m in HomeScenes.Moment.allCases { picks[m] = homes.scene(for: m) }
        }
    }

    private var multipleHomes: Bool { Set(homes.scenes.map(\.home)).count > 1 }

    private func binding(_ moment: HomeScenes.Moment) -> Binding<UUID?> {
        Binding(get: { picks[moment] ?? nil }, set: { picks[moment] = $0; homes.setScene($0, for: moment) })
    }
}
