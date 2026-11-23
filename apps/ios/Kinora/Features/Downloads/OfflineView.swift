import SwiftUI
import KinoraKit

/// The app started without a connection to the server: the last profile's downloads, playable from the device.
/// Watched positions wait in ProgressQueue and reach the server once it answers again.
struct OfflineView: View {
    @Environment(AppModel.self) private var model
    @State private var retrying = false

    var body: some View {
        NavigationStack {
            DownloadsView()
                .safeAreaInset(edge: .top) { banner }
        }
        // the player, as on the home screen
        .fullScreenCover(isPresented: Binding(get: { model.playerVisible && model.player != nil },
                                              set: { model.playerVisible = $0 })) {
            if let p = model.player { PlayerView(player: p) }
        }
    }

    private var banner: some View {
        VStack(alignment: .leading, spacing: 10) {
            Label(L("Nincs kapcsolat a szerverrel"), systemImage: "wifi.slash")
                .font(.headline)
            Text(L("{0} letöltései offline is lejátszhatók. A többihez kapcsolat kell.", model.offlineProfile?.name ?? ""))
                .font(.subheadline)
                .foregroundStyle(Theme.plaster)
                .fixedSize(horizontal: false, vertical: true)
            ErrorText(message: model.notice)
            HStack(spacing: 16) {
                Button {
                    retrying = true
                    model.notice = nil
                    Task {
                        await model.retryOnline()
                        retrying = false
                    }
                } label: {
                    if retrying { ProgressView() } else { Text(L("Újra")) }
                }
                .buttonStyle(.borderedProminent)
                .disabled(retrying)
                Button(L("Másik szerver")) { model.leaveOffline() }
            }
        }
        .padding(16)
        .frame(maxWidth: .infinity, alignment: .leading)
        .background(.thinMaterial)
    }
}
