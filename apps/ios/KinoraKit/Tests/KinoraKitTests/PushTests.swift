import Foundation
import Testing
@testable import KinoraKit

struct PushTests {
    /// Sealed by the Go server (internal/push.Seal) with a key of 32 × 0x07.
    static let fromServer = "FUfdaGvCejNW4NQLPE46hlyKRC2BNMMRLr-UdOMG0MaqX1kWHF4X3c2qxShri5kauBEZ4zq0XqyZolPUFmfd-2jelGeXnRTWJWa_ZE6qnYa2CwdK94EZoGJm2_kQuqlP2Qgw_0w4oM5Sno4AOT0Guz34aVcaWSjLh07AIDY7HggwXaewX7QmazTs8YZxauMNq8qYjloR3aKeUY1X7Nhgjfhjk0QjW9dPnu0k102fuLq7hrHD0DYLS2w_z7jowf5V1X8x1Gz8nTILcd_G4LOq"

    @Test func opensWhatTheServerSealed() throws {
        let m = try PushCrypto.message(Self.fromServer, key: Data(repeating: 7, count: 32))
        #expect(m.title == "Mátrix")
        #expect(m.item == 42)
        #expect(m.image == "/api/images/42/poster")
        #expect(m.type == "request_available")
    }

    @Test func wrongKeyFails() {
        #expect(throws: (any Error).self) { try PushCrypto.message(Self.fromServer, key: Data(repeating: 8, count: 32)) }
    }

    @Test func roundTrip() throws {
        let key = PushCrypto.newKey()
        #expect(key.count == 32)
        let sealed = try PushCrypto.seal(Data("szia".utf8), key: key)
        #expect(sealed.count == 12 + 4 + 16)
        #expect(String(decoding: try PushCrypto.open(sealed, key: key), as: UTF8.self) == "szia")
    }

    @Test func clientDataMatchesRelay() {
        let d = PushRelay.clientData(challenge: "c", serverId: "srv_x", apnsToken: "ab", environment: "sandbox")
        #expect(String(decoding: d, as: UTF8.self) == "kinora-relay-v1\nc\nsrv_x\nab\nsandbox")
    }

    @Test func base64URL() {
        let d = Data([0xfb, 0xff, 0x01])
        #expect(d.base64URLEncoded == "-_8B")
        #expect(Data(base64URLEncoded: "-_8B") == d)
    }

    @Test func storeKeepsOnePairingPerServer() {
        let defaults = UserDefaults(suiteName: "push-test-\(UUID())")!
        let store = PushStore(defaults: defaults, keychain: Keychain(service: "kinora.push.test.\(UUID())"))
        let p = PushPairing(savedServerId: "a", relayUrl: "https://r", relayServerId: "srv_1", handle: "h1", apnsToken: "t", environment: "sandbox")
        store.save(p, key: Data(repeating: 1, count: 32))
        var q = p
        q.handle = "h2"
        store.save(q, key: Data(repeating: 2, count: 32))
        #expect(store.pairings.count == 1)
        #expect(store.pairing(relayServer: "srv_1")?.handle == "h2")
        store.remove(saved: "a")
        #expect(store.pairings.isEmpty)
    }

    @Test func statusDecodes() throws {
        let s = try JSONDecoder.kinora.decode(PushStatus.self, from: Data(#"{"enabled":true,"relay_url":"https://r","server_id":"srv_1","paired":false}"#.utf8))
        #expect(s == PushStatus(enabled: true, relayUrl: "https://r", serverId: "srv_1", paired: false))
    }

    @Test func provisionlessBuildIsProduction() {
        #expect(PushEnvironment.current(Bundle(for: Marker.self)) == "production")
    }
}

private final class Marker {}
