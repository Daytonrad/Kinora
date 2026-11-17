import Foundation

/// Server address handling. Mirrors apps/shared/src/address.ts so every Kinora client
/// understands the same inputs ("192.168.1.10", "nas.local:9000", "https://mozi.pelda.hu").
public enum ServerAddress {

    /// True for IP literals, localhost and mDNS-style names: these live on the home network.
    public static func isLocalHost(_ host: String) -> Bool {
        let h = host.trimmingCharacters(in: CharacterSet(charactersIn: "[]")).lowercased()
        if h.wholeMatch(of: /\d{1,3}(\.\d{1,3}){3}/) != nil { return true }
        if h.contains(":") || h == "localhost" || !h.contains(".") { return true }
        return h.hasSuffix(".local") || h.hasSuffix(".lan") || h.hasSuffix(".home") || h.hasSuffix(".internal")
    }

    /// RFC 1918 / loopback / link-local / .local: reachable only inside the home network.
    public static func isPrivateHost(_ host: String) -> Bool {
        if let m = host.wholeMatch(of: /(\d+)\.(\d+)\.\d+\.\d+/), let a = Int(m.1), let b = Int(m.2) {
            return a == 10 || a == 127 || (a == 192 && b == 168) || (a == 172 && (16...31).contains(b)) || (a == 169 && b == 254)
        }
        let h = host.lowercased()
        return h.hasSuffix(".local") || h.hasSuffix(".lan") || h.hasSuffix(".home") || !h.contains(".")
    }

    /// Turns user input into base URLs to try, in order.
    /// "192.168.1.10" → http://192.168.1.10:8096, then :80, then https;
    /// "kinora.pelda.hu" → https first (reverse proxy), then http :8096 and :80.
    public static func candidates(_ input: String) -> [String] {
        var s = input.filter { !$0.isWhitespace }
        guard !s.isEmpty else { return [] }
        while s.hasSuffix("/") { s.removeLast() }
        guard !s.isEmpty else { return [] }

        var scheme = ""
        var rest = s
        if let m = s.wholeMatch(of: /(?i)(https?):\/\/(.+)/) {
            scheme = m.1.lowercased()
            rest = String(m.2)
        }
        let hostPort = String(rest.split(separator: "/", maxSplits: 1, omittingEmptySubsequences: false).first ?? "")
        let path = String(rest.dropFirst(hostPort.count))
        let hasPort = hostPort.wholeMatch(of: /\[.*\]:\d+/) != nil
            || (hostPort.wholeMatch(of: /[^:]+:\d+/) != nil)
        let port = KinoraProtocol.defaultPort

        if !scheme.isEmpty {
            if hasPort { return ["\(scheme)://\(rest)"] }
            return scheme == "https"
                ? ["https://\(rest)", "https://\(hostPort):\(port)\(path)"]
                : ["http://\(hostPort):\(port)\(path)", "http://\(rest)"]
        }
        let host = hasPort ? hostPort.replacing(/:\d+$/, with: "") : hostPort
        if hasPort {
            return isLocalHost(host)
                ? ["http://\(hostPort)\(path)", "https://\(hostPort)\(path)"]
                : ["https://\(hostPort)\(path)", "http://\(hostPort)\(path)"]
        }
        return isLocalHost(host)
            ? ["http://\(hostPort):\(port)\(path)", "http://\(hostPort)\(path)", "https://\(hostPort)\(path)"]
            : ["https://\(hostPort)\(path)", "http://\(hostPort):\(port)\(path)", "http://\(hostPort)\(path)"]
    }

    /// The base URL a /api/status answer really came from (after redirects such as http → https).
    public static func base(fromResponse responseURL: String?, fallback: String) -> String {
        guard let r = responseURL, let range = r.range(of: "/api/status"), range.lowerBound > r.startIndex else {
            return fallback
        }
        return String(r[..<range.lowerBound])
    }

    /// Merges address lists, most preferred first, without duplicates (at most 8).
    public static func merge(_ lists: [String?]...) -> [String] {
        var out: [String] = []
        for list in lists {
            for item in list {
                var v = (item ?? "").trimmingCharacters(in: .whitespaces)
                while v.hasSuffix("/") { v.removeLast() }
                if !v.isEmpty && !out.contains(v) { out.append(v) }
            }
        }
        return Array(out.prefix(8))
    }

    /// The host part of a base URL, for messages.
    public static func host(of base: String) -> String {
        URL(string: base)?.host() ?? base
    }
}
