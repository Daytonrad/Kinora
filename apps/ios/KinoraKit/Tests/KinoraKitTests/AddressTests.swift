import Testing
@testable import KinoraKit

// Same cases as apps/shared/test/unit.test.mjs: every client must read addresses alike.
@Suite struct AddressTests {
    @Test func lanIP() {
        #expect(ServerAddress.candidates("192.168.1.10") == ["http://192.168.1.10:8096", "http://192.168.1.10", "https://192.168.1.10"])
        #expect(ServerAddress.candidates(" 192.168.1.10:9000 ") == ["http://192.168.1.10:9000", "https://192.168.1.10:9000"])
    }

    @Test func explicitScheme() {
        #expect(ServerAddress.candidates("http://10.0.0.5") == ["http://10.0.0.5:8096", "http://10.0.0.5"])
        #expect(ServerAddress.candidates("https://kinora.pelda.hu/") == ["https://kinora.pelda.hu", "https://kinora.pelda.hu:8096"])
        #expect(ServerAddress.candidates("HTTP://nas.local:8096") == ["http://nas.local:8096"])
    }

    @Test func names() {
        #expect(ServerAddress.candidates("nas.local").first == "http://nas.local:8096")
        #expect(ServerAddress.candidates("kinora.pelda.hu") == ["https://kinora.pelda.hu", "http://kinora.pelda.hu:8096", "http://kinora.pelda.hu"])
        #expect(ServerAddress.candidates("mozi.duckdns.org:8920") == ["https://mozi.duckdns.org:8920", "http://mozi.duckdns.org:8920"])
        #expect(ServerAddress.candidates("") == [])
    }

    @Test func privateHosts() {
        #expect(ServerAddress.isPrivateHost("192.168.0.4"))
        #expect(ServerAddress.isPrivateHost("172.20.1.1"))
        #expect(!ServerAddress.isPrivateHost("172.32.1.1"))
        #expect(!ServerAddress.isPrivateHost("kinora.pelda.hu"))
    }

    @Test func responseBase() {
        #expect(ServerAddress.base(fromResponse: "https://k.example/api/status", fallback: "http://k.example") == "https://k.example")
        #expect(ServerAddress.base(fromResponse: nil, fallback: "http://x:8096") == "http://x:8096")
    }

    @Test func merge() {
        #expect(ServerAddress.merge(["http://a/", nil], ["http://a", "https://b"]) == ["http://a", "https://b"])
    }

    @Test func compatibility() {
        #expect(ServerStatus(version: "1", apiVersion: 1, serverName: "x").incompatibility == .serverTooOld)
        #expect(ServerStatus(version: "1", apiVersion: 2, minClientApi: 3, serverName: "x").incompatibility == .serverTooNew)
        #expect(ServerStatus(version: "1", serverName: "x").incompatibility == nil)
    }
}
