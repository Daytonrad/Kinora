#if os(iOS)
import UIKit
#endif

/// The app's vibrations, one vocabulary for every screen. iOS plays them only when the
/// system haptics are on; on Apple TV they do nothing.
///
/// - `tap`: a light knock for frequent controls (play/pause, ±10 s, reactions, chat send).
/// - `firm`: a heavier knock for a jump the viewer chose (skip intro, next episode, start a download).
/// - `select`: a tick for picking one of several (audio, subtitles, speed, quality, chapters, toggles).
/// - `success` / `warning` / `error`: outcomes (joined a party, a download finished; leaving or
///   deleting; a wrong PIN or a failed join).
///
/// Only what the viewer did vibrates: the watch party moving the player, remote commands from the
/// lock screen and automatic skips stay silent.
@MainActor
enum Haptic {
    case tap, firm, select, success, warning, error

    func play() {
        #if os(iOS)
        switch self {
        case .tap: Self.light.impactOccurred()
        case .firm: Self.medium.impactOccurred()
        case .select: Self.selection.selectionChanged()
        case .success: Self.notice.notificationOccurred(.success)
        case .warning: Self.notice.notificationOccurred(.warning)
        case .error: Self.notice.notificationOccurred(.error)
        }
        #endif
    }

    #if os(iOS)
    private static let light = UIImpactFeedbackGenerator(style: .light)
    private static let medium = UIImpactFeedbackGenerator(style: .medium)
    private static let selection = UISelectionFeedbackGenerator()
    private static let notice = UINotificationFeedbackGenerator()
    #endif
}
