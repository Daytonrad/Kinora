import AppIntents
import KinoraKit

// Siri, the Shortcuts app, Spotlight and the Action button. Every intent ends in a
// kinora:// link that the app opens the same way it opens a widget tap, so profile
// PIN locks, the login screen and "wait until home" all still apply.

/// Hands links from the intents to the running app; the app attaches itself on launch.
@MainActor
enum IntentRouter {
    static weak var model: AppModel? {
        didSet { if let url = pending, let model { pending = nil; model.open(url) } }
    }
    private static var pending: URL?

    static func open(_ link: KinoraLink) {
        if let model { model.open(link.url) } else { pending = link.url }
    }

    /// "Play <series>" for an episode, so the suggestion always resumes the show.
    static func donate(_ item: Item) {
        let target: TitleEntity
        if item.kind == .episode, let series = item.seriesId {
            target = TitleEntity(id: series, title: item.seriesTitle ?? item.title, subtitle: L("Sorozat"))
        } else if item.kind == .movie {
            target = TitleEntity(item)
        } else { return }
        var intent = PlayTitleIntent()
        intent.target = target
        Task { _ = try? await IntentDonationManager.shared.donate(intent: intent) }
    }

    /// A client with the app's saved login, for looking things up before the app is up.
    static func client() throws -> KinoraClient {
        let store = ServerStore.shared()
        guard let server = store.current, let token = store.token(for: server.id) else {
            throw IntentError.signedOut
        }
        return KinoraClient(server: server, token: token)
    }
}

enum IntentError: Error, CustomLocalizedStringResourceConvertible {
    case signedOut, nothingToResume

    var localizedStringResource: LocalizedStringResource {
        switch self {
        case .signedOut: "\(L("Előbb jelentkezz be a Kinora appban."))"
        case .nothingToResume: "\(L("Nincs félbehagyott film vagy sorozat."))"
        }
    }
}

// MARK: - Titles as Shortcuts parameters

struct TitleEntity: AppEntity {
    static let typeDisplayRepresentation: TypeDisplayRepresentation = "Cím"
    static let defaultQuery = TitleQuery()

    let id: Int
    let title: String
    let subtitle: String?

    var displayRepresentation: DisplayRepresentation {
        DisplayRepresentation(title: "\(title)", subtitle: subtitle.map { "\($0)" })
    }

    init(id: Int, title: String, subtitle: String?) {
        self.id = id; self.title = title; self.subtitle = subtitle
    }

    init(_ item: Item) {
        id = item.id
        if item.kind == .episode {
            title = item.seriesTitle ?? item.title
            subtitle = item.title
        } else {
            title = item.title
            subtitle = [item.kind == .series ? L("Sorozat") : L("Film"), item.year.map(String.init)]
                .compactMap { $0 }.joined(separator: " · ")
        }
    }
}

struct TitleQuery: EntityStringQuery {
    func entities(for identifiers: [Int]) async throws -> [TitleEntity] {
        let client = try await IntentRouter.client()
        var found: [TitleEntity] = []
        for id in identifiers {
            if let detail = try? await client.item(id) { found.append(TitleEntity(detail.item)) }
        }
        return found
    }

    func entities(matching string: String) async throws -> [TitleEntity] {
        let query = string.trimmingCharacters(in: .whitespacesAndNewlines)
        guard !query.isEmpty else { return [] }
        return try await IntentRouter.client().search(query)
            .filter { $0.kind == .movie || $0.kind == .series }
            .prefix(10).map(TitleEntity.init)
    }

    /// What Shortcuts offers before anything is typed: what the user is watching now.
    func suggestedEntities() async throws -> [TitleEntity] {
        let home = try await IntentRouter.client().home()
        return WidgetFeed.continueWatching(home).prefix(8).map(TitleEntity.init)
    }
}

// MARK: - Intents

struct ContinueWatchingIntent: AppIntent {
    static let title: LocalizedStringResource = "Folytatás"
    static let description = IntentDescription("Ott folytatja a lejátszást, ahol legutóbb abbahagytad.")
    static let openAppWhenRun = true

    @MainActor
    func perform() async throws -> some IntentResult {
        let home = try await IntentRouter.client().home()
        guard let next = WidgetFeed.continueWatching(home).first else { throw IntentError.nothingToResume }
        IntentRouter.open(.play(next.id))
        return .result()
    }
}

struct PlayTitleIntent: AppIntent {
    static let title: LocalizedStringResource = "Cím lejátszása"
    static let description = IntentDescription("Elindít egy filmet, sorozatnál a következő részt.")
    static let openAppWhenRun = true

    @Parameter(title: "Cím", requestValueDialog: "Mit szeretnél nézni?")
    var target: TitleEntity

    static var parameterSummary: some ParameterSummary { Summary("\(\.$target) lejátszása") }

    @MainActor
    func perform() async throws -> some IntentResult {
        // A series plays its next episode; with none left, its page opens.
        let detail = try await IntentRouter.client().item(target.id)
        if detail.item.kind == .series || detail.item.kind == .season {
            IntentRouter.open(detail.playNext.map { .play($0.id) } ?? .item(target.id))
        } else {
            IntentRouter.open(.play(target.id))
        }
        return .result()
    }
}

struct OpenTitleIntent: AppIntent {
    static let title: LocalizedStringResource = "Cím megnyitása"
    static let description = IntentDescription("Megnyitja egy film vagy sorozat adatlapját.")
    static let openAppWhenRun = true

    @Parameter(title: "Cím", requestValueDialog: "Melyik címet nyissam meg?")
    var target: TitleEntity

    static var parameterSummary: some ParameterSummary { Summary("\(\.$target) megnyitása") }

    @MainActor
    func perform() async throws -> some IntentResult {
        IntentRouter.open(.item(target.id))
        return .result()
    }
}

struct JoinPartyIntent: AppIntent {
    static let title: LocalizedStringResource = "Csatlakozás watch partyhoz"
    static let description = IntentDescription("Belép egy watch partyba a szobakód alapján.")
    static let openAppWhenRun = true

    @Parameter(title: "Szobakód", requestValueDialog: "Mi a szoba kódja?")
    var code: String

    @MainActor
    func perform() async throws -> some IntentResult {
        guard let code = PartySync.normalizedCode(code) else {
            throw $code.needsValueError("\(L("Ez nem jó kód. Mi a szoba kódja?"))")
        }
        IntentRouter.open(.party(code))
        return .result()
    }
}

// MARK: - Siri phrases (work with no setup; listed in the Shortcuts app)

struct KinoraShortcuts: AppShortcutsProvider {
    static var appShortcuts: [AppShortcut] {
        AppShortcut(intent: ContinueWatchingIntent(), phrases: [
            "Folytasd a \(.applicationName)-t",
            "Folytatás a \(.applicationName) appban",
            "Resume \(.applicationName)",
            "Continue watching in \(.applicationName)",
        ], shortTitle: "Folytatás", systemImageName: "play.circle.fill")
        AppShortcut(intent: PlayTitleIntent(), phrases: [
            "\(\.$target) lejátszása a \(.applicationName) appban",
            "Indítsd el: \(\.$target) a \(.applicationName) appban",
            "Lejátszás a \(.applicationName) appban",
            "Play \(\.$target) in \(.applicationName)",
            "Play something in \(.applicationName)",
        ], shortTitle: "Lejátszás", systemImageName: "film")
        AppShortcut(intent: OpenTitleIntent(), phrases: [
            "\(\.$target) megnyitása a \(.applicationName) appban",
            "Show \(\.$target) in \(.applicationName)",
        ], shortTitle: "Megnyitás", systemImageName: "info.circle")
        AppShortcut(intent: JoinPartyIntent(), phrases: [
            "Watch party a \(.applicationName) appban",
            "Join a watch party in \(.applicationName)",
        ], shortTitle: "Watch party", systemImageName: "person.2.fill")
    }
}
