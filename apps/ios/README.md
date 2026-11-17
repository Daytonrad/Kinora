# Kinora iOS

Natív SwiftUI kliens iPhone-ra és iPadre (iOS 17+). Nem a közös webes felületet csomagolja: a lejátszás AVPlayerrel,
később kép a képben, AirPlay és widgetek is natívan.

```
ios/
  project.yml          XcodeGen-leírás – ebből készül a Kinora.xcodeproj (git-ignorált)
  Config/
    App.xcconfig       verzió, bundle ID, közös build-beállítások
    Local.example.xcconfig → másold Local.xcconfig néven, ide jön a saját Team ID-d
  Kinora/              az app (SwiftUI): App/ (állapot, téma), Features/ (képernyők), Assets.xcassets
  KinoraWidgets/       WidgetKit-bővítmény: Folytatás és Új a könyvtárban
  KinoraKit/           Swift csomag: API-kliens, modellek, szerverlista + Keychain, felderítés – UI nélkül, Macen tesztelhető
```

## Indítás

1. Xcode 16+ és [XcodeGen](https://github.com/yonaskolb/XcodeGen): `brew install xcodegen`
2. Aláírás (csak eszközre telepítéshez, illetve a Keychainhez – lásd lent):
   ```bash
   cp Config/Local.example.xcconfig Config/Local.xcconfig
   ```
   és írd bele a `DEVELOPMENT_TEAM`-et (Xcode → Settings → Accounts, vagy developer.apple.com → Membership).
   A `Local.xcconfig` git-ignorált, a Team ID nem kerül a repóba. Saját Team-mel a `KINORA_BUNDLE_ID`-t is itt írd felül, mert az alapértelmezett bundle ID a hivatalos buildé.
3. `xcodegen` ebben a mappában, majd nyisd meg a `Kinora.xcodeproj`-ot és Run.

Az app és a widgetek egy App Groupon (`group.<bundle ID>`) osztoznak: ebben van a szerverlista, a widgetek utolsó
adata és a Keychain-hozzáférési csoport. Automatikus aláírásnál az Xcode a saját Team-edhez létrehozza a csoportot,
kézzel semmit nem kell beállítani a developer.apple.com-on. Saját bundle ID-vel a csoport neve is vele változik.

A `project.yml` módosítása után futtasd újra az `xcodegen`-t. A generált `.xcodeproj` és `Kinora/Info.plist` nincs verziókezelve.

Parancssorból:

```bash
swift test --package-path KinoraKit
```

```bash
xcodebuild -project Kinora.xcodeproj -scheme Kinora -sdk iphonesimulator -destination 'generic/platform=iOS Simulator' -derivedDataPath build build
```

A CI ugyanezt futtatja `CODE_SIGNING_ALLOWED=NO`-val.

**Keychain a simulatoron:** aláírás nélküli builddel (`CODE_SIGNING_ALLOWED=NO`) az appnak nincs Keychain-jogosultsága, így a token nem
mentődik, és újraindítás után újra be kell lépni. A belépés megmaradásának kipróbálásához normál (aláírt) builddel futtasd.

## Tesztszerver

A repó gyökerében egy helyi szerver (`go run ./cmd/kinora …` vagy a Docker dev konténer) elérhető a simulatorból
`http://localhost:8096` címen. A simulator ugyanazon a gépen fut, port-továbbítás nem kell. Telefonról a Mac LAN-címével
(`http://192.168.x.y:8096`), vagy a „Szerver keresése” gombbal.

Távoli szerverhez HTTPS kell (reverse proxy, Cloudflare, Tailscale): az App Transport Security csak helyi címekre
(`NSAllowsLocalNetworking`: IP-címek és `.local` nevek) engedi a sima HTTP-t.

## Ütemterv

- **v0.1** (kész): szerver hozzáadása, keresés a helyi hálózaton, belépés jelszóval vagy kóddal (Quick Connect), profilválasztó PIN-nel.
- **v0.2** (kész): kezdőlap, könyvtárak rendezéssel, részletek, sorozatok évadokkal, keresés, kedvenc és megnézett jelölés.
- **v0.3** (kész): saját AVPlayer-lejátszó (direct play, remux és átkódolt HLS, tekerés a még el nem készült részre új
  munkamenettel), haladás mentése, következő rész visszaszámlálással, intro/összefoglaló/stáblista átugrása, kép a képben,
  AirPlay, vezérlés a zárolási képernyőről. Feliratok még nincsenek.
- **v0.4** (kész): widgetek. „Folytatás” (kicsi, közepes, nagy, és a zárolási képernyőre is): koppintásra ott folytatja,
  ahol abbahagytad. „Új a könyvtárban” (kicsi, közepes, nagy): a legújabb címek, koppintásra a részletek. Ha a szerver
  nem érhető el, az utolsó adatot mutatják. Linkek: `kinora://item/<id>` és `kinora://play/<id>`. Fekvő telefonon a
  nagy borítókép nem tölti ki a képernyőt.
- Később: offline letöltés, watch party, tvOS.

## Licenc

MIT, mint a Kinora.
