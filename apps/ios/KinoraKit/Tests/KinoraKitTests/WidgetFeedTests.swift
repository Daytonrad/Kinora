import Foundation
import Testing
@testable import KinoraKit

struct WidgetFeedTests {
    private func item(_ id: Int, series: Int? = nil) -> Item {
        var i = Item(id: id, kind: series == nil ? .movie : .episode, title: "T\(id)")
        i.seriesId = series
        return i
    }

    private func row(_ id: String, _ items: [Item]) -> HomeRow {
        HomeRow(id: id, title: id, style: .poster, items: items)
    }

    @Test func continueMergesRowsOncePerSeries() {
        let home = HomeData(hero: nil, rows: [
            row("nextup", [item(7, series: 3), item(20, series: 9)]),
            row("continue", [item(8), item(12, series: 3)]),
            row("new-1", [item(99)]),
        ])
        #expect(WidgetFeed.continueWatching(home).map(\.id) == [8, 12, 20])
    }

    @Test func recentlyAddedInterleavesLibraries() {
        let home = HomeData(hero: nil, rows: [
            row("continue", [item(8)]),
            row("new-1", [item(1), item(2), item(3)]),
            row("new-2", [item(10), item(2)]),
        ])
        #expect(WidgetFeed.recentlyAdded(home).map(\.id) == [1, 10, 2, 3])
    }

    @Test func links() {
        #expect(KinoraLink(url: URL(string: "kinora://play/42")!) == .play(42))
        #expect(KinoraLink(url: URL(string: "KINORA://Item/7")!) == .item(7))
        #expect(KinoraLink(url: URL(string: "kinora://item/x")!) == nil)
        #expect(KinoraLink(url: URL(string: "https://item/7")!) == nil)
        #expect(KinoraLink.item(5).url.absoluteString == "kinora://item/5")
    }
}
