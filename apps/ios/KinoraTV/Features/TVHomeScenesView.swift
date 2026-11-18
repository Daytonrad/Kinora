import SwiftUI
import KinoraKit

/// Beállítások → Otthon-jelenetek on the Apple TV: which Home app scene runs at the start,
/// pause and end of playback. Same logic and storage as the iPhone app (HomeScenes).
struct TVHomeScenesView: View {
    @State private var homes = HomeScenes.shared
    @State private var picks: [HomeScenes.Moment: UUID] = [:]

    var body: some View {
        ScrollView {
            VStack(alignment: .leading, spacing: 22) {
                Text(L("Otthon-jelenetek")).font(.title2).padding(.bottom, 20)
                if homes.denied {
                    Text(L("A Kinora nem férhet hozzá az Otthonhoz. Engedélyezd a Beállítások → Adatvédelem → HomeKit menüben."))
                        .foregroundStyle(Theme.muted)
                } else if !homes.loaded {
                    ProgressView().frame(maxWidth: .infinity)
                } else if homes.scenes.isEmpty {
                    Text(L("Még nincs jeleneted. Az Otthon appban hozz létre egyet (például „Mozi”: lámpák le, TV be), aztán itt kiválaszthatod."))
                        .foregroundStyle(Theme.muted)
                } else {
                    ForEach(HomeScenes.Moment.allCases) { moment in
                        TVHeading(text: moment.label).padding(.top, 20)
                        ScrollView(.horizontal) {
                            HStack(spacing: 20) {
                                pill(L("Semmi"), id: nil, moment: moment)
                                ForEach(homes.scenes) { scene in
                                    pill(multipleHomes ? "\(scene.name) · \(scene.home)" : scene.name, id: scene.id, moment: moment)
                                }
                            }
                            .padding(.vertical, 20)
                        }
                        .scrollClipDisabled()
                        .focusSection()
                    }
                    Text(L("A szünet jelenete pár másodperc után fut le, hogy egy tekerés ne kapcsolja fel a lámpákat. Részek között, automatikus továbblépésnél nem fut semmi. Csak ezen a készüléken érvényes."))
                        .font(.caption).foregroundStyle(Theme.muted).padding(.top, 20)
                }
            }
            .frame(maxWidth: 1400, alignment: .leading)
            .padding(80)
        }
        .background(Theme.black)
        .onAppear {
            homes.connect()
            for m in HomeScenes.Moment.allCases { picks[m] = homes.scene(for: m) }
        }
    }

    private var multipleHomes: Bool { Set(homes.scenes.map(\.home)).count > 1 }

    private func pill(_ title: String, id: UUID?, moment: HomeScenes.Moment) -> some View {
        Button(title) {
            picks[moment] = id
            homes.setScene(id, for: moment)
        }
        .buttonStyle(TVPillStyle(prominent: picks[moment] == id))
    }
}
