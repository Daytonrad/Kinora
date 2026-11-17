import SwiftUI
import KinoraKit

/// v0.1 placeholder: proves the session works. Browsing arrives in v0.2.
struct HomeView: View {
    @Environment(AppModel.self) private var model

    var body: some View {
        NavigationStack {
            ScrollView {
                VStack(alignment: .leading, spacing: 20) {
                    BrandMark(size: 30)
                    if let me = model.me, let p = me.profile {
                        HStack(spacing: 16) {
                            ProfileFace(profile: p, size: 64, showsName: false)
                            VStack(alignment: .leading) {
                                Text("Szia, \(p.name)!").font(Theme.display(34, weight: .bold))
                                Text("\(me.serverName) · Kinora \(me.version)").foregroundStyle(Theme.muted)
                            }
                        }
                    }
                    Text("A böngészés és a lejátszás a következő lépésben érkezik.")
                        .foregroundStyle(Theme.plaster)
                        .padding(16)
                        .frame(maxWidth: .infinity, alignment: .leading)
                        .background(Theme.panel, in: .rect(cornerRadius: 10))
                }
                .padding(24)
            }
            .background(Theme.black)
            .toolbar {
                ToolbarItem(placement: .topBarTrailing) {
                    Menu {
                        Button("Profilváltás", systemImage: "person.2") { model.switchProfile() }
                        Button("Másik szerver", systemImage: "server.rack") { model.showServers() }
                        Button("Kijelentkezés", systemImage: "rectangle.portrait.and.arrow.right", role: .destructive) {
                            Task { await model.logout() }
                        }
                    } label: {
                        Image(systemName: "person.crop.circle").font(.title2)
                    }
                    .accessibilityLabel("Fiók")
                }
            }
        }
    }
}
