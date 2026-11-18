import SwiftUI
import Observation
import KinoraKit

/// One watch party room, joined: the WebSocket, the members and chat, and the sync that keeps
/// the player on the room's clock. iPhone, iPad and Apple TV share it.
///
/// The player tells the room what the viewer did (play, pause, seek, skip, next episode,
/// buffering); the room answers with its state, and `sync` moves the player there. A viewer
/// who may not control is snapped back by the server's state.
@MainActor @Observable
final class PartySession {
    struct Line: Identifiable, Equatable {
        let id = UUID()
        var name: String?
        var color: String?
        var text: String
        var at = Date.now
    }

    struct Floating: Identifiable, Equatable {
        let id = UUID()
        var emoji: String
        var name: String
        /// 0…1 across the screen.
        var x: Double
    }

    let code: String
    private(set) var members: [PartyMember] = []
    private(set) var lines: [Line] = []
    private(set) var reactions: [String] = []
    private(set) var floating: [Floating] = []
    /// Connection trouble or the members the room waits for.
    private(set) var status: String?
    private(set) var state: PartyState?
    private(set) var myId = 0
    /// The room is gone (expired, wrong code, no access): reconnecting stopped.
    private(set) var gone = false
    /// Unread chat lines, for a badge while the panel is closed.
    var unread = 0
    var panelOpen = false { didSet { if panelOpen { unread = 0 } } }

    var me: PartyMember? { members.first { $0.id == myId } }
    var isHost: Bool { me?.host ?? false }
    var everyone: Bool { state?.everyone ?? false }
    var canControl: Bool { isHost || everyone }
    var appLink: URL { KinoraLink.party(code).url }
    private(set) var webLink: URL?

    /// The player that follows the room (it owns this session).
    weak var player: PlayerModel? {
        didSet { if let player { buffering(player.waiting) } }
    }

    private let client: KinoraClient
    private let urlSession = URLSession(configuration: .default)
    private var socket: URLSessionWebSocketTask?
    private var clock = PartyClock()
    private var loops: [Task<Void, Never>] = []
    private var closed = false
    private var retry = 0
    private var sentBuffering = false
    /// States this viewer's own adopt messages still have to come back as (see `adopt`).
    private var adopting = 0
    /// A next episode this viewer asked for: older states (still on the previous one) are ignored.
    private var pendingItem: Int?

    init(code: String, client: KinoraClient) {
        self.code = code
        self.client = client
    }

    // MARK: - connection

    func connect() {
        guard !closed else { return }
        loops.forEach { $0.cancel() }
        loops = [
            Task { [weak self] in await self?.run() },
            Task { [weak self] in
                while !Task.isCancelled {
                    self?.send(.ping(client: PartyClock.localNow))
                    try? await Task.sleep(for: .seconds(5))
                }
            },
            Task { [weak self] in
                while !Task.isCancelled {
                    try? await Task.sleep(for: .seconds(1))
                    self?.sync(fromState: false)
                }
            },
        ]
        Task { webLink = await client.partyWebLink(code) }
    }

    /// Waits for the room's first state (the start position), at most a few seconds.
    func firstState() async -> PartyState? {
        for _ in 0..<60 where state == nil && !gone {
            try? await Task.sleep(for: .milliseconds(100))
        }
        return state
    }

    func leave() {
        guard !closed else { return }
        closed = true
        loops.forEach { $0.cancel() }
        loops = []
        socket?.cancel(with: .normalClosure, reason: nil)
        socket = nil
    }

    private func run() async {
        while !closed, !Task.isCancelled {
            guard let req = await client.partySocketRequest(code) else { gone = true; return }
            let ws = urlSession.webSocketTask(with: req)
            socket = ws
            ws.resume()
            sentBuffering = false
            var opened = false
            do {
                while !closed {
                    let msg = try await ws.receive()
                    if !opened {
                        opened = true
                        retry = 0
                        status = waitingText
                        send(.ping(client: PartyClock.localNow))
                        if let b = player?.waiting, b { send(.buffering(true)); sentBuffering = true }
                    }
                    switch msg {
                    case .string(let s): handle(Data(s.utf8))
                    case .data(let d): handle(d)
                    @unknown default: break
                    }
                }
            } catch {
                guard !closed else { return }
                // The handshake was refused: the room is gone or not for this profile.
                if let http = ws.response as? HTTPURLResponse, [403, 404].contains(http.statusCode) {
                    gone = true
                    status = http.statusCode == 404 ? "Ez a watch party már nem létezik." : "Ez a watch party nem érhető el ezzel a profillal."
                    return
                }
                status = "A kapcsolat megszakadt, újracsatlakozás…"
                let wait = min(8.0, 0.5 * pow(2, Double(retry)))
                retry += 1
                try? await Task.sleep(for: .seconds(wait))
            }
        }
    }

    private func send(_ c: PartyCommand) {
        guard let socket, socket.state == .running else { return }
        socket.send(.data(c.json)) { _ in }
    }

    private func handle(_ data: Data) {
        guard let e = PartyEvent.decode(data) else { return }
        switch e {
        case .hello(let id, let r):
            myId = id
            reactions = r
        case .pong(let c, let s):
            clock.pong(client: c, server: s, now: PartyClock.localNow)
        case .state(let st):
            onState(st)
        case .chat(let name, let color, let text):
            add(Line(name: name, color: color, text: text))
        case .system(let text):
            add(Line(text: text))
        case .react(let name, _, let emoji):
            let f = Floating(emoji: emoji, name: name, x: .random(in: 0.1...0.8))
            floating.append(f)
            Task { [weak self] in
                try? await Task.sleep(for: .seconds(2.7))
                self?.floating.removeAll { $0.id == f.id }
            }
        }
    }

    private func add(_ line: Line) {
        lines.append(line)
        if lines.count > 60 { lines.removeFirst(lines.count - 60) }
        if !panelOpen, line.name != nil { unread += 1 }
    }

    private var waitingText: String? {
        guard let st = state, st.intended, !st.waiting.isEmpty else { return nil }
        return "Várunk rá: " + st.waiting.joined(separator: ", ")
    }

    private func onState(_ st: PartyState) {
        if adopting > 0, st.by == myId { adopting -= 1 }
        if let p = pendingItem {
            guard st.itemId == p else { return }
            pendingItem = nil
        }
        state = st
        let joined = members.count != st.members.count
        members = st.members
        if joined { player?.refreshPartyActivity() }
        if !gone { status = waitingText }
        guard let player else { return }
        if st.itemId != player.item.id {
            player.partySwitch(to: st.itemId)
            return
        }
        sync(fromState: true)
    }

    /// Moves the player to the room's clock (on every state, and once a second).
    func sync(fromState: Bool) {
        guard let st = state, let player, st.itemId == player.item.id, pendingItem == nil, adopting == 0,
              !player.loading, player.error == nil else { return }
        let target = st.position(at: clock.serverNow(PartyClock.localNow))
        let d = PartySync.decide(target: target, current: player.livePosition, playing: st.playing, fromState: fromState)
        player.partyApply(playing: st.playing, seek: d.seek, rate: d.rate)
    }

    // MARK: - from the player and the panel

    /// The host brings the room to where it already is (a party started from the player,
    /// or from the detail page with a resume position). The room's states until then are
    /// older than that and are not applied.
    func adopt(position: Double, playing: Bool) {
        adopting = playing ? 2 : 1
        send(.seek(to: position))
        if playing { send(.play) }
        Task { [weak self] in
            try? await Task.sleep(for: .seconds(4))
            self?.adopting = 0
        }
    }

    func played() { send(.play) }
    func paused(at pos: Double) { send(.pause(at: pos)) }
    func seeked(to pos: Double) { send(.seek(to: pos)) }
    func skipped(to pos: Double, kind: Segment.Kind) { send(.skip(to: pos, kind: kind)) }

    func nextItem(_ id: Int) {
        pendingItem = id
        send(.item(id))
        // Refused (someone may not see that title): stop waiting and follow the room again.
        Task { [weak self] in
            try? await Task.sleep(for: .seconds(6))
            if self?.pendingItem == id { self?.pendingItem = nil }
        }
    }

    func buffering(_ on: Bool) {
        guard on != sentBuffering else { return }
        sentBuffering = on
        send(.buffering(on))
    }

    func setEveryone(_ on: Bool) { send(.everyone(on)) }

    func chat(_ text: String) {
        let t = text.trimmingCharacters(in: .whitespacesAndNewlines)
        guard !t.isEmpty else { return }
        send(.chat(String(t.prefix(500))))
    }

    func react(_ emoji: String) { send(.react(emoji)) }
}

extension Color {
    /// A member's colour as the server sends it ("#e0a030"); amber when it cannot be read.
    init(partyHex s: String) {
        let hex = s.hasPrefix("#") ? String(s.dropFirst()) : s
        guard hex.count == 6, let v = UInt32(hex, radix: 16) else { self = Color(red: 0.94, green: 0.71, blue: 0.30); return }
        self = Color(red: Double(v >> 16 & 0xFF) / 255, green: Double(v >> 8 & 0xFF) / 255, blue: Double(v & 0xFF) / 255)
    }
}
