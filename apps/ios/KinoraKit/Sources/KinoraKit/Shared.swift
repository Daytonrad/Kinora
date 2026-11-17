import Foundation

/// The App Group the app and its widgets share: saved servers in its UserDefaults suite,
/// tokens in its Keychain access group, the widgets' last good data in its container.
/// Its id is in each target's Info.plist (`KinoraAppGroup`), so a fork with its own
/// bundle ID gets its own group without touching code.
public enum AppGroup {
    public static let id: String? = {
        guard let v = Bundle.main.object(forInfoDictionaryKey: "KinoraAppGroup") as? String,
              !v.isEmpty, !v.contains("$(") else { return nil }
        return v
    }()

    public static var defaults: UserDefaults? { id.flatMap(UserDefaults.init(suiteName:)) }

    public static var container: URL? {
        id.flatMap { FileManager.default.containerURL(forSecurityApplicationGroupIdentifier: $0) }
    }
}

extension ServerStore {
    /// The store the app and the widgets both see. Builds without the App Group
    /// (tests, `swift test`) fall back to the app's own storage.
    public static func shared() -> ServerStore {
        guard let suite = AppGroup.defaults else { return ServerStore() }
        let store = ServerStore(defaults: suite, keychain: Keychain(accessGroup: AppGroup.id))
        store.adopt(from: .standard)
        return store
    }
}
