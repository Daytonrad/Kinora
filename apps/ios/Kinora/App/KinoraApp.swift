import SwiftUI

@main
struct KinoraApp: App {
    @State private var model = AppModel()
    @Environment(\.scenePhase) private var phase

    var body: some Scene {
        WindowGroup {
            RootView()
                .environment(model)
                .preferredColorScheme(.dark)
                .tint(Theme.amber)
                .task { await model.start() }
                .onOpenURL { model.open($0) }
        }
        // Watched marks, favorites and the like change what the widgets show;
        // refreshing on the way out catches all of them.
        .onChange(of: phase) { _, now in
            if now == .background { model.reloadWidgets() }
        }
    }
}
