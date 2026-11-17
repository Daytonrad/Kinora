import SwiftUI
import UIKit
import KinoraKit

/// Artwork in memory. Images need the Bearer token, so they are fetched through the
/// client instead of AsyncImage. Items without artwork are remembered and not asked again.
@MainActor
final class ArtworkCache {
    static let shared = ArtworkCache()

    private let images = NSCache<NSString, UIImage>()
    private var missing = Set<String>()
    private var inflight: [String: Task<UIImage?, Never>] = [:]

    private init() { images.countLimit = 400 }

    private func key(_ server: String, _ id: Int, _ kind: ImageKind) -> String { "\(server)/\(id)/\(kind.rawValue)" }

    func cached(server: String, id: Int, kind: ImageKind) -> UIImage? {
        images.object(forKey: key(server, id, kind) as NSString)
    }

    func load(client: KinoraClient, server: String, id: Int, kind: ImageKind) async -> UIImage? {
        let k = key(server, id, kind)
        if let img = images.object(forKey: k as NSString) { return img }
        if missing.contains(k) { return nil }
        if let running = inflight[k] { return await running.value }

        let task = Task<UIImage?, Never> { [weak self] in
            do {
                let data = try await client.image(id, kind)
                guard let img = UIImage(data: data) else { return nil }
                return await img.byPreparingForDisplay() ?? img
            } catch KinoraError.http(404, _) {
                self?.missing.insert(k)
                return nil
            } catch {
                return nil // offline or cancelled: try again next time
            }
        }
        inflight[k] = task
        let img = await task.value
        inflight[k] = nil
        if let img { images.setObject(img, forKey: k as NSString) }
        return img
    }
}

/// Poster, backdrop or episode still with a quiet placeholder while loading or when missing.
struct ArtworkView: View {
    @Environment(AppModel.self) private var model
    var item: Item
    var kind: ImageKind = .poster
    @State private var image: UIImage?

    var body: some View {
        Rectangle()
            .fill(Placeholder.gradient(for: item))
            .overlay {
                if let image {
                    Image(uiImage: image).resizable().scaledToFill().transition(.opacity)
                } else {
                    Text(item.title)
                        .font(Theme.display(kind == .poster ? 18 : 22, weight: .bold))
                        .foregroundStyle(Theme.plaster.opacity(0.85))
                        .multilineTextAlignment(.center)
                        .lineLimit(3)
                        .minimumScaleFactor(0.6)
                        .padding(10)
                }
            }
            .clipped()
            .accessibilityHidden(true)
            .task(id: "\(item.id)/\(kind.rawValue)") { await load() }
    }

    private func load() async {
        guard let client = model.client, let server = model.server?.id else { return }
        if let hit = ArtworkCache.shared.cached(server: server, id: item.id, kind: kind) { image = hit; return }
        let img = await ArtworkCache.shared.load(client: client, server: server, id: item.id, kind: kind)
        withAnimation(.easeOut(duration: 0.2)) { image = img }
    }
}

private enum Placeholder {
    private static let tones: [Color] = [
        Color(hex: 0x3A2E26), Color(hex: 0x2F3433), Color(hex: 0x3B3226), Color(hex: 0x2C2A33), Color(hex: 0x33292B),
    ]

    static func gradient(for item: Item) -> LinearGradient {
        let base = tones[abs(item.seriesId ?? item.id) % tones.count]
        return LinearGradient(colors: [base, Theme.panel], startPoint: .topLeading, endPoint: .bottomTrailing)
    }
}

// MARK: - cards

/// 2:3 poster with title and a second line (year or episode).
struct PosterCard: View {
    var item: Item
    var width: CGFloat? = 120

    var body: some View {
        VStack(alignment: .leading, spacing: 6) {
            ArtworkView(item: item, kind: .poster)
                .aspectRatio(2 / 3, contentMode: .fit)
                .overlay(alignment: .bottom) { ProgressStripe(item: item) }
                .overlay(alignment: .topTrailing) { WatchedBadge(item: item) }
                .clipShape(.rect(cornerRadius: 8))
            Text(item.title).font(.subheadline.weight(.semibold)).lineLimit(1)
            Text(Format.subtitle(item)).font(.caption).foregroundStyle(Theme.muted).lineLimit(1)
        }
        .frame(width: width)
        .accessibilityElement(children: .combine)
    }
}

/// 16:9 card for continue-watching style rows and episodes.
struct WideCard: View {
    var item: Item
    var width: CGFloat = 240

    var body: some View {
        VStack(alignment: .leading, spacing: 6) {
            ArtworkView(item: item, kind: item.kind == .episode ? .thumb : .backdrop)
                .aspectRatio(16 / 9, contentMode: .fit)
                .overlay(alignment: .bottom) { ProgressStripe(item: item) }
                .overlay(alignment: .topTrailing) { WatchedBadge(item: item) }
                .clipShape(.rect(cornerRadius: 8))
            Text(item.kind == .episode ? (item.seriesTitle ?? item.title) : item.title)
                .font(.subheadline.weight(.semibold)).lineLimit(1)
            Text(item.kind == .episode ? [item.episodeLabel, item.title].compactMap { $0 }.joined(separator: " · ") : Format.subtitle(item))
                .font(.caption).foregroundStyle(Theme.muted).lineLimit(1)
        }
        .frame(width: width)
        .accessibilityElement(children: .combine)
    }
}

struct ProgressStripe: View {
    var item: Item
    var body: some View {
        if let p = item.progress {
            GeometryReader { g in
                ZStack(alignment: .leading) {
                    Rectangle().fill(.black.opacity(0.55))
                    Rectangle().fill(Theme.amber).frame(width: g.size.width * p)
                }
            }
            .frame(height: 4)
            .accessibilityLabel("\(Int(p * 100)) százalék megnézve")
        }
    }
}

struct WatchedBadge: View {
    var item: Item
    var body: some View {
        if item.userData?.played == true {
            Image(systemName: "checkmark")
                .font(.caption2.bold())
                .foregroundStyle(Theme.amberInk)
                .padding(5)
                .background(Theme.amber, in: .circle)
                .padding(6)
                .accessibilityLabel("Megnézve")
        } else if let n = item.unplayedCount, n > 0, item.kind == .series || item.kind == .season {
            Text("\(n)")
                .font(.caption2.bold().monospacedDigit())
                .foregroundStyle(Theme.amberInk)
                .padding(.horizontal, 7).padding(.vertical, 3)
                .background(Theme.amber, in: .capsule)
                .padding(6)
                .accessibilityLabel("\(n) nem látott rész")
        }
    }
}

// MARK: - helpers

enum Format {
    /// "1 ó 32 perc", "45 perc"
    static func runtime(_ seconds: Double?) -> String? {
        guard let s = seconds, s > 0 else { return nil }
        let minutes = max(1, Int((s / 60).rounded()))
        let h = minutes / 60, m = minutes % 60
        if h == 0 { return "\(m) perc" }
        return m == 0 ? "\(h) óra" : "\(h) ó \(m) perc"
    }

    static func subtitle(_ item: Item) -> String {
        switch item.kind {
        case .series:
            if let n = item.episodeCount { return "\(n) rész" }
            return "Sorozat"
        case .season:
            return item.episodeCount.map { "\($0) rész" } ?? ""
        case .episode:
            return item.episodeLabel ?? ""
        default:
            return item.year.map(String.init) ?? ""
        }
    }

    /// "2019 · 1 ó 32 perc · PG-13 · ★ 7.9"
    static func meta(_ item: Item) -> String {
        var parts: [String] = []
        if let y = item.year { parts.append(String(y)) }
        if item.kind == .movie || item.kind == .episode, let r = runtime(item.runtimeSec) { parts.append(r) }
        if let c = item.contentRating, !c.isEmpty { parts.append(c) }
        if let r = item.communityRating, r > 0 { parts.append("★ " + r.formatted(.number.precision(.fractionLength(1)))) }
        if let w = item.width, let h = item.height { parts.append(resolution(w, h)) }
        return parts.joined(separator: " · ")
    }

    private static func resolution(_ w: Int, _ h: Int) -> String {
        switch max(w, h * 16 / 9) {
        case 3200...: "4K"
        case 1800...: "1080p"
        case 1200...: "720p"
        default: "SD"
        }
    }
}

/// Spinner, error with retry, or the content.
struct LoadingState: View {
    var error: String?
    var retry: () -> Void

    var body: some View {
        VStack(spacing: 14) {
            if let error {
                Image(systemName: "wifi.exclamationmark").font(.largeTitle).foregroundStyle(Theme.muted)
                Text(error).multilineTextAlignment(.center).foregroundStyle(Theme.plaster)
                Button("Újra", action: retry).buttonStyle(.bordered)
            } else {
                ProgressView().controlSize(.large)
            }
        }
        .padding(40)
        .frame(maxWidth: .infinity, minHeight: 300)
    }
}

/// Profile switch / server / logout, in every tab's toolbar.
struct AccountMenu: View {
    @Environment(AppModel.self) private var model

    var body: some View {
        Menu {
            if let p = model.me?.profile {
                Text(p.name)
            }
            Button("Profilváltás", systemImage: "person.2") { model.switchProfile() }
            Button("Másik szerver", systemImage: "server.rack") { model.showServers() }
            Button("Kijelentkezés", systemImage: "rectangle.portrait.and.arrow.right", role: .destructive) {
                Task { await model.logout() }
            }
        } label: {
            if let p = model.me?.profile {
                ProfileFace(profile: p, size: 30, showsName: false)
            } else {
                Image(systemName: "person.crop.circle")
            }
        }
        .accessibilityLabel("Fiók")
    }
}

extension View {
    /// Shared navigation destinations for the browse stacks.
    func browseDestinations() -> some View {
        navigationDestination(for: Item.self) { ItemDetailView(item: $0) }
            .navigationDestination(for: Library.self) { LibraryView(library: $0) }
    }

    func kinoraScreen() -> some View {
        background(Theme.black)
            .toolbarBackground(Theme.black, for: .navigationBar)
    }
}
