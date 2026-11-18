import Foundation

public enum KinoraError: Error, Equatable, LocalizedError, Sendable {
    /// No answer at all (offline, wrong address, server down).
    case network
    case unreachable(String, isPrivate: Bool)
    case notKinora(String)
    case setupNeeded
    case serverTooOld
    case serverTooNew
    /// The token is no longer valid: log in again.
    case unauthorized
    case http(Int, String)

    public var status: Int {
        switch self {
        case .unauthorized: 401
        case .http(let code, _): code
        default: 0
        }
    }

    public var errorDescription: String? {
        switch self {
        case .network:
            L("Nem érem el a szervert.")
        case .unreachable(let address, let isPrivate):
            L("Nem érem el a szervert ({0}). Ellenőrizd, hogy fut-e, és hogy ugyanazon a hálózaton vagy-e.", address)
                + (isPrivate ? L(" Ez otthoni (belső) cím: csak az otthoni Wi-Fi-ről érhető el.") : "")
        case .notKinora(let address):
            L("A(z) {0} címen nem Kinora-szerver válaszol.", address)
        case .setupNeeded:
            L("Ezt a szervert még be kell állítani. Nyisd meg böngészőben, és hozd létre az admin fiókot.")
        case .serverTooOld:
            L("A szerver túl régi ehhez az apphoz. Frissítsd a Kinorát a szerveren.")
        case .serverTooNew:
            L("Az app túl régi ehhez a szerverhez. Frissítsd az appot.")
        case .unauthorized:
            L("A bejelentkezés lejárt, lépj be újra.")
        case .http(let code, let message):
            message.isEmpty ? "HTTP \(code)" : Language.shared.server(message)
        }
    }
}
