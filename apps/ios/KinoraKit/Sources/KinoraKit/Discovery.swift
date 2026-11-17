import Foundation
#if canImport(Darwin)
import Darwin
#endif

/// Finding and checking Kinora servers.
public enum Discovery {

    /// Asks base/api/status. Succeeds only for a Kinora server; returns the base URL the
    /// answer really came from (a reverse proxy may redirect http → https).
    public static func probe(_ base: String, timeout: TimeInterval = 4, session: URLSession = .kinora) async throws -> (url: String, status: ServerStatus) {
        guard let url = URL(string: base + "/api/status") else { throw KinoraError.network }
        var req = URLRequest(url: url, cachePolicy: .reloadIgnoringLocalCacheData, timeoutInterval: timeout)
        req.httpMethod = "GET"
        let (data, response): (Data, URLResponse)
        do { (data, response) = try await session.data(for: req) }
        catch { throw KinoraError.network }
        guard let http = response as? HTTPURLResponse, http.statusCode == 200 else { throw KinoraError.network }
        guard let status = try? JSONDecoder.kinora.decode(ServerStatus.self, from: data),
              status.product == "kinora" || status.setupNeeded != nil else {
            throw KinoraError.notKinora(base)
        }
        return (ServerAddress.base(fromResponse: http.url?.absoluteString, fallback: base), status)
    }

    /// Tries every candidate address for what the user typed; returns the first Kinora server.
    public static func resolve(_ input: String, session: URLSession = .kinora) async throws -> (url: String, status: ServerStatus) {
        let candidates = ServerAddress.candidates(input)
        guard let first = candidates.first else { throw KinoraError.unreachable(input, isPrivate: false) }
        var sawSomething = false
        for url in candidates {
            do {
                let r = try await probe(url, session: session)
                if let bad = r.status.incompatibility { throw bad }
                if r.status.setupNeeded == true { throw KinoraError.setupNeeded }
                return r
            } catch let e as KinoraError {
                switch e {
                case .notKinora: sawSomething = true
                case .network: continue
                default: throw e
                }
            }
        }
        if sawSomething { throw KinoraError.notKinora(input) }
        throw KinoraError.unreachable(input, isPrivate: ServerAddress.isPrivateHost(ServerAddress.host(of: first)))
    }

    /// Finds a reachable address of a saved server (home network first, then its public
    /// address). Only an answer from the same server counts. Returns the updated server.
    public static func reach(_ server: SavedServer, session: URLSession = .kinora) async -> SavedServer? {
        for address in server.addresses {
            guard let r = try? await probe(address, timeout: address == server.url ? 5 : 4, session: session) else { continue }
            if let sid = r.status.serverId, sid != server.id { continue } // another server answered
            var s = server
            s.urls = ServerAddress.merge([r.url], server.urls, [server.url, r.status.publicUrl])
            s.url = r.url
            return s
        }
        return nil
    }

    /// Scans the local /24 network for Kinora servers on the default port.
    /// (UDP broadcast would need Apple's multicast entitlement, plain HTTP does not.)
    /// Calls `found` as servers answer; returns when the scan is done.
    public static func scanLocalNetwork(timeout: TimeInterval = 1.2, concurrency: Int = 48,
                                        session: URLSession = .kinora,
                                        found: @escaping @Sendable (DiscoveredServer) async -> Void) async {
        let prefixes: [String]
        if let ip = localIPv4(), let dot = ip.lastIndex(of: ".") {
            prefixes = [String(ip[...dot])]
        } else {
            prefixes = ["192.168.1.", "192.168.0."]
        }
        let hosts = prefixes.flatMap { p in (1...254).map { "http://\(p)\($0):\(KinoraProtocol.defaultPort)" } }
        await withTaskGroup(of: DiscoveredServer?.self) { group in
            var next = 0
            func addNext() {
                guard next < hosts.count else { return }
                let base = hosts[next]
                next += 1
                group.addTask {
                    guard let r = try? await probe(base, timeout: timeout, session: session) else { return nil }
                    return DiscoveredServer(id: r.status.serverId ?? r.url, name: r.status.serverName,
                                            address: r.url, version: r.status.version)
                }
            }
            for _ in 0..<min(concurrency, hosts.count) { addNext() }
            var seen = Set<String>()
            while let result = await group.next() {
                if let s = result, seen.insert(s.id).inserted { await found(s) }
                if Task.isCancelled { group.cancelAll(); return }
                addNext()
            }
        }
    }

    /// The device's IPv4 address on Wi-Fi/Ethernet (en0 preferred), nil when offline.
    public static func localIPv4() -> String? {
        var ifaddr: UnsafeMutablePointer<ifaddrs>?
        guard getifaddrs(&ifaddr) == 0, let first = ifaddr else { return nil }
        defer { freeifaddrs(ifaddr) }
        var fallback: String?
        for ptr in sequence(first: first, next: { $0.pointee.ifa_next }) {
            let ifa = ptr.pointee
            guard let addr = ifa.ifa_addr, addr.pointee.sa_family == UInt8(AF_INET) else { continue }
            let flags = Int32(ifa.ifa_flags)
            guard flags & IFF_UP != 0, flags & IFF_LOOPBACK == 0 else { continue }
            var host = [CChar](repeating: 0, count: Int(NI_MAXHOST))
            guard getnameinfo(addr, socklen_t(addr.pointee.sa_len), &host, socklen_t(host.count), nil, 0, NI_NUMERICHOST) == 0 else { continue }
            let ip = String(decoding: host.prefix { $0 != 0 }.map { UInt8(bitPattern: $0) }, as: UTF8.self)
            let name = String(cString: ifa.ifa_name)
            if name == "en0" { return ip }
            if fallback == nil, name.hasPrefix("en") { fallback = ip }
        }
        return fallback
    }
}
