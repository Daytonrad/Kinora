import Foundation
import Capacitor
import UIKit

/// Native helpers for the Kinora web UI on iPhone/iPad.
///
/// LAN discovery: receiving UDP broadcast replies needs Apple's restricted multicast
/// entitlement on iOS 14+, so the iOS plugin finds servers by probing the device's own
/// /24 subnet over HTTP (GET :8096/api/status) instead. Picture-in-picture is handled
/// by WKWebView itself (webkitSetPresentationMode), so enterPip only reports that.
@objc(KinoraNativePlugin)
public class KinoraNativePlugin: CAPPlugin, CAPBridgedPlugin {
    public let identifier = "KinoraNativePlugin"
    public let jsName = "KinoraNative"
    public let pluginMethods: [CAPPluginMethod] = [
        CAPPluginMethod(name: "info", returnType: CAPPluginReturnPromise),
        CAPPluginMethod(name: "discover", returnType: CAPPluginReturnPromise),
        CAPPluginMethod(name: "enterPip", returnType: CAPPluginReturnPromise),
        CAPPluginMethod(name: "keepAwake", returnType: CAPPluginReturnPromise)
    ]

    @objc func info(_ call: CAPPluginCall) {
        DispatchQueue.main.async {
            call.resolve(["tv": false, "model": UIDevice.current.model, "ip": Self.localIPv4() ?? ""])
        }
    }

    @objc func keepAwake(_ call: CAPPluginCall) {
        let on = call.getBool("on") ?? false
        DispatchQueue.main.async {
            UIApplication.shared.isIdleTimerDisabled = on
            call.resolve()
        }
    }

    @objc func enterPip(_ call: CAPPluginCall) {
        call.reject("A kép a képben módot a lejátszó kezeli (webkitSetPresentationMode).")
    }

    @objc func discover(_ call: CAPPluginCall) {
        guard let ip = Self.localIPv4() else {
            call.resolve(["servers": []])
            return
        }
        let prefix = ip.split(separator: ".").prefix(3).joined(separator: ".")
        let config = URLSessionConfiguration.ephemeral
        config.timeoutIntervalForRequest = 1.2
        config.httpMaximumConnectionsPerHost = 2
        let session = URLSession(configuration: config)
        let group = DispatchGroup()
        let lock = NSLock()
        var found: [[String: Any]] = []
        var seen = Set<String>()
        for host in 1...254 {
            guard let url = URL(string: "http://\(prefix).\(host):8096/api/status") else { continue }
            group.enter()
            session.dataTask(with: url) { data, _, _ in
                defer { group.leave() }
                guard let data = data,
                      let j = try? JSONSerialization.jsonObject(with: data) as? [String: Any],
                      (j["product"] as? String) == "kinora" else { return }
                let id = (j["server_id"] as? String) ?? url.host ?? ""
                lock.lock()
                if !seen.contains(id) {
                    seen.insert(id)
                    found.append(["id": id, "name": j["server_name"] as? String ?? "Kinora",
                                  "address": "http://\(prefix).\(host):8096", "version": j["version"] as? String ?? ""])
                }
                lock.unlock()
            }.resume()
        }
        group.notify(queue: .main) {
            session.invalidateAndCancel()
            call.resolve(["servers": found])
        }
    }

    /// The Wi-Fi (en0) IPv4 address, falling back to any non-loopback interface.
    static func localIPv4() -> String? {
        var ifaddr: UnsafeMutablePointer<ifaddrs>?
        guard getifaddrs(&ifaddr) == 0, let first = ifaddr else { return nil }
        defer { freeifaddrs(ifaddr) }
        var fallback: String?
        for ptr in sequence(first: first, next: { $0.pointee.ifa_next }) {
            let ifa = ptr.pointee
            guard let addr = ifa.ifa_addr, addr.pointee.sa_family == UInt8(AF_INET) else { continue }
            let name = String(cString: ifa.ifa_name)
            var host = [CChar](repeating: 0, count: Int(NI_MAXHOST))
            getnameinfo(addr, socklen_t(addr.pointee.sa_len), &host, socklen_t(host.count), nil, 0, NI_NUMERICHOST)
            let ip = String(cString: host)
            if ip.hasPrefix("127.") { continue }
            if name == "en0" { return ip }
            if fallback == nil { fallback = ip }
        }
        return fallback
    }
}
