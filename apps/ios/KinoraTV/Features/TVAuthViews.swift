import SwiftUI
import KinoraKit

/// Two-column auth page: brand and explanation on the left, the controls on the right.
private struct TVAuthPage<Side: View, Content: View>: View {
    var title: String
    @ViewBuilder var side: Side
    @ViewBuilder var content: Content

    var body: some View {
        HStack(alignment: .top, spacing: 100) {
            VStack(alignment: .leading, spacing: 28) {
                BrandMark(size: 76)
                Text(title).font(Theme.display(64, weight: .bold)).foregroundStyle(Theme.plasterBright)
                side
            }
            .frame(width: 640, alignment: .leading)
            VStack(alignment: .leading, spacing: 26) { content }
                .frame(maxWidth: .infinity, alignment: .leading)
        }
        .padding(.horizontal, 100)
        .padding(.vertical, 80)
        .frame(maxWidth: .infinity, maxHeight: .infinity, alignment: .topLeading)
    }
}

/// The server the auth screens are about.
private struct TVServerLine: View {
    @Environment(AppModel.self) private var model
    var body: some View {
        if let s = model.server {
            Label("\(s.name) · \(ServerAddress.host(of: s.url))", systemImage: "server.rack")
                .font(.callout).foregroundStyle(Theme.muted)
        }
    }
}

// MARK: - servers

struct TVServersView: View {
    @Environment(AppModel.self) private var model
    @State private var address = ""
    @State private var found: [DiscoveredServer] = []
    @State private var scanning = false
    @State private var busy = false
    @State private var error: String?

    var body: some View {
        TVAuthPage(title: "Válassz szervert") {
            Text("A Kinora a saját médiaszerveredről játszik le. Ha a szerver ezen a hálózaton fut, lent megjelenik; különben írd be a címét.")
                .font(.title3).foregroundStyle(Theme.plaster)
            TVErrorText(message: error ?? model.notice)
        } content: {
            ScrollView {
                VStack(alignment: .leading, spacing: 22) {
                    if !model.servers.isEmpty {
                        TVHeading(text: "Mentett szerverek")
                        ForEach(model.servers) { s in
                            Button { open(s) } label: {
                                row(s.name, ServerAddress.host(of: s.url), icon: "server.rack")
                            }
                            .buttonStyle(TVRowStyle())
                            .contextMenu {
                                Button("Elfelejtés", systemImage: "trash", role: .destructive) { model.forget(s) }
                            }
                        }
                    }
                    HStack {
                        TVHeading(text: "Ezen a hálózaton")
                        if scanning { ProgressView().padding(.leading, 12) }
                    }
                    ForEach(found) { f in
                        Button { connect(f.address) } label: {
                            row(f.name, ServerAddress.host(of: f.address) + " · " + f.version, icon: "wifi")
                        }
                        .buttonStyle(TVRowStyle())
                    }
                    if !scanning && found.isEmpty {
                        Text("Nem találtam szervert a helyi hálózaton.").foregroundStyle(Theme.muted)
                    }
                    TVHeading(text: "Cím megadása").padding(.top, 10)
                    TextField("pl. 192.168.1.20:8096 vagy kinora.example.com", text: $address)
                        .textContentType(.URL)
                        .keyboardType(.URL)
                        .autocorrectionDisabled()
                        .textInputAutocapitalization(.never)
                        .onSubmit { connect(address) }
                    HStack(spacing: 24) {
                        Button { connect(address) } label: { Label("Csatlakozás", systemImage: "arrow.right") }
                            .buttonStyle(TVPillStyle(prominent: true))
                            .disabled(address.trimmingCharacters(in: .whitespaces).isEmpty || busy)
                        Button { scan() } label: { Label("Keresés újra", systemImage: "arrow.clockwise") }
                            .buttonStyle(TVPillStyle())
                            .disabled(scanning)
                        if busy { ProgressView() }
                    }
                }
                .padding(30)
            }
            .scrollClipDisabled()
        }
        .task { scan() }
        .onDisappear { model.notice = nil }
    }

    private func row(_ title: String, _ subtitle: String, icon: String) -> some View {
        HStack(spacing: 24) {
            Image(systemName: icon).font(.title2).frame(width: 50)
            VStack(alignment: .leading, spacing: 4) {
                Text(title).font(.headline)
                Text(subtitle).font(.callout).opacity(0.75)
            }
        }
    }

    private func connect(_ input: String) {
        let input = input.trimmingCharacters(in: .whitespaces)
        guard !busy, !input.isEmpty else { return }
        run { try await model.connect(to: input) }
    }

    private func open(_ s: SavedServer) {
        guard !busy else { return }
        run { try await model.open(s) }
    }

    private func run(_ work: @escaping () async throws -> Void) {
        error = nil
        model.notice = nil
        busy = true
        Task {
            defer { busy = false }
            do { try await work() } catch { self.error = error.localizedDescription }
        }
    }

    private func scan() {
        guard !scanning else { return }
        scanning = true
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

// MARK: - login

struct TVLoginView: View {
    @Environment(AppModel.self) private var model
    @State private var username = ""
    @State private var password = ""
    @State private var busy = false
    @State private var error: String?

    var body: some View {
        TVAuthPage(title: "Belépés") {
            TVServerLine()
            Text("A legegyszerűbb a kóddal belépés: a tévén megjelenő kódot egy már bejelentkezett telefonon vagy gépen hagyod jóvá.")
                .font(.title3).foregroundStyle(Theme.plaster)
            TVErrorText(message: error ?? model.notice)
        } content: {
            if model.status?.quickconnectEnabled != false {
                Button { model.notice = nil; model.go(.quickConnect) } label: {
                    Label("Belépés kóddal", systemImage: "qrcode")
                }
                .buttonStyle(TVPillStyle(prominent: true))
                .padding(.bottom, 20)
            }
            TVHeading(text: "Felhasználónév és jelszó")
            TextField("Felhasználónév", text: $username)
                .textContentType(.username)
                .autocorrectionDisabled()
                .textInputAutocapitalization(.never)
            SecureField("Jelszó", text: $password)
                .textContentType(.password)
                .onSubmit(login)
            HStack(spacing: 24) {
                Button(action: login) { Label("Belépés", systemImage: "person.fill.checkmark") }
                    .buttonStyle(TVPillStyle())
                    .disabled(username.isEmpty || password.isEmpty || busy)
                Button("Másik szerver") { model.showServers() }
                    .buttonStyle(TVPillStyle())
                if busy { ProgressView() }
            }
        }
        .onAppear { username = model.server?.username ?? "" }
    }

    private func login() {
        guard !busy, !username.isEmpty, !password.isEmpty else { return }
        error = nil
        model.notice = nil
        busy = true
        Task {
            defer { busy = false }
            do {
                try await model.login(username: username.trimmingCharacters(in: .whitespaces), password: password)
            } catch {
                self.error = error.localizedDescription
            }
        }
    }
}

// MARK: - quick connect

struct TVQuickConnectView: View {
    @Environment(AppModel.self) private var model
    @State private var start: QuickConnectStart?
    @State private var message: String?
    @State private var failed = false
    @State private var attempt = 0

    var body: some View {
        TVAuthPage(title: "Belépés kóddal") {
            TVServerLine()
            if let server = model.server {
                Text("Egy már bejelentkezett telefonon vagy gépen nyisd meg a Kinorát (\(server.url)), és írd be a kódot.")
                    .font(.title3).foregroundStyle(Theme.plaster)
            }
            Text("iPhone-on: Beállítások → Eszköz csatlakoztatása. A webes felületen: profil menü → Eszköz csatlakoztatása.")
                .font(.callout).foregroundStyle(Theme.muted)
        } content: {
            if let start {
                Text(spaced(start.code))
                    .font(Theme.display(150))
                    .monospacedDigit()
                    .foregroundStyle(Theme.amber)
                    .frame(maxWidth: .infinity)
                    .padding(.vertical, 40)
                    .background(Theme.panel, in: .rect(cornerRadius: 24))
                    .accessibilityLabel(start.code.map(String.init).joined(separator: " "))
                HStack(spacing: 18) {
                    if !failed { ProgressView() }
                    Text(message ?? "Várakozás a jóváhagyásra…")
                        .foregroundStyle(failed ? Theme.danger : Theme.muted)
                }
            } else if failed {
                TVErrorText(message: message)
            } else {
                ProgressView().frame(maxWidth: .infinity).padding(60)
            }
            HStack(spacing: 24) {
                if failed {
                    Button { attempt += 1 } label: { Label("Új kód", systemImage: "arrow.clockwise") }
                        .buttonStyle(TVPillStyle(prominent: true))
                }
                Button { model.go(.login) } label: { Label("Jelszóval", systemImage: "person") }
                    .buttonStyle(TVPillStyle())
                Button("Másik szerver") { model.showServers() }
                    .buttonStyle(TVPillStyle())
            }
            .padding(.top, 20)
        }
        .task(id: attempt) { await run() }
    }

    private func spaced(_ code: String) -> String {
        code.count == 6 ? code.prefix(3) + " " + code.suffix(3) : code
    }

    private func run() async {
        start = nil
        failed = false
        message = nil
        do {
            let s = try await model.startQuickConnect()
            start = s
            try await Task.sleep(for: .seconds(2))
            while !Task.isCancelled {
                let st = try await model.pollQuickConnect(s)
                switch st.status {
                case .approved: return
                case .denied: failed = true; message = "A csatlakozást elutasították."; return
                case .expired: failed = true; message = "A kód lejárt."; return
                case .pending: break
                }
                try await Task.sleep(for: .seconds(max(1, s.pollInterval ?? 2)))
            }
        } catch is CancellationError {
        } catch {
            failed = true
            message = error.localizedDescription
        }
    }
}

// MARK: - profiles

struct TVProfilesView: View {
    @Environment(AppModel.self) private var model
    @State private var pinFor: Profile?
    @State private var error: String?

    var body: some View {
        VStack(spacing: 70) {
            BrandMark(size: 60)
            Text("Ki nézi?").font(Theme.display(72, weight: .bold))
            if let profiles = model.me?.profiles {
                HStack(spacing: 60) {
                    ForEach(profiles) { p in
                        Button { pick(p) } label: { TVProfileFace(profile: p) }
                            .buttonStyle(TVCardStyle())
                    }
                }
            } else {
                ProgressView()
            }
            TVErrorText(message: error)
            Button("Másik szerver") { model.showServers() }
                .buttonStyle(TVPillStyle())
        }
        .frame(maxWidth: .infinity, maxHeight: .infinity)
        .task { if model.me == nil { try? await model.guarded { try await model.loadMe() } } }
        .sheet(item: $pinFor) { TVPinSheet(profile: $0) }
    }

    private func pick(_ p: Profile) {
        if p.hasPin { pinFor = p; return }
        error = nil
        Task {
            do { try await model.guarded { try await model.selectProfile(p, pin: nil) } }
            catch { self.error = error.localizedDescription }
        }
    }
}

struct TVProfileFace: View {
    @Environment(\.tvCardFocused) private var focused
    var profile: Profile
    var size: CGFloat = 220

    var body: some View {
        VStack(spacing: 22) {
            ZStack(alignment: .bottomTrailing) {
                Text(profile.name.prefix(1).uppercased())
                    .font(Theme.display(size * 0.5))
                    .foregroundStyle(Theme.black)
                    .frame(width: size, height: size)
                    .background(Color(css: profile.color), in: .rect(cornerRadius: size * 0.18))
                    .overlay {
                        RoundedRectangle(cornerRadius: size * 0.18).stroke(Theme.plasterBright, lineWidth: focused ? 6 : 0)
                    }
                if profile.hasPin {
                    Image(systemName: "lock.fill")
                        .font(.callout)
                        .padding(12)
                        .background(Theme.black.opacity(0.75), in: .circle)
                        .padding(12)
                }
            }
            Text(profile.name).font(.title3.weight(.semibold))
                .foregroundStyle(focused ? Theme.plasterBright : Theme.plaster)
            if profile.isKids {
                Text("Gyerek").font(.callout).foregroundStyle(Theme.muted)
            }
        }
    }
}

private struct TVPinSheet: View {
    @Environment(AppModel.self) private var model
    @Environment(\.dismiss) private var dismiss
    var profile: Profile
    @State private var pin = ""
    @State private var busy = false
    @State private var error: String?

    var body: some View {
        VStack(spacing: 40) {
            TVProfileFace(profile: profile, size: 160)
            Text("Add meg a PIN-kódot").font(.title2.bold())
            SecureField("PIN", text: $pin)
                .keyboardType(.numberPad)
                .frame(width: 500)
                .onSubmit(submit)
            TVErrorText(message: error)
            HStack(spacing: 24) {
                Button("Belépés", action: submit)
                    .buttonStyle(TVPillStyle(prominent: true))
                    .disabled(pin.isEmpty || busy)
                Button("Mégse") { dismiss() }
                    .buttonStyle(TVPillStyle())
            }
        }
        .padding(80)
    }

    private func submit() {
        guard !pin.isEmpty, !busy else { return }
        busy = true
        error = nil
        Task {
            defer { busy = false }
            do {
                try await model.guarded { try await model.selectProfile(profile, pin: pin) }
                dismiss()
            } catch {
                self.error = error.localizedDescription
                pin = ""
            }
        }
    }
}
