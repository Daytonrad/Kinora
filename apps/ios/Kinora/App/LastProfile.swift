import Foundation
import KinoraKit

/// The profile used last with each server. When the app starts without a connection, the downloads of this
/// profile are shown (and only those: another profile, a kids profile above all, never sees them).
enum LastProfile {
    private static func key(_ server: String) -> String { "kinora.lastProfile." + server }

    static func save(_ profile: Profile, server: String) {
        guard let data = try? JSONEncoder().encode(profile) else { return }
        UserDefaults.standard.set(data, forKey: key(server))
    }

    static func load(server: String) -> Profile? {
        UserDefaults.standard.data(forKey: key(server)).flatMap { try? JSONDecoder().decode(Profile.self, from: $0) }
    }

    static func forget(server: String) {
        UserDefaults.standard.removeObject(forKey: key(server))
    }
}
