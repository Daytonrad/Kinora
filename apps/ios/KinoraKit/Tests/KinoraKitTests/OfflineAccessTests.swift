import Testing
@testable import KinoraKit

@Suite struct OfflineAccessTests {
    @Test func ownProfileOnly() {
        #expect(OfflineAccess.canSee(owners: [1, 3], profileID: 1, isKids: false))
        #expect(OfflineAccess.canSee(owners: [1, 3], profileID: 3, isKids: true))
        #expect(!OfflineAccess.canSee(owners: [1], profileID: 2, isKids: false))
        #expect(!OfflineAccess.canSee(owners: [1], profileID: 2, isKids: true)) // a kids profile never sees another's download
        #expect(!OfflineAccess.canSee(owners: [1], profileID: nil, isKids: false))
    }

    @Test func olderDownloadsAreForGrownUps() {
        #expect(OfflineAccess.canSee(owners: nil, profileID: 1, isKids: false))
        #expect(!OfflineAccess.canSee(owners: nil, profileID: 2, isKids: true))
        #expect(!OfflineAccess.canSee(owners: nil, profileID: nil, isKids: false))
    }

    @Test func deleteKeepsTheFileForOthers() {
        #expect(OfflineAccess.afterDelete(owners: [1, 2], profileID: 1) == [2])
        #expect(OfflineAccess.afterDelete(owners: [1], profileID: 1) == nil)
        #expect(OfflineAccess.afterDelete(owners: nil, profileID: 1) == nil)
    }
}
