import SwiftUI
import KinoraKit

/// "Szerver hozzáadása": type an address, search the home network, or pick a saved server.
struct ServersView: View {
    @Environment(AppModel.self) private var model
    @State private var address = ""
    @State private var error: String?
    @State private var busy = false
    @State private var scanning = false
    @State private var found: [DiscoveredServer] = []
    @State private var scannedOnce = false
    @FocusState private var focused: Bool

    var body: some View {
        AuthPage {
            BrandMark()
            Text("Szerver hozzáadása")
                .font(Theme.display(40, weight: .bold))
            // Verbatim: the example address must not turn into a tappable link.
            Text(verbatim: "Add meg a Kinora-szerver címét. Elég az IP-cím, pl. 192.168.1.10 – a port alapból 8096. Otthonon kívülről a nyilvános címet (pl. https://kinora.pelda.hu).")
                .foregroundStyle(Theme.plaster)

            VStack(alignment: .leading, spacing: 6) {
                Text("Szerver címe").font(.subheadline).foregroundStyle(Theme.muted)
                TextField("", text: $address, prompt: Text("192.168.1.10:8096").foregroundStyle(Theme.muted))
                    .textContentType(.URL)
                    .keyboardType(.URL)
                    .textInputAutocapitalization(.never)
                    .autocorrectionDisabled()
                    .submitLabel(.go)
                    .focused($focused)
                    .onSubmit { connect(address) }
                    .kinoraField()
                    .accessibilityLabel("Szerver címe")
            }

            ErrorText(message: error ?? model.notice)

            // Side by side when they fit; stacked on a narrow phone or with large text.
            ViewThatFits(in: .horizontal) {
                HStack(spacing: 12) { connectButton; scanButton.fixedSize() }
                VStack(spacing: 12) { connectButton; scanButton }
            }

            if scanning || !found.isEmpty || scannedOnce {
                section("A helyi hálózaton") {
                    ForEach(found) { s in
                        row(title: s.name, subtitle: ServerAddress.host(of: s.address) + " · " + s.version, icon: "server.rack") {
                            connect(s.address)
                        }
                    }
                    if scanning {
                        HStack { ProgressView(); Text("Keresés a helyi hálózaton…").foregroundStyle(Theme.muted) }
                            .padding(.vertical, 8)
                    } else if found.isEmpty {
                        Text("Nem találtam szervert. Írd be kézzel a címét.").foregroundStyle(Theme.muted)
                    }
                }
            }

            if !model.servers.isEmpty {
                section("Mentett szerverek") {
                    ForEach(model.servers) { s in
                        row(title: s.name, subtitle: ServerAddress.host(of: s.url) + (s.username.map { " · " + $0 } ?? ""), icon: "play.tv") {
                            open(s)
                        }
                        .contextMenu {
                            Button("Elfelejtés", systemImage: "trash", role: .destructive) { model.forget(s) }
                        }
                    }
                    Text("Egy mentett szerver elfelejtéséhez tartsd lenyomva a sorát.")
                        .font(.footnote)
                        .foregroundStyle(Theme.muted)
                }
            }

            Text("Kinora \(AppModel.appVersion)")
                .font(.footnote)
                .foregroundStyle(Theme.muted)
                .padding(.top, 12)
        }
        .onDisappear { model.notice = nil }
    }

    private var connectButton: some View {
        Button { connect(address) } label: {
            if busy { ProgressView().tint(Theme.amberInk) } else { Text("Csatlakozás") }
        }
        .buttonStyle(PrimaryButtonStyle())
        .disabled(busy || address.trimmingCharacters(in: .whitespaces).isEmpty)
    }

    private var scanButton: some View {
        Button { scan() } label: {
            Label(scanning ? "Keresés…" : "Szerver keresése", systemImage: "wifi")
                .lineLimit(1)
        }
        .buttonStyle(SecondaryButtonStyle())
        .disabled(scanning)
    }

    private func section<C: View>(_ title: String, @ViewBuilder _ content: () -> C) -> some View {
        VStack(alignment: .leading, spacing: 8) {
            Text(title).font(.headline).foregroundStyle(Theme.plaster).padding(.top, 8)
            content()
        }
    }

    private func row(title: String, subtitle: String, icon: String, action: @escaping () -> Void) -> some View {
        Button(action: action) {
            HStack(spacing: 14) {
                Image(systemName: icon).font(.title3).foregroundStyle(Theme.amber).frame(width: 32)
                VStack(alignment: .leading, spacing: 2) {
                    Text(title).font(.headline).foregroundStyle(Theme.plasterBright)
                    Text(subtitle).font(.subheadline).foregroundStyle(Theme.muted)
                }
                Spacer()
                Image(systemName: "chevron.right").foregroundStyle(Theme.muted)
            }
            .padding(14)
            .background(Theme.panel, in: .rect(cornerRadius: 10))
        }
        .buttonStyle(.plain)
        .disabled(busy)
    }

    private func connect(_ input: String) {
        guard !busy else { return }
        focused = false
        error = nil
        model.notice = nil
        busy = true
        Task {
            defer { busy = false }
            do { try await model.connect(to: input) }
            catch { self.error = error.localizedDescription; Haptic.error.play() }
        }
    }

    private func open(_ s: SavedServer) {
        guard !busy else { return }
        error = nil
        busy = true
        Task {
            defer { busy = false }
            do { try await model.open(s) }
            catch { self.error = error.localizedDescription; Haptic.error.play() }
        }
    }

    private func scan() {
        scanning = true
        scannedOnce = true
        found = []
        Task {
            await Discovery.scanLocalNetwork { server in
                await MainActor.run {
                    if !found.contains(where: { $0.id == server.id }) { found.append(server) }
                }
            }
            scanning = false
        }
    }
}
