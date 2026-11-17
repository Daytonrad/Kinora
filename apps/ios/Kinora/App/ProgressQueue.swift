import Foundation
import KinoraKit

/// Position reports that could not reach the server (offline viewing); sent when a
/// connection is back. Only the latest report per title is kept.
@MainActor
enum ProgressQueue {
    struct Entry: Codable, Equatable {
        var position: Double
        var ended: Bool
    }

    private static let key = "kinora.pendingProgress"
    private static var flushing = false

    static var pending: [Int: Entry] {
        get {
            UserDefaults.standard.data(forKey: key).flatMap { try? JSONDecoder().decode([Int: Entry].self, from: $0) } ?? [:]
        }
        set { UserDefaults.standard.set(try? JSONEncoder().encode(newValue), forKey: key) }
    }

    /// Sends the report; queues it when the server can't be reached.
    static func report(_ id: Int, position: Double, ended: Bool, client: KinoraClient) async {
        do {
            try await client.reportProgress(id, position: position, ended: ended)
            if pending[id] != nil { pending[id] = nil }
        } catch where !retryable(error) {
            // The server refused it (unknown title): retrying won't help.
        } catch {
            var all = pending
            // A finished viewing stays finished even if a later report says otherwise.
            let wasEnded = all[id]?.ended == true && !ended && position < 1
            all[id] = wasEnded ? all[id] : Entry(position: position, ended: ended)
            pending = all
        }
    }

    /// No answer, a server error or a lost sign-in: worth sending again later.
    private static func retryable(_ error: Error) -> Bool {
        guard case .http(let code, _) = error as? KinoraError else { return true }
        return code >= 500 || code == 401 || code == 408 || code == 429
    }

    /// Sends what is waiting; stops at the first network failure.
    static func flush(_ client: KinoraClient) async {
        guard !flushing, !pending.isEmpty else { return }
        flushing = true
        defer { flushing = false }
        for (id, e) in pending {
            do {
                try await client.reportProgress(id, position: e.position, ended: e.ended)
                pending[id] = nil
            } catch where !retryable(error) {
                pending[id] = nil
            } catch {
                return
            }
        }
    }
}
