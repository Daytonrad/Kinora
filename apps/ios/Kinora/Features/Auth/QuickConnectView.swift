import SwiftUI
import KinoraKit

/// Log in with a 6-digit code approved on a device that is already signed in.
struct QuickConnectView: View {
    @Environment(AppModel.self) private var model
    @State private var start: QuickConnectStart?
    @State private var message: String?
    @State private var failed = false
    @State private var attempt = 0

    var body: some View {
        AuthPage {
            ServerHeader()
            Text("Belépés kóddal").font(Theme.display(40, weight: .bold))

            if let start {
                Text(spaced(start.code))
                    .font(Theme.display(64))
                    .monospacedDigit()
                    .foregroundStyle(Theme.amber)
                    .frame(maxWidth: .infinity)
                    .padding(.vertical, 20)
                    .background(Theme.panel, in: .rect(cornerRadius: 12))
                    .accessibilityLabel(start.code.map(String.init).joined(separator: " "))
                    .textSelection(.enabled)
                if let server = model.server {
                    Text("Egy már bejelentkezett telefonon vagy gépen nyisd meg: \(server.url), és írd be ezt a kódot.")
                        .foregroundStyle(Theme.plaster)
                }
                Text("A webes felületen: profil menü → Eszköz csatlakoztatása")
                    .font(.subheadline).foregroundStyle(Theme.muted)
                HStack(spacing: 10) {
                    if !failed { ProgressView() }
                    Text(message ?? "Várakozás a jóváhagyásra…").foregroundStyle(failed ? Theme.danger : Theme.muted)
                }
                if failed {
                    Button("Új kód kérése") { attempt += 1 }.buttonStyle(PrimaryButtonStyle())
                }
            } else if failed {
                ErrorText(message: message)
                Button("Újra") { attempt += 1 }.buttonStyle(PrimaryButtonStyle())
            } else {
                ProgressView().frame(maxWidth: .infinity).padding(40)
            }

            Button { model.go(.login) } label: { Label("Belépés jelszóval", systemImage: "person") }
                .buttonStyle(SecondaryButtonStyle())
            OtherServerButton()
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
