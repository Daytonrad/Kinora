import Foundation

/// Who may see a title saved on the device. Every profile sees only its own downloads (offline too, where no
/// PIN or rating check can reach the server): the server allowed the title to that profile when it was
/// downloaded. Titles saved before profiles were recorded (`owners == nil`) are shown to grown-up profiles only.
public enum OfflineAccess {
    public static func canSee(owners: [Int]?, profileID: Int?, isKids: Bool) -> Bool {
        guard let profileID else { return false }
        guard let owners else { return !isKids }
        return owners.contains(profileID)
    }

    /// The owners after a profile deletes the title: nil means the file itself can go.
    public static func afterDelete(owners: [Int]?, profileID: Int?) -> [Int]? {
        guard let owners, let profileID else { return nil }
        let rest = owners.filter { $0 != profileID }
        return rest.isEmpty ? nil : rest
    }
}
