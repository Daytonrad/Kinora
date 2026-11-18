import CoreSpotlight
import KinoraKit
import UniformTypeIdentifiers

/// Movies and series the user came across show up in the system search; a hit opens
/// kinora://item/<id>. Episodes are left out, their series stands for them.
enum Spotlight {
    private static let domain = "kinora.items"

    static func index(_ items: [Item]) {
        let entries = items.filter { $0.kind == .movie || $0.kind == .series }.map { item in
            let attrs = CSSearchableItemAttributeSet(contentType: .movie)
            attrs.title = item.title
            attrs.displayName = item.title
            attrs.contentDescription = [item.year.map(String.init), item.genres?.prefix(3).joined(separator: ", "), item.overview]
                .compactMap { $0 }.filter { !$0.isEmpty }.joined(separator: " · ")
            attrs.keywords = [item.kind == .series ? L("sorozat") : L("film")] + (item.genres ?? [])
            return CSSearchableItem(uniqueIdentifier: "kinora://item/\(item.id)", domainIdentifier: domain, attributeSet: attrs)
        }
        guard !entries.isEmpty else { return }
        CSSearchableIndex.default().indexSearchableItems(entries)
    }

    /// Signed out or switched server: the old titles would lead nowhere.
    static func clear() {
        CSSearchableIndex.default().deleteSearchableItems(withDomainIdentifiers: [domain])
    }
}
