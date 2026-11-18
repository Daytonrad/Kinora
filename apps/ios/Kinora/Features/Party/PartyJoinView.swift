import SwiftUI
import UIKit
import KinoraKit

/// Join a room: point the camera at the host's QR code (on their phone or the TV), or type
/// the 6-character code. A recognised code joins at once; errors stay on this screen.
struct PartyJoinView: View {
    @Environment(AppModel.self) private var model
    @Environment(\.dismiss) private var dismiss
    @Environment(\.openURL) private var openURL
    @Environment(\.scenePhase) private var scenePhase

    @State private var code = ""
    @State private var camera = CameraAccess.current
    @State private var torch = false
    /// A code was read and is being joined: the camera holds still meanwhile.
    @State private var scanned: String?
    /// "Not a party code", for a moment, when the camera reads some other QR code.
    @State private var foreign: String?
    @State private var foreignTask: Task<Void, Never>?
    @State private var error: String?
    @FocusState private var typing: Bool

    private let hasCamera = QRScanner.available

    var body: some View {
        NavigationStack {
            ScrollView {
                VStack(spacing: 22) {
                    if hasCamera {
                        viewfinder
                        divider
                    }
                    codeField
                    if let error {
                        Label(error, systemImage: "exclamationmark.triangle.fill")
                            .font(.subheadline)
                            .foregroundStyle(Theme.danger)
                            .frame(maxWidth: .infinity, alignment: .leading)
                            .transition(.opacity)
                    }
                    Text(L("A kódot és a QR-kódot a party indítója látja: telefonon a lejátszó watch party paneljén, Apple TV-n a lejátszó jobb felső sarkában. A meghívó linkje is egyből ide hoz."))
                        .font(.footnote)
                        .foregroundStyle(Theme.muted)
                        .frame(maxWidth: .infinity, alignment: .leading)
                }
                .frame(maxWidth: 420)
                .padding(.horizontal, 20)
                .padding(.vertical, 16)
                .frame(maxWidth: .infinity)
                .animation(.easeOut(duration: 0.2), value: error)
            }
            .scrollDismissesKeyboard(.interactively)
            .background(Theme.black)
            .navigationTitle(L("Csatlakozás watch partyhoz"))
            .navigationBarTitleDisplayMode(.inline)
            .toolbar {
                ToolbarItem(placement: .cancellationAction) { Button(L("Mégse")) { dismiss() } }
                ToolbarItem(placement: .confirmationAction) {
                    if model.partyJoining { ProgressView() } else {
                        Button(L("Csatlakozás")) { join(code) }.disabled(PartySync.normalizedCode(code) == nil)
                    }
                }
            }
        }
        .presentationDetents(hasCamera ? [.large] : [.medium, .large])
        .preferredColorScheme(.dark)
        .onAppear {
            // Without a camera to look through, the keyboard is the way in.
            if !hasCamera || camera == .denied { typing = true }
        }
        .onChange(of: scenePhase) { _, phase in
            // Back from Settings with the camera allowed.
            if phase == .active { camera = CameraAccess.current }
        }
        .onChange(of: model.partyError) { _, e in
            guard let e else { return }
            model.partyError = nil
            error = e
            scanned = nil
        }
        .onChange(of: model.playerVisible) { _, on in if on { dismiss() } }
        .onDisappear { foreignTask?.cancel() }
    }

    // MARK: - camera

    private var viewfinder: some View {
        ZStack {
            switch camera {
            case .allowed:
                QRScanner(paused: scanned != nil || typing, torch: torch, onCode: read)
                    .accessibilityLabel(L("Kamera kép, QR-kód beolvasása"))
                ScanFrame(done: scanned != nil)
                    .padding(36)
                    .allowsHitTesting(false)
                VStack {
                    HStack {
                        Spacer()
                        if QRScanner.hasTorch {
                            Button { Haptic.tap.play(); torch.toggle() } label: {
                                Image(systemName: torch ? "flashlight.on.fill" : "flashlight.off.fill")
                                    .font(.system(size: 17, weight: .semibold))
                                    .frame(width: 44, height: 44)
                                    .background(.black.opacity(0.45), in: .circle)
                            }
                            .foregroundStyle(torch ? Theme.amber : .white)
                            .accessibilityLabel(torch ? L("Zseblámpa kikapcsolása") : L("Zseblámpa bekapcsolása"))
                        }
                    }
                    Spacer()
                    hint
                }
                .padding(12)
                if typing && scanned == nil {
                    // The keyboard is up: the camera rests until the field lets go.
                    Color.black.opacity(0.55)
                        .overlay {
                            Label(L("Koppints ide a beolvasáshoz"), systemImage: "qrcode.viewfinder")
                                .font(.subheadline.weight(.semibold))
                                .foregroundStyle(.white)
                        }
                        .onTapGesture { typing = false }
                        .accessibilityAddTraits(.isButton)
                }
            case .undecided:
                cameraCard(icon: "qrcode.viewfinder",
                           title: L("QR-kód beolvasása"),
                           text: L("Irányítsd a kamerát a házigazda telefonján vagy a tévén megjelenő kódra."),
                           button: L("Kamera bekapcsolása")) {
                    Task { camera = await CameraAccess.request(); if camera == .denied { typing = true } }
                }
            case .denied:
                cameraCard(icon: "camera.badge.ellipsis",
                           title: L("Nincs hozzáférés a kamerához"),
                           text: L("A QR-kód beolvasásához engedélyezd a kamerát a Beállításokban, vagy írd be lent a kódot."),
                           button: L("Beállítások megnyitása")) {
                    if let url = URL(string: UIApplication.openSettingsURLString) { openURL(url) }
                }
            }
        }
        .aspectRatio(1, contentMode: .fit)
        .frame(maxWidth: 360)
        .clipShape(.rect(cornerRadius: 24))
        .overlay(RoundedRectangle(cornerRadius: 24).strokeBorder(Theme.rule))
    }

    private var hint: some View {
        Group {
            if let scanned {
                Label(L("Csatlakozás: {0}", scanned), systemImage: "checkmark.circle.fill")
            } else if let foreign {
                Label(foreign, systemImage: "xmark.circle.fill")
            } else {
                Text(L("Irányítsd a kamerát a QR-kódra"))
            }
        }
        .font(.subheadline.weight(.semibold))
        .foregroundStyle(scanned != nil ? Theme.amber : .white)
        .padding(.horizontal, 14).padding(.vertical, 8)
        .background(.black.opacity(0.6), in: .capsule)
        .animation(.easeOut(duration: 0.2), value: scanned)
        .animation(.easeOut(duration: 0.2), value: foreign)
        .accessibilityAddTraits(.updatesFrequently)
    }

    private func cameraCard(icon: String, title: String, text: String, button: String,
                            action: @escaping () -> Void) -> some View {
        VStack(spacing: 14) {
            Image(systemName: icon)
                .font(.system(size: 52, weight: .light))
                .foregroundStyle(Theme.amber)
            Text(title).font(.headline).foregroundStyle(Theme.plasterBright)
            Text(text)
                .font(.subheadline)
                .foregroundStyle(Theme.muted)
                .multilineTextAlignment(.center)
            Button(button, action: action)
                .buttonStyle(.borderedProminent)
                .tint(Theme.amber)
                .foregroundStyle(Theme.amberInk)
                .padding(.top, 4)
        }
        .padding(28)
        .frame(maxWidth: .infinity, maxHeight: .infinity)
        .background(Theme.panel)
    }

    private func read(_ raw: String) {
        guard scanned == nil, !model.partyJoining else { return }
        guard let c = PartySync.code(scanned: raw) else {
            guard foreign == nil else { return }
            foreign = L("Ez nem watch party QR-kód")
            Haptic.warning.play()
            foreignTask?.cancel()
            foreignTask = Task {
                try? await Task.sleep(for: .seconds(2.5))
                if !Task.isCancelled { foreign = nil }
            }
            return
        }
        Haptic.firm.play()
        torch = false
        scanned = c
        code = c
        UIAccessibility.post(notification: .announcement, argument: L("Kód beolvasva: {0}. Csatlakozás.", c))
        join(c)
    }

    // MARK: - typing

    private var divider: some View {
        HStack(spacing: 12) {
            Rectangle().fill(Theme.rule).frame(height: 1)
            Text(L("vagy írd be a kódot")).font(.footnote).foregroundStyle(Theme.muted).fixedSize()
            Rectangle().fill(Theme.rule).frame(height: 1)
        }
    }

    /// Six cells over an invisible field: the cells show what is typed, the field takes the keys.
    private var codeField: some View {
        let chars = Array(code)
        return ZStack {
            TextField("", text: $code)
                .textInputAutocapitalization(.characters)
                .autocorrectionDisabled()
                .keyboardType(.asciiCapable)
                .textContentType(.oneTimeCode)
                .submitLabel(.join)
                .focused($typing)
                .onSubmit { join(code) }
                .foregroundStyle(.clear)
                .tint(.clear)
                .accessibilityLabel(L("Watch party kód, 6 karakter"))
            HStack(spacing: 8) {
                ForEach(0..<6, id: \.self) { i in
                    let active = typing && i == min(chars.count, 5)
                    Text(i < chars.count ? String(chars[i]) : "")
                        .font(.title.monospaced().weight(.bold))
                        .foregroundStyle(Theme.plasterBright)
                        .frame(maxWidth: .infinity)
                        .frame(height: 58)
                        .background(Theme.raised, in: .rect(cornerRadius: 12))
                        .overlay(RoundedRectangle(cornerRadius: 12)
                            .strokeBorder(active ? Theme.amber : (error != nil ? Theme.danger.opacity(0.6) : Theme.rule),
                                          lineWidth: active ? 2 : 1))
                }
            }
            .allowsHitTesting(false)
            .accessibilityHidden(true)
        }
        .contentShape(.rect)
        .onTapGesture { typing = true }
        .onChange(of: code) { _, new in
            // Pasted invite links and lower case are fine; only the code stays.
            let clean = PartySync.code(scanned: new)
                ?? String(new.uppercased().filter { $0.isASCII && ($0.isLetter || $0.isNumber) }.prefix(6))
            if clean != new { code = clean; return }
            error = nil
            if clean.count == 6 { join(clean) }
        }
    }

    private func join(_ c: String) {
        guard PartySync.normalizedCode(c) != nil, !model.partyJoining else { return }
        error = nil
        model.joinParty(c)
    }
}

/// Amber corner brackets around the scan area; they close in and turn solid on a read.
private struct ScanFrame: View {
    var done: Bool

    var body: some View {
        GeometryReader { geo in
            let s = min(geo.size.width, geo.size.height)
            let arm = s * 0.18
            Path { p in
                for (x, y, dx, dy) in [(0.0, 0.0, 1.0, 1.0), (s, 0, -1, 1), (0, s, 1, -1), (s, s, -1, -1)] {
                    p.move(to: CGPoint(x: x, y: y + dy * arm))
                    p.addLine(to: CGPoint(x: x, y: y))
                    p.addLine(to: CGPoint(x: x + dx * arm, y: y))
                }
            }
            .stroke(Theme.amber, style: StrokeStyle(lineWidth: 5, lineCap: .round, lineJoin: .round))
            .frame(width: s, height: s)
            .background(done ? Theme.amber.opacity(0.18) : .clear, in: .rect(cornerRadius: 12))
            .scaleEffect(done ? 0.9 : 1)
            .position(x: geo.size.width / 2, y: geo.size.height / 2)
            .animation(.spring(duration: 0.35), value: done)
        }
    }
}
