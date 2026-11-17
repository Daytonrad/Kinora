import SwiftUI
import KinoraKit

#if os(iOS)
/// The watch party panel beside the video: code and invite, members, chat, reactions.
struct PartyPanel: View {
    @Bindable var party: PartySession
    var leave: () -> Void
    @State private var message = ""
    @FocusState private var typing: Bool

    var body: some View {
        VStack(alignment: .leading, spacing: 12) {
            header
            if let s = party.status {
                Text(s).font(.caption).foregroundStyle(Theme.amber)
            }
            members
            Divider().overlay(Theme.rule)
            chat
            reactions
            composer
        }
        .padding(14)
        .frame(width: 320)
        .frame(maxHeight: .infinity)
        .background(.ultraThinMaterial.opacity(0.95), in: .rect(cornerRadius: 18))
        .environment(\.colorScheme, .dark)
        .padding(.vertical, 8)
        .padding(.trailing, 8)
    }

    private var header: some View {
        HStack(alignment: .firstTextBaseline) {
            VStack(alignment: .leading, spacing: 2) {
                Text("Watch party").font(.caption).foregroundStyle(Theme.muted)
                Text(party.code)
                    .font(.title2.monospaced().weight(.bold))
                    .accessibilityLabel("Kód: " + party.code.map(String.init).joined(separator: " "))
            }
            Spacer()
            ShareLink(item: inviteText, subject: Text("Watch party"), message: Text(inviteText)) {
                Image(systemName: "square.and.arrow.up").frame(width: 36, height: 36).contentShape(.rect)
            }
            .accessibilityLabel("Meghívó küldése")
            Button { party.panelOpen = false } label: {
                Image(systemName: "xmark").frame(width: 36, height: 36).contentShape(.rect)
            }
            .accessibilityLabel("Panel bezárása")
        }
        .buttonStyle(.plain)
    }

    /// Code, app link and web link: friends with the app tap the first link, others open the web player.
    private var inviteText: String {
        var lines = ["Nézzük együtt a Kinorán! Kód: \(party.code)", party.appLink.absoluteString]
        if let web = party.webLink { lines.append(web.absoluteString) }
        return lines.joined(separator: "\n")
    }

    private var members: some View {
        VStack(alignment: .leading, spacing: 6) {
            ForEach(party.members) { m in
                HStack(spacing: 8) {
                    Circle().fill(Color(partyHex: m.color)).frame(width: 9, height: 9)
                    Text(m.name).lineLimit(1)
                    if m.host { Text("házigazda").font(.caption).foregroundStyle(Theme.muted) }
                    if m.buffering { Text("tölt…").font(.caption).foregroundStyle(Theme.amber) }
                }
                .font(.subheadline)
                .accessibilityElement(children: .combine)
            }
            if party.isHost {
                Toggle("Mindenki irányíthat", isOn: Binding(get: { party.everyone }, set: { party.setEveryone($0) }))
                    .font(.caption)
                    .tint(Theme.amber)
            } else if !party.canControl {
                Text("A házigazda irányítja a lejátszást.").font(.caption).foregroundStyle(Theme.muted)
            }
        }
    }

    private var chat: some View {
        ScrollViewReader { proxy in
            ScrollView {
                LazyVStack(alignment: .leading, spacing: 6) {
                    ForEach(party.lines) { line in
                        Group {
                            if let name = line.name {
                                Text("\(Text(name + ": ").bold().foregroundColor(Color(partyHex: line.color ?? ""))))\(line.text)")
                            } else {
                                Text(line.text).italic().foregroundStyle(Theme.muted)
                            }
                        }
                        .font(.footnote)
                        .frame(maxWidth: .infinity, alignment: .leading)
                        .id(line.id)
                    }
                }
            }
            .onChange(of: party.lines.last?.id) { _, id in
                if let id { withAnimation { proxy.scrollTo(id, anchor: .bottom) } }
            }
        }
        .frame(maxHeight: .infinity)
    }

    private var reactions: some View {
        HStack(spacing: 2) {
            ForEach(party.reactions, id: \.self) { e in
                Button(e) { party.react(e) }
                    .font(.title3)
                    .frame(maxWidth: .infinity, minHeight: 36)
                    .accessibilityLabel("Reakció: " + e)
            }
        }
        .buttonStyle(.plain)
    }

    private var composer: some View {
        VStack(spacing: 10) {
            HStack {
                TextField("Üzenet", text: $message)
                    .textFieldStyle(.roundedBorder)
                    .focused($typing)
                    .submitLabel(.send)
                    .onSubmit(send)
                Button("Küldés", action: send).disabled(message.trimmingCharacters(in: .whitespaces).isEmpty)
            }
            Button(role: .destructive, action: leave) {
                Label("Kilépés a watch partyból", systemImage: "rectangle.portrait.and.arrow.right")
                    .font(.footnote.weight(.semibold))
                    .frame(maxWidth: .infinity)
            }
            .buttonStyle(.bordered)
            .tint(Theme.danger)
        }
    }

    private func send() {
        party.chat(message)
        message = ""
        typing = true
    }
}
#endif

/// Reactions float up over the video, the sender's name under the emoji.
struct PartyReactionsLayer: View {
    var party: PartySession

    var body: some View {
        GeometryReader { geo in
            ForEach(party.floating) { f in
                FloatingReaction(item: f)
                    .position(x: geo.size.width * f.x, y: geo.size.height * 0.8)
            }
        }
        .allowsHitTesting(false)
        .accessibilityHidden(true)
    }
}

struct FloatingReaction: View {
    var item: PartySession.Floating
    var big = false
    @State private var up = false

    var body: some View {
        VStack(spacing: 2) {
            Text(item.emoji).font(.system(size: big ? 80 : 44))
            Text(item.name).font(big ? .callout : .caption2).foregroundStyle(.white.opacity(0.8))
        }
        .offset(y: up ? (big ? -420 : -260) : 0)
        .opacity(up ? 0 : 1)
        .onAppear { withAnimation(.easeOut(duration: 2.6)) { up = true } }
    }
}

#if os(iOS)
/// Join a room with the code a friend sent.
struct PartyJoinView: View {
    @Environment(AppModel.self) private var model
    @Environment(\.dismiss) private var dismiss
    @State private var code = ""
    @FocusState private var focused: Bool

    var body: some View {
        NavigationStack {
            Form {
                Section {
                    TextField("ABC234", text: $code)
                        .font(.title2.monospaced())
                        .textInputAutocapitalization(.characters)
                        .autocorrectionDisabled()
                        .focused($focused)
                        .submitLabel(.join)
                        .onSubmit(join)
                } header: {
                    Text("6 karakteres kód")
                } footer: {
                    Text("A kódot a party indítója látja a lejátszóban, a meghívó linkjével pedig egyből csatlakozhatsz.")
                }
            }
            .navigationTitle("Csatlakozás watch partyhoz")
            .navigationBarTitleDisplayMode(.inline)
            .toolbar {
                ToolbarItem(placement: .cancellationAction) { Button("Mégse") { dismiss() } }
                ToolbarItem(placement: .confirmationAction) {
                    if model.partyJoining { ProgressView() } else {
                        Button("Csatlakozás", action: join).disabled(PartySync.normalizedCode(code) == nil)
                    }
                }
            }
            .onAppear { focused = true }
            .onChange(of: model.playerVisible) { _, on in if on { dismiss() } }
        }
    }

    private func join() { model.joinParty(code) }
}
#endif
