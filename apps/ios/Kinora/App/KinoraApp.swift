import SwiftUI

@main
struct KinoraApp: App {
    @State private var model = AppModel()

    var body: some Scene {
        WindowGroup {
            RootView()
                .environment(model)
                .preferredColorScheme(.dark)
                .tint(Theme.amber)
                .task { await model.start() }
        }
    }
}
