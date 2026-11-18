import Foundation
import KinoraKit

/// A title saved on the device for offline viewing.
struct SavedDownload: Codable, Identifiable, Equatable {
    var item: Item
    var quality: String
    var size: Int64
    var savedAt: Date
    /// Subtitle tracks; `url` is the file name next to the video.
    var subtitles: [SubtitleTrack]
    var segments: [Segment]?
    var chapters: [Chapter]?
    var id: Int { item.id }
}

/// Offline downloads: the server transcodes to MP4 first, then the file comes over a
/// background URLSession, so it continues while the app is suspended.
@MainActor @Observable
final class DownloadStore {
    enum Phase: Equatable {
        var isFailed: Bool { if case .failed = self { true } else { false } }
        var isPreparingPhase: Bool { if case .preparing = self { true } else { false } }

        /// The server is preparing the file (0...1).
        case preparing(Double)
        /// The file is coming to the device (0...1).
        case transferring(Double)
        case failed(String)
    }

    /// A download in progress; kept on disk so it survives a restart.
    struct Pending: Codable {
        var item: Item
        var key: String
        var quality: String
        var subtitles: [SubtitleTrack] = []
        var segments: [Segment]?
        var chapters: [Chapter]?
    }

    static let shared = DownloadStore()

    private(set) var saved: [SavedDownload] = []
    private(set) var phases: [Int: Phase] = [:] { didSet { phasesChanged(from: oldValue) } }
    private var pending: [Int: Pending] = [:]
    private var client: KinoraClient?
    private var poller: Task<Void, Never>?
    private let session: URLSession
    private let bridge = SessionBridge()

    let dir: URL

    private init() {
        let base = FileManager.default.urls(for: .applicationSupportDirectory, in: .userDomainMask)[0]
        var d = base.appendingPathComponent("Downloads", isDirectory: true)
        try? FileManager.default.createDirectory(at: d, withIntermediateDirectories: true)
        var values = URLResourceValues()
        values.isExcludedFromBackup = true
        try? d.setResourceValues(values)
        dir = d
        let config = URLSessionConfiguration.background(withIdentifier: "kinora.downloads")
        config.sessionSendsLaunchEvents = true
        config.isDiscretionary = false
        config.allowsCellularAccess = true
        session = URLSession(configuration: config, delegate: bridge, delegateQueue: nil)
        bridge.dir = d
        bridge.store = self
        load()
        session.getAllTasks { tasks in
            let running = Set(tasks.compactMap { $0.taskDescription.flatMap(Int.init) })
            Task { @MainActor in self.resume(running: running) }
        }
    }

    // MARK: - queries

    func download(for id: Int) -> SavedDownload? { saved.first { $0.id == id } }

    func fileURL(_ d: SavedDownload) -> URL { dir.appendingPathComponent("\(d.id).mp4") }

    /// Playback info for a saved title, with subtitles pointing at the local files.
    func localInfo(for item: Item) -> PlaybackInfo? {
        guard let d = download(for: item.id), FileManager.default.fileExists(atPath: fileURL(d).path) else { return nil }
        let subs = d.subtitles.map { t in
            var t = t
            t.url = dir.appendingPathComponent(t.url).absoluteString
            return t
        }
        return .local(url: fileURL(d).absoluteString, item: d.item, subtitles: subs, segments: d.segments, chapters: d.chapters)
    }

    func pendingTitle(_ id: Int) -> String {
        guard let i = pending[id]?.item else { return L("Letöltés") }
        return [i.seriesTitle, i.episodeLabel, i.title].compactMap { $0 }.joined(separator: " · ")
    }

    var usedBytes: Int64 { saved.reduce(0) { $0 + $1.size } }

    // MARK: - actions

    /// Signed in: polling of server-side jobs can continue.
    func attach(_ client: KinoraClient) {
        self.client = client
        if pending.values.contains(where: { phases[$0.item.id].isPreparing }) { poll() }
    }

    /// Why a new download can't start: the space limit is reached or the device is almost full.
    func roomError() -> String? {
        let limit = Int64(AppSettings.downloadLimitGB) * 1_000_000_000
        if limit > 0, usedBytes >= limit {
            return L("Elérted a letöltésekre beállított {0} GB-os keretet. Törölj valamit, vagy emeld a keretet a beállításokban.", AppSettings.downloadLimitGB)
        }
        let free = (try? dir.resourceValues(forKeys: [.volumeAvailableCapacityForImportantUsageKey]))?
            .volumeAvailableCapacityForImportantUsage ?? .max
        if free < 1_500_000_000 { return L("Kevés a szabad hely az eszközön (1,5 GB alatt).") }
        return nil
    }

    func start(_ item: Item, quality: String, client: KinoraClient) async throws {
        if let room = roomError() { throw DownloadError(message: room) }
        self.client = client
        let job = try await client.createDownload(item.id, quality: quality)
        var p = Pending(item: item, key: job.key, quality: quality)
        p.subtitles = (try? await client.subtitleStreams(item.id)) ?? []
        // Skip markers and chapters for offline use; a stream session the request opened is stopped.
        if let info = try? await client.playback(item.id, PlaybackRequest(start: 0)) {
            if let s = info.session { try? await client.stopStream(s) }
            p.segments = info.segments
            p.chapters = info.chapters
        }
        pending[item.id] = p
        persist()
        if job.isDone { transfer(p) } else { phases[item.id] = .preparing(job.progress ?? 0); poll() }
    }

    /// Queues every episode of a season that is not saved or on its way yet; returns how many started.
    @discardableResult
    func startAll(_ episodes: [Item], quality: String, client: KinoraClient) async throws -> Int {
        var started = 0
        for ep in episodes where download(for: ep.id) == nil && phases[ep.id] == nil {
            do { try await start(ep, quality: quality, client: client); started += 1 }
            catch where started > 0 { break }
        }
        return started
    }

    /// Watched to the end: removed when the user asked for that.
    func watched(_ id: Int) {
        if AppSettings.deleteWatchedDownloads { delete(id) }
    }

    func cancel(_ id: Int) {
        session.getAllTasks { tasks in
            tasks.filter { $0.taskDescription == String(id) }.forEach { $0.cancel() }
        }
        pending[id] = nil
        phases[id] = nil
        persist()
    }

    func delete(_ id: Int) {
        guard let d = download(for: id) else { return }
        try? FileManager.default.removeItem(at: fileURL(d))
        for t in d.subtitles { try? FileManager.default.removeItem(at: dir.appendingPathComponent(t.url)) }
        saved.removeAll { $0.id == id }
        persist()
    }

    // MARK: - server side

    private func poll() {
        guard poller == nil else { return }
        poller = Task { [weak self] in
            while let self, !Task.isCancelled {
                let waiting = self.pending.values.filter { self.phases[$0.item.id].isPreparing }
                if waiting.isEmpty { break }
                if let c = self.client, let list = try? await c.downloads() {
                    for p in waiting {
                        guard let job = list.jobs.first(where: { $0.key == p.key }) else {
                            self.phases[p.item.id] = .failed(L("A szerver elvetette a letöltést."))
                            continue
                        }
                        if job.isDone { self.transfer(p) }
                        else if job.isFailed { self.phases[p.item.id] = .failed(job.error ?? L("Sikertelen átkódolás.")) }
                        else { self.phases[p.item.id] = .preparing(job.progress ?? 0) }
                    }
                }
                try? await Task.sleep(for: .seconds(3))
            }
            self?.poller = nil
        }
    }

    private func transfer(_ p: Pending) {
        guard let client else { return }
        phases[p.item.id] = .transferring(0)
        Task {
            guard let req = await client.downloadRequest(p.key) else {
                phases[p.item.id] = .failed(L("A szerver nem érhető el."))
                return
            }
            // Subtitles are small: fetch them now with the session token.
            var subs: [SubtitleTrack] = []
            for t in p.subtitles {
                guard let data = try? await client.data(t.url) else { continue }
                let name = "\(p.item.id).\(t.idx).vtt"
                try? data.write(to: dir.appendingPathComponent(name))
                var local = t
                local.url = name
                subs.append(local)
            }
            pending[p.item.id]?.subtitles = subs
            persist()
            var request = req
            request.allowsCellularAccess = !AppSettings.downloadsWifiOnly
            let task = session.downloadTask(with: request)
            task.taskDescription = String(p.item.id)
            task.resume()
        }
    }

    private func resume(running: Set<Int>) {
        for (id, _) in pending where phases[id] == nil {
            // A transfer the system still runs, or a job to check again on the server.
            phases[id] = running.contains(id) ? .transferring(0) : .preparing(0)
        }
    }

    // MARK: - session events (from the bridge)

    fileprivate func progressed(_ id: Int, _ fraction: Double) {
        if pending[id] != nil { phases[id] = .transferring(fraction) }
    }

    fileprivate func finished(_ id: Int, size: Int64) {
        guard let p = pending[id] else { return }
        saved.removeAll { $0.id == id }
        saved.insert(SavedDownload(item: p.item, quality: p.quality, size: size, savedAt: .now,
                                   subtitles: p.subtitles, segments: p.segments, chapters: p.chapters), at: 0)
        pending[id] = nil
        phases[id] = nil
        persist()
    }

    fileprivate func failed(_ id: Int, _ message: String) {
        guard pending[id] != nil else { return }
        phases[id] = .failed(message)
    }

    // MARK: - Live Activity

    /// Counts what ended and shows the running downloads on the lock screen.
    private func phasesChanged(from old: [Int: Phase]) {
        for (id, was) in old where !was.isFailed {
            switch phases[id] {
            case .failed?: LiveActivities.downloadEnded(failed: true)
            case nil where download(for: id) != nil: LiveActivities.downloadEnded(failed: false)
            default: break
            }
        }
        let running = phases.filter { !$0.value.isFailed }.sorted { $0.key < $1.key }
        let front = running.first { if case .transferring = $0.value { true } else { false } } ?? running.first
        let total = running.reduce(0.0) { sum, e in
            switch e.value {
            // Preparing on the server counts as the first half, the transfer as the second.
            case .preparing(let f): sum + f * 0.5
            case .transferring(let f): sum + 0.5 + f * 0.5
            case .failed: sum
            }
        }
        LiveActivities.downloadsChanged(
            active: running.count,
            title: front.map { pendingTitle($0.key) } ?? "",
            fraction: running.isEmpty ? 1 : total / Double(running.count),
            preparing: running.allSatisfy { $0.value.isPreparingPhase })
    }

    // MARK: - persistence

    private var indexURL: URL { dir.appendingPathComponent("index.json") }
    private var pendingURL: URL { dir.appendingPathComponent("pending.json") }

    private func load() {
        saved = (try? Data(contentsOf: indexURL)).flatMap { try? JSONDecoder().decode([SavedDownload].self, from: $0) } ?? []
        pending = (try? Data(contentsOf: pendingURL)).flatMap { try? JSONDecoder().decode([Int: Pending].self, from: $0) } ?? [:]
    }

    private func persist() {
        try? JSONEncoder().encode(saved).write(to: indexURL, options: .atomic)
        try? JSONEncoder().encode(pending).write(to: pendingURL, options: .atomic)
    }
}

struct DownloadError: LocalizedError {
    var message: String
    var errorDescription: String? { message }
}

private extension Optional where Wrapped == DownloadStore.Phase {
    var isPreparing: Bool { if case .preparing? = self { true } else { false } }
}

/// URLSession delegate; the file must be moved before the callback returns.
private final class SessionBridge: NSObject, URLSessionDownloadDelegate, @unchecked Sendable {
    var dir: URL?
    weak var store: DownloadStore?

    func urlSession(_ session: URLSession, downloadTask: URLSessionDownloadTask, didWriteData _: Int64,
                    totalBytesWritten written: Int64, totalBytesExpectedToWrite total: Int64) {
        guard let id = downloadTask.taskDescription.flatMap(Int.init), total > 0 else { return }
        let f = Double(written) / Double(total)
        Task { @MainActor [weak store] in store?.progressed(id, f) }
    }

    func urlSession(_ session: URLSession, downloadTask: URLSessionDownloadTask, didFinishDownloadingTo location: URL) {
        guard let id = downloadTask.taskDescription.flatMap(Int.init), let dir else { return }
        let code = (downloadTask.response as? HTTPURLResponse)?.statusCode ?? 0
        guard code == 200 || code == 206 else {
            Task { @MainActor [weak store] in store?.failed(id, L("A szerver hibát adott ({0}).", code)) }
            return
        }
        let dest = dir.appendingPathComponent("\(id).mp4")
        try? FileManager.default.removeItem(at: dest)
        do {
            try FileManager.default.moveItem(at: location, to: dest)
            let size = (try? FileManager.default.attributesOfItem(atPath: dest.path)[.size] as? Int64) ?? 0
            Task { @MainActor [weak store] in store?.finished(id, size: size) }
        } catch {
            Task { @MainActor [weak store] in store?.failed(id, L("Nem sikerült menteni a fájlt.")) }
        }
    }

    func urlSession(_ session: URLSession, task: URLSessionTask, didCompleteWithError error: (any Error)?) {
        guard let error, let id = task.taskDescription.flatMap(Int.init) else { return }
        if (error as? URLError)?.code == .cancelled { return }
        Task { @MainActor [weak store] in store?.failed(id, L("Megszakadt a letöltés.")) }
    }
}
