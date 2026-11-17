import SwiftUI
import KinoraKit

/// Approves a TV's (or another device's) Quick Connect code from this phone.
struct ConnectDeviceView: View {
    @Environment(AppModel.self) private var model

    @State private var code = ""
    @State private var request: QuickConnectRequest?
    @State private var profileID = 0
    @State private var busy = false
    @State private var error: String?
    @State private var approved = false
    @FocusState private var codeFocused: Bool

    var body: some View {
        Form {
            if approved {
                Section {
                    Label("Kész! Az eszköz pár másodpercen belül belép.", systemImage: "checkmark.circle.fill")
                        .foregroundStyle(Theme.amber)
                    Button("Másik eszköz csatlakoztatása") { reset() }
                }
                .listRowBackground(Theme.panel)
            } else if let request {
                Section {
                    VStack(spacing: 10) {
                        Text(request.code)
                            .font(Theme.display(40))
                            .monospacedDigit()
                            .tracking(4)
                        Text("\(request.deviceName.flatMap { $0.isEmpty ? nil : $0 } ?? "Ismeretlen eszköz") szeretne belépni a fiókodba.")
                            .multilineTextAlignment(.center)
                            .foregroundStyle(Theme.plaster)
                    }
                    .frame(maxWidth: .infinity)
                    .padding(.vertical, 8)
                }
                .listRowBackground(Theme.panel)
                Section("Melyik profillal lépjen be?") {
                    Picker("Profil", selection: $profileID) {
                        Text("Profilválasztó az eszközön").tag(0)
                        ForEach(model.me?.profiles ?? []) { p in
                            Text(p.isKids ? "\(p.name) (Gyerek)" : p.name).tag(p.id)
                        }
                    }
                    .pickerStyle(.inline)
                    .labelsHidden()
                }
                .listRowBackground(Theme.panel)
                Section {
                    ErrorText(message: error)
                    Button { decide(approve: true) } label: {
                        Text("Jóváhagyás").frame(maxWidth: .infinity)
                    }
                    .buttonStyle(PrimaryButtonStyle())
                    .listRowInsets(EdgeInsets())
                    .listRowBackground(Color.clear)
                    Button("Elutasítás", role: .destructive) { decide(approve: false) }
                        .foregroundStyle(Theme.danger)
                        .frame(maxWidth: .infinity)
                }
                .disabled(busy)
                .listRowBackground(Theme.panel)
            } else {
                Section {
                    TextField("123456", text: $code)
                        .keyboardType(.numberPad)
                        .textContentType(.oneTimeCode)
                        .font(Theme.display(32))
                        .monospacedDigit()
                        .multilineTextAlignment(.center)
                        .focused($codeFocused)
                        .onChange(of: code) { _, v in
                            let digits = String(v.filter(\.isNumber).prefix(6))
                            if digits != v { code = digits }
                        }
                    ErrorText(message: error)
                    Button {
                        lookup()
                    } label: {
                        HStack {
                            if busy { ProgressView() }
                            Text("Tovább")
                        }
                        .frame(maxWidth: .infinity)
                    }
                    .disabled(busy || code.count < 6)
                } header: {
                    Text("Írd be a tévén látható 6 jegyű kódot")
                } footer: {
                    Text("A tévén vagy a másik eszközön válaszd a „Belépés kóddal” lehetőséget.")
                }
                .listRowBackground(Theme.panel)
            }
        }
        .scrollContentBackground(.hidden)
        .background(Theme.black)
        .navigationTitle("Eszköz csatlakoztatása")
        .navigationBarTitleDisplayMode(.inline)
        .onAppear { codeFocused = true }
        .onAppear { profileID = model.me?.profile?.id ?? 0 }
    }

    private func lookup() {
        busy = true
        error = nil
        Task {
            defer { busy = false }
            do {
                let c = code
                request = try await model.api { try await $0.lookupQuickConnect(c) }
            } catch {
                self.error = error.localizedDescription
            }
        }
    }

    private func decide(approve: Bool) {
        guard let request else { return }
        busy = true
        error = nil
        Task {
            defer { busy = false }
            do {
                let pid = profileID
                if approve {
                    try await model.api { try await $0.approveQuickConnect(request.code, profileID: pid) }
                    approved = true
                } else {
                    try await model.api { try await $0.denyQuickConnect(request.code) }
                    reset()
                }
            } catch {
                self.error = error.localizedDescription
            }
        }
    }

    private func reset() {
        approved = false
        request = nil
        code = ""
        error = nil
        codeFocused = true
    }
}
