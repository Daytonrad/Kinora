import Foundation
import Testing
@testable import KinoraKit

struct PartyTests {
    @Test func decodesState() throws {
        let json = #"""
        {"t":"state","code":"ABC234","item_id":42,"playing":true,"intended":true,"pos":61.5,"at":1000,
         "waiting":[],"members":[{"id":1,"name":"Anna","color":"#f80","buffering":false,"host":true}],
         "everyone":false,"by":1,"seq":3}
        """#
        guard case .state(let st) = PartyEvent.decode(Data(json.utf8)) else { Issue.record("not a state"); return }
        #expect(st.itemId == 42 && st.playing && st.pos == 61.5 && st.seq == 3)
        #expect(st.members == [PartyMember(id: 1, name: "Anna", color: "#f80", host: true)])
    }

    @Test func decodesOtherMessages() {
        #expect(PartyEvent.decode(Data(#"{"t":"hello","id":4,"reactions":["😂"]}"#.utf8)) == .hello(id: 4, reactions: ["😂"]))
        #expect(PartyEvent.decode(Data(#"{"t":"pong","c":100,"s":5000}"#.utf8)) == .pong(client: 100, server: 5000))
        #expect(PartyEvent.decode(Data(#"{"t":"sys","text":"Anna elindította","at":1}"#.utf8)) == .system("Anna elindította"))
        #expect(PartyEvent.decode(Data(##"{"t":"react","name":"A","color":"#fff","emoji":"🔥"}"##.utf8))
                == .react(name: "A", color: "#fff", emoji: "🔥"))
        #expect(PartyEvent.decode(Data(#"{"t":"future"}"#.utf8)) == nil)
        #expect(PartyEvent.decode(Data("nope".utf8)) == nil)
    }

    @Test func encodesCommands() throws {
        func obj(_ c: PartyCommand) throws -> NSDictionary {
            try #require(JSONSerialization.jsonObject(with: c.json) as? NSDictionary)
        }
        #expect(try obj(.pause(at: 12.5)) == ["t": "pause", "pos": 12.5])
        #expect(try obj(.skip(to: 90, kind: .intro)) == ["t": "skip", "pos": 90, "kind": "intro"])
        #expect(try obj(.item(7)) == ["t": "item", "item_id": 7, "pos": 0])
        #expect(try obj(.buffering(true)) == ["t": "buffering", "v": true])
    }

    @Test func positionAdvancesOnlyWhilePlaying() {
        let playing = PartyState(itemId: 1, playing: true, pos: 10, at: 1000)
        #expect(playing.position(at: 3000) == 12)
        let paused = PartyState(itemId: 1, playing: false, pos: 10, at: 1000)
        #expect(paused.position(at: 3000) == 10)
    }

    @Test func clockTrustsFastRoundTrips() {
        var clock = PartyClock()
        clock.pong(client: 1000, server: 6000, now: 1100) // rtt 100 → offset 4950
        #expect(clock.offset == 4950)
        clock.pong(client: 2000, server: 9000, now: 2400) // rtt 400: too slow, ignored
        #expect(clock.offset == 4950)
        clock.pong(client: 3000, server: 8010, now: 3020) // rtt 20 → offset 5000
        #expect(clock.offset == 5000)
        #expect(clock.serverNow(10) == 5010)
    }

    @Test func syncSeeksNudgesOrHolds() {
        #expect(PartySync.decide(target: 100, current: 90, playing: true, fromState: false) == .init(seek: 100, rate: 1))
        #expect(PartySync.decide(target: 100, current: 99.5, playing: true, fromState: false) == .init(seek: nil, rate: 1.05))
        #expect(PartySync.decide(target: 100, current: 100.5, playing: true, fromState: false) == .init(seek: nil, rate: 0.95))
        #expect(PartySync.decide(target: 100, current: 100.1, playing: true, fromState: false) == .init(seek: nil, rate: 1))
        // Paused: only a fresh state moves the viewer.
        #expect(PartySync.decide(target: 100, current: 50, playing: false, fromState: false) == .init(seek: nil, rate: 1))
        #expect(PartySync.decide(target: 100, current: 50, playing: false, fromState: true) == .init(seek: 100, rate: 1))
    }

    @Test func scannedCodes() {
        #expect(PartySync.code(scanned: "kinora://party/abc234") == "ABC234")
        #expect(PartySync.code(scanned: "https://media.example.com/#/party/ABC234") == "ABC234")
        #expect(PartySync.code(scanned: "http://192.168.1.5:8096/#/party/xyz789?x=1") == "XYZ789")
        #expect(PartySync.code(scanned: "https://media.example.com/party/ABC234") == "ABC234")
        #expect(PartySync.code(scanned: " abc234\n") == "ABC234")
        #expect(PartySync.code(scanned: "kinora://item/42") == nil)
        #expect(PartySync.code(scanned: "https://example.com/#/item/42") == nil)
        #expect(PartySync.code(scanned: "WIFI:S:home;T:WPA;P:secret;;") == nil)
        #expect(PartySync.code(scanned: "Hello world") == nil)
    }

    @Test func codesAndLinks() {
        #expect(PartySync.normalizedCode(" abc-234 ") == "ABC234")
        #expect(PartySync.normalizedCode("ABC23") == nil)
        #expect(KinoraLink(url: URL(string: "kinora://party/abc234")!) == .party("ABC234"))
        #expect(KinoraLink.party("ABC234").url.absoluteString == "kinora://party/ABC234")
        #expect(KinoraLink(url: URL(string: "kinora://play/12")!) == .play(12))
        #expect(KinoraLink(url: URL(string: "kinora://play/x")!) == nil)
    }

    @Test func nonFiniteCommandIsDroppedNotACrash() {
        #expect(PartyCommand.pause(at: .nan).json.isEmpty)
        #expect(PartyCommand.seek(to: .infinity).json.isEmpty)
        #expect(!PartyCommand.pause(at: 12.5).json.isEmpty)
    }
}
