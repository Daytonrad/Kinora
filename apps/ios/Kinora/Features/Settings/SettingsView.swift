import SwiftUI
import KinoraKit

/// The web /settings page, natively: account, playback, app, devices and server.
struct SettingsView: View {
    @Environment(AppModel.self) private var model
    @Environment(\.dismiss) private var dismiss

    @State private var error: String?
    @State private var devices: [Device]?
    @State private var devicesError: String?
    @State private var subLangs = ""
    @State private var audioLangs = ""
    @State private var backgroundAudio = AppSettings.backgroundAudio
    @State private var subtitleStyle = AppSettings.subtitleStyle
    @State private var cellularMax = AppSettings.cellularMaxBitrate
    @State private var wifiOnly = AppSettings.downloadsWifiOnly
    @State private var deleteWatched = AppSettings.deleteWatchedDownloads
    @State private var downloadLimit = AppSettings.downloadLimitGB
    @State private var addingAddress = false
    @State private var newAddress = ""
    @State private var addressBusy = false
    @State private var confirmLogout = false
    @State private var confirmRemoveOthers = false
    @State private var notice: String?
    @FocusState private var focused: LangField?

    private enum LangField { case sub, audio }

    private var profile: Profile? { model.me?.profile }
    /// Kids profiles and profile-locked devices (Quick Connect with a fixed profile) only look.
    private var canEdit: Bool { (profile.map { !$0.isKids } ?? false) && model.me?.profileLocked != true }

    var body: some View {
        NavigationStack {
            Form {
                if let error {
                    Section { ErrorText(message: error) }
                }
                account
                if canEdit { playback }
                app
                subtitleSection
                downloadsSection
                Section {
                    NavigationLink {
                        HomeScenesView()
                    } label: {
                        Label("Otthon-jelenetek", systemImage: "lightbulb.max")
                    }
                } footer: {
                    Text("Például lekapcsolja a lámpákat, amikor elindul egy film.")
                }
                .listRowBackground(Theme.panel)
                devicesSection
                serverSection
                Section {
                } footer: {
                    Text("Kinora \(AppModel.appVersion) · iOS")
                        .frame(maxWidth: .infinity)
                        .padding(.top, 8)
                }
            }
            .scrollContentBackground(.hidden)
            .background(Theme.black)
            .navigationTitle("Beállítások")
            .navigationBarTitleDisplayMode(.inline)
            .toolbarBackground(Theme.black, for: .navigationBar)
            .toolbar {
                ToolbarItem(placement: .confirmationAction) {
                    Button("Kész") { saveLanguages(); dismiss() }
                }
            }
            .navigationDestination(for: ConnectRoute.self) { _ in ConnectDeviceView() }
            .task { await loadDevices() }
            .onAppear {
                subLangs = profile?.subLangs ?? ""
                audioLangs = profile?.audioLangs ?? ""
            }
            .onChange(of: focused) { old, _ in if old != nil { saveLanguages() } }
            .alert("Külső cím hozzáadása", isPresented: $addingAddress) {
                TextField("https://kinora.pelda.hu", text: $newAddress)
                    .textInputAutocapitalization(.never)
                    .autocorrectionDisabled()
                    .keyboardType(.URL)
                Button("Mégse", role: .cancel) { newAddress = "" }
                Button("Hozzáadás") { addAddress() }
            } message: {
                Text("Az a cím, amelyen a szerver otthonon kívül is elérhető.")
            }
            .confirmationDialog("Biztosan kijelentkezel?", isPresented: $confirmLogout, titleVisibility: .visible) {
                Button("Kijelentkezés", role: .destructive) { Task { await model.logout() } }
            }
            .confirmationDialog("Kiléptetsz minden más eszközt?", isPresented: $confirmRemoveOthers, titleVisibility: .visible) {
                Button("Kiléptetés mindenhonnan", role: .destructive) { removeOthers() }
            } message: {
                Text("Ezen a telefonon bejelentkezve maradsz; a többi eszközön újra be kell lépni.")
            }
        }
        .tint(Theme.amber)
        .preferredColorScheme(.dark)
    }

    // MARK: - sections

    private var account: some View {
        Section("Fiók") {
            if let p = profile, let me = model.me {
                HStack(spacing: 14) {
                    ProfileFace(profile: p, size: 44, showsName: false)
                    VStack(alignment: .leading, spacing: 2) {
                        Text(p.name).font(.headline)
                        Text(me.user.username).font(.subheadline).foregroundStyle(Theme.muted)
                    }
                }
                .padding(.vertical, 4)
            }
            if model.me?.profileLocked != true {
                Button { model.switchProfile() } label: {
                    Label("Profilváltás", systemImage: "person.2")
                }
            }
            if canEdit {
                NavigationLink(value: ConnectRoute()) {
                    Label("Eszköz csatlakoztatása", systemImage: "tv.and.mediabox")
                }
            }
        }
        .listRowBackground(Theme.panel)
    }

    private var playback: some View {
        Section("Lejátszás") {
            Picker("Feliratok", selection: setting(\.subMode, default: "auto") { ProfileUpdate(subMode: $0) }) {
                ForEach(SettingChoices.subModes, id: \.0) { Text($0.1).tag($0.0) }
            }
            LabeledContent("Feliratnyelvek") {
                TextField("pl. hun, eng", text: $subLangs)
                    .multilineTextAlignment(.trailing)
                    .focused($focused, equals: .sub)
                    .onSubmit(saveLanguages)
            }
            LabeledContent("Hangsávnyelvek") {
                TextField("pl. hun, eng", text: $audioLangs)
                    .multilineTextAlignment(.trailing)
                    .focused($focused, equals: .audio)
                    .onSubmit(saveLanguages)
            }
            Picker("Maximális minőség", selection: setting(\.maxBitrate, default: 0) { ProfileUpdate(maxBitrate: $0) }) {
                ForEach(SettingChoices.bitrates, id: \.0) { Text($0.1).tag($0.0) }
            }
            Toggle("Következő rész automatikus indítása",
                   isOn: setting(\.autoplayNext, default: true) { ProfileUpdate(autoplayNext: $0) })
        }
        .textInputAutocapitalization(.never)
        .autocorrectionDisabled()
        .listRowBackground(Theme.panel)
    }

    private var subtitleSection: some View {
        Section {
            Picker("Feliratméret", selection: $subtitleStyle.size) {
                ForEach(SubtitleStyle.Size.allCases, id: \.self) { Text(SubtitleStyle.label($0)).tag($0) }
            }
            Picker("Felirat háttere", selection: $subtitleStyle.background) {
                ForEach(SubtitleStyle.Background.allCases, id: \.self) { Text(SubtitleStyle.label($0)).tag($0) }
            }
            Picker("Felirat színe", selection: $subtitleStyle.color) {
                ForEach(SubtitleStyle.Color.allCases, id: \.self) { Text(SubtitleStyle.label($0)).tag($0) }
            }
            Text("Így néz ki a felirat")
                .font(.system(size: subtitleStyle.points, weight: .semibold))
                .foregroundStyle(subtitleStyle.color == .yellow ? Color(red: 1, green: 0.88, blue: 0.3) : .white)
                .padding(.horizontal, subtitleStyle.background == .box ? 10 : 0)
                .padding(.vertical, subtitleStyle.background == .box ? 4 : 0)
                .background(subtitleStyle.background == .box ? Color.black.opacity(0.6) : .clear, in: .rect(cornerRadius: 6))
                .shadow(color: .black, radius: subtitleStyle.background == .none ? 0 : 3)
                .frame(maxWidth: .infinity)
                .padding(.vertical, 12)
                .background(LinearGradient(colors: [.gray.opacity(0.7), .blue.opacity(0.4)], startPoint: .top, endPoint: .bottom),
                            in: .rect(cornerRadius: 8))
                .accessibilityHidden(true)
            Picker("Minőség mobilneten", selection: $cellularMax) {
                Text("Nincs korlát").tag(0)
                ForEach(SettingChoices.bitrates.filter { $0.0 > 0 }, id: \.0) { Text($0.1).tag($0.0) }
            }
        } header: {
            Text("Felirat és minőség ezen az eszközön")
        } footer: {
            Text("Mobilneten és személyes hotspoton a lejátszás legfeljebb ekkora sávszélességet használ. A lejátszóban egy-egy videónál felülírhatod.")
        }
        .onChange(of: subtitleStyle) { _, s in AppSettings.subtitleStyle = s }
        .onChange(of: cellularMax) { _, v in AppSettings.cellularMaxBitrate = v }
        .listRowBackground(Theme.panel)
    }

    private var app: some View {
        Section {
            Picker("Intró átugrása", selection: setting(\.skipIntro, default: "manual") { ProfileUpdate(skipIntro: $0) }) {
                ForEach(SettingChoices.skipModes, id: \.0) { Text($0.1).tag($0.0) }
            }
            .disabled(!canEdit)
            Toggle("Hang folytatása a háttérben", isOn: $backgroundAudio)
                .onChange(of: backgroundAudio) { _, on in AppSettings.backgroundAudio = on }
        } header: {
            Text("Alkalmazás")
        } footer: {
            Text("Ha nem indul el a kép a képben, a hang akkor is szól tovább, amikor kilépsz az appból vagy lezárod a telefont.")
        }
        .listRowBackground(Theme.panel)
    }

    private var downloadsSection: some View {
        Section {
            Toggle("Csak Wi-Fi-n", isOn: $wifiOnly)
                .onChange(of: wifiOnly) { _, on in AppSettings.downloadsWifiOnly = on }
            Toggle("Megnézett letöltések törlése", isOn: $deleteWatched)
                .onChange(of: deleteWatched) { _, on in AppSettings.deleteWatchedDownloads = on }
            Picker("Tárhelykeret", selection: $downloadLimit) {
                ForEach([0, 5, 10, 20, 50, 100], id: \.self) { gb in
                    Text(gb == 0 ? "Nincs" : "\(gb) GB").tag(gb)
                }
            }
            .onChange(of: downloadLimit) { _, gb in AppSettings.downloadLimitGB = gb }
        } header: {
            Text("Letöltések")
        } footer: {
            Text("Csak Wi-Fi-n: mobilneten a letöltés megvárja a Wi-Fi-t. A megnézett letöltés a végignézés után törlődik az eszközről. A keret elérése után új letöltés nem indul.")
        }
        .listRowBackground(Theme.panel)
    }

    @ViewBuilder private var devicesSection: some View {
        Section {
            if let devices {
                ForEach(devices) { d in
                    DeviceRow(device: d)
                        .swipeActions {
                            if !d.current && canEdit {
                                Button("Kiléptetés", role: .destructive) { remove(d) }
                            }
                        }
                        .contextMenu {
                            if !d.current && canEdit {
                                Button("Kiléptetés", systemImage: "rectangle.portrait.and.arrow.right", role: .destructive) { remove(d) }
                            }
                        }
                }
                if canEdit && devices.count > 1 {
                    Button("Kiléptetés mindenhonnan (ezen kívül)", role: .destructive) { confirmRemoveOthers = true }
                        .foregroundStyle(Theme.danger)
                }
            } else if let devicesError {
                ErrorText(message: devicesError)
                Button("Újra") { Task { await loadDevices() } }
            } else {
                ProgressView().frame(maxWidth: .infinity)
            }
        } header: {
            Text("Bejelentkezett eszközök")
        } footer: {
            if canEdit, (devices?.count ?? 0) > 1 { Text("Egy eszköz kiléptetéséhez húzd balra a sorát.") }
        }
        .listRowBackground(Theme.panel)
    }

    private var serverSection: some View {
        Section {
            if let server = model.server {
                VStack(alignment: .leading, spacing: 2) {
                    Text(model.me?.serverName ?? server.name).font(.headline)
                    if let version = model.me?.version {
                        Text("Verzió \(version)").font(.subheadline).foregroundStyle(Theme.muted)
                    }
                }
                ForEach(server.addresses, id: \.self) { url in
                    VStack(alignment: .leading, spacing: 2) {
                        Text(url).font(.callout).lineLimit(2).textSelection(.enabled)
                        if url == server.url {
                            Text("most ezt használja").font(.caption).foregroundStyle(Theme.amber)
                        }
                    }
                }
                Button {
                    addingAddress = true
                } label: {
                    if addressBusy {
                        HStack { ProgressView(); Text("Cím ellenőrzése…") }
                    } else {
                        Label("Külső cím hozzáadása", systemImage: "plus")
                    }
                }
                .disabled(addressBusy)
                if let notice { Text(notice).font(.callout).foregroundStyle(Theme.muted) }
            }
            Button { model.showServers() } label: {
                Label("Másik szerver", systemImage: "server.rack")
            }
            Button(role: .destructive) { confirmLogout = true } label: {
                Label("Kijelentkezés", systemImage: "rectangle.portrait.and.arrow.right")
                    .foregroundStyle(Theme.danger)
            }
        } header: {
            Text("Szerver")
        }
        .listRowBackground(Theme.panel)
    }

    // MARK: - actions

    /// A binding to a profile setting that saves on change.
    private func setting<V: Equatable>(_ key: KeyPath<Profile, V?>, default fallback: V,
                                       _ update: @escaping (V) -> ProfileUpdate) -> Binding<V> {
        Binding {
            profile?[keyPath: key] ?? fallback
        } set: { value in
            guard value != (profile?[keyPath: key] ?? fallback) else { return }
            save(update(value))
        }
    }

    private func save(_ update: ProfileUpdate) {
        Task {
            do {
                try await model.updateProfile(update)
                error = nil
            } catch is CancellationError {
            } catch {
                self.error = error.localizedDescription
            }
        }
    }

    private func saveLanguages() {
        guard canEdit, let p = profile else { return }
        let sub = subLangs.trimmingCharacters(in: .whitespaces)
        let audio = audioLangs.trimmingCharacters(in: .whitespaces)
        var u = ProfileUpdate()
        if sub != (p.subLangs ?? "") { u.subLangs = sub }
        if audio != (p.audioLangs ?? "") { u.audioLangs = audio }
        if u != ProfileUpdate() { save(u) }
    }

    private func loadDevices() async {
        do {
            devices = try await model.api { try await $0.devices() }
            devicesError = nil
        } catch is CancellationError {
        } catch {
            if devices == nil { devicesError = error.localizedDescription }
        }
    }

    private func remove(_ d: Device) {
        Task {
            do {
                try await model.api { try await $0.removeDevice(d.id) }
                withAnimation { devices?.removeAll { $0.id == d.id } }
            } catch {
                self.error = error.localizedDescription
            }
        }
    }

    private func removeOthers() {
        Task {
            do {
                try await model.api { try await $0.removeOtherDevices() }
                await loadDevices()
            } catch {
                self.error = error.localizedDescription
            }
        }
    }

    private func addAddress() {
        let input = newAddress.trimmingCharacters(in: .whitespaces)
        newAddress = ""
        guard !input.isEmpty else { return }
        addressBusy = true
        notice = nil
        Task {
            defer { addressBusy = false }
            do {
                try await model.addAddress(input)
                notice = "Cím hozzáadva."
                error = nil
            } catch {
                self.error = error.localizedDescription
            }
        }
    }
}

private struct ConnectRoute: Hashable {}

private struct DeviceRow: View {
    var device: Device

    var body: some View {
        HStack(spacing: 14) {
            Image(systemName: device.isTV ? "tv" : device.platform == "web" ? "globe" : "iphone")
                .font(.title3)
                .foregroundStyle(Theme.muted)
                .frame(width: 28)
            VStack(alignment: .leading, spacing: 2) {
                Text((device.name.isEmpty ? device.platformLabel : device.name) + (device.current ? " · ez az eszköz" : ""))
                    .font(.body.weight(device.current ? .semibold : .regular))
                Text(detail).font(.caption).foregroundStyle(Theme.muted)
            }
        }
        .accessibilityElement(children: .combine)
    }

    private var detail: String {
        let seen = Int64(Date.now.timeIntervalSince1970) - device.lastSeen
        return [device.platformLabel, device.appVersion.isEmpty ? nil : "v" + device.appVersion,
                device.ip.isEmpty ? nil : device.ip, SettingChoices.ago(seen)]
            .compactMap { $0 }.joined(separator: " · ")
    }
}
