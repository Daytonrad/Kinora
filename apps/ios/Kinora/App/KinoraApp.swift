import CoreSpotlight
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
                .task {
                    _ = NetworkStatus.shared
                    IntentRouter.model = model
                    HomeScenes.shared.warmUp()
                    await model.start()
                    KinoraShortcuts.updateAppShortcutParameters()
                }
                .onOpenURL { model.open($0) }
                .onReceive(NotificationCenter.default.publisher(for: NetworkStatus.cameOnline)) { _ in model.syncOffline() }
                .onContinueUserActivity(CSSearchableItemActionType) { activity in
                    if let id = activity.userInfo?[CSSearchableItemActivityIdentifier] as? String,
                       let url = URL(string: id) { model.open(url) }
                }
        }
        // Watched marks, favorites and the like change what the widgets show;
        // refreshing on the way out catches all of them.
        .onChange(of: phase) { _, now in
            if now == .background { model.reloadWidgets() }
            if now == .active { model.syncOffline() }
        }
    }
}
