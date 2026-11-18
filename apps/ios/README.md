# Kinora iOS

Natív SwiftUI kliens iPhone-ra és iPadre (iOS 17+), valamint Apple TV-re (tvOS 17+). Nem a közös webes felületet csomagolja: a lejátszás AVPlayerrel,
később kép a képben, AirPlay és widgetek is natívan.

```
ios/
  project.yml          XcodeGen-leírás – ebből készül a Kinora.xcodeproj (git-ignorált)
  Config/
    App.xcconfig       verzió, bundle ID, közös build-beállítások
    Local.example.xcconfig → másold Local.xcconfig néven, ide jön a saját Team ID-d
  Kinora/              az app (SwiftUI): App/ (állapot, téma), Features/ (képernyők), Assets.xcassets
  KinoraWidgets/       WidgetKit-bővítmény: Folytatás és Új a könyvtárban
  KinoraTV/            Apple TV app: saját, Siri Remote-ra és fókuszra írt képernyők; a modellt, a lejátszó logikáját
                       és a borítókép-gyorsítótárat a Kinora/ mappából fordítja (ugyanaz a bundle ID: universal purchase)
  KinoraTopShelf/      tvOS Top Shelf-bővítmény: Folytatás és Új a könyvtárban a kezdőképernyő tetején
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

Apple TV:

```bash
xcodebuild -project Kinora.xcodeproj -scheme KinoraTV -sdk appletvsimulator -destination 'generic/platform=tvOS Simulator' -derivedDataPath build build
```

A CI mindkettőt futtatja `CODE_SIGNING_ALLOWED=NO`-val.

**Keychain a simulatoron:** aláírás nélküli builddel (`CODE_SIGNING_ALLOWED=NO`) az appnak nincs Keychain-jogosultsága, így a token nem
mentődik, és újraindítás után újra be kell lépni. A belépés megmaradásának kipróbálásához normál (aláírt) builddel futtasd.

## Tesztszerver

A repó gyökerében egy helyi szerver (`go run ./cmd/kinora …` vagy a Docker dev konténer) elérhető a simulatorból
`http://localhost:8096` címen. A simulator ugyanazon a gépen fut, port-továbbítás nem kell. Telefonról a Mac LAN-címével
(`http://192.168.x.y:8096`), vagy a „Szerver keresése” gombbal.

Az app bármilyen címre enged sima HTTP-t (`NSAllowsArbitraryLoads`), mert a szervert a felhasználó adja meg, és
sok otthoni szerver nyilvános címen is HTTP-n válaszol. Interneten át viszont ilyenkor a jelszó és a token titkosítatlanul
megy, ezért távoli eléréshez HTTPS ajánlott (reverse proxy, Cloudflare, Tailscale).

## Apple TV

- Képernyők: szerverválasztó (helyi keresés vagy cím), belépés kóddal vagy jelszóval, profilok PIN-nel, majd
  Kezdőlap (kiemelt cím, sorok), Könyvtárak (rendezés, rács), Keresés, Beállítások (profilváltás, feliratméret, kilépés).
  Hosszú kattintás egy kártyán: lejátszás, elölről, adatlap.
- Lejátszó a Siri Remote-ra: kattintás vagy play/pause = szünet, balra/jobbra ±10 mp, Menu = vezérlők elrejtése, majd
  kilépés. Az intro/stáblista átugrása és a következő rész gomb megjelenéskor fókuszt kap. Menük: felirat (és időzítés),
  hangsáv, fejezetek, sebesség és minőség.
- **Vezérlés iPhone-ról, zárolt képernyőn is:** a lejátszó Now Playing adatot (cím, borítókép, idő) tesz közzé és
  fogadja a távoli parancsokat. Az iPhone a zárolási képernyőn és a Vezérlőközpontban felajánlja a tévé lejátszását,
  az Apple TV Remote pedig ugyanúgy vezérli, mint a Siri Remote. A telefonra ehhez nem kell semmi külön.
- **Top Shelf:** ha a kezdőképernyőn a Kinora ikonján áll a fókusz, felül nagyban látszik a „Folytatás” (plakátok
  haladássávval, kattintásra ott folytatja) és az „Új a könyvtárban” (plakátok, kattintásra az adatlap). A bővítmény az
  App Groupon át az app belépését használja, a képeket letölti a közös tárolóba, és szerver nélkül az utolsó állapotot
  mutatja. Az app ugyanott frissíti, ahol az iPhone a widgeteket.
- Letöltés, kép a képben és widget tvOS-en nincs.
- Az ikon rétegei és a Top Shelf képek az `apps/branding/brand.mjs`-ből készülnek.

## Watch party

Közös nézés a szerver szobáin át, ugyanaz a protokoll és szinkronszabály, mint a weben, így app és böngésző egy szobában lehet.

- **Indítás:** az adatlapon („Watch party indítása” filmnél és epizódnál), vagy lejátszás közben a beállítások menüből
  (ilyenkor a szoba onnan indul, ahol épp tartasz). Apple TV-n az adatlap „Watch party” gombja vagy a sebesség/minőség menü.
- **Csatlakozás:** a kezdőlap fejlécének gombja. A képernyő felső része kamera: a házigazda QR-kódját (telefonon a
  panelen, Apple TV-n a lejátszó sarkában) beolvasva egyből csatlakozik, zseblámpával sötétben is. Alatta a 6 karakteres
  kód beírható (6 karakternél magától indul, beillesztett meghívó linkből is kiveszi a kódot). Kamera-engedély nélkül vagy
  kamera nélküli eszközön csak a kódmező látszik, a Beállításokra mutató gombbal. A QR-kód a `kinora://party/<KÓD>` link,
  így a telefon saját kamerája is az appba nyitja; a beolvasó a webes linket (`…/#/party/<KÓD>`) is elfogadja.
  Apple TV-n a Beállítások → „Csatlakozás watch partyhoz”. Akinek nincs appja, a meghívóban lévő webes linket nyitja meg.
- **Panel (iPhone, iPad):** kód, QR-kód (amíg egyedül vagy, magától látszik) és meghívó küldése, résztvevők (házigazda, ki tölt), „Mindenki irányíthat” kapcsoló a
  házigazdának, csevegés olvasatlan-jelzővel, reakciók a videó fölött úszva, kilépés.
- **Apple TV:** a kód, a QR-kód és a résztvevők a jobb felső sarokban (amíg más nincs a szobában, a QR-kód végig látszik), a csevegés és a reakciók pár másodpercig a videó fölött; a
  Watch party menüben reakció, üzenet írása, „Mindenki irányíthat” és kilépés.
- **Szinkron:** a szerver órája a mérvadó (ping/pong alapján becsülve); 1,5 mp-nél nagyobb eltérésnél tekerés, kisebbnél
  5%-kal gyorsabb vagy lassabb lejátszás. Ha valaki tölt, a szoba megvárja.
- Party alatt a sebesség 1×, nincs automatikus átugrás és következő rész visszaszámlálás, a letöltött példány helyett a
  szerverről játszik. Az átugrás és a következő rész gombját csak az látja, aki irányíthat.

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
- **v0.5**: beállítások a fiókmenüből, mint a webes `/settings`: lejátszás (feliratok, nyelvek, maximális minőség,
  következő rész), intró átugrása, hang a háttérben, bejelentkezett eszközök kiléptetése, tévé jóváhagyása kóddal
  („Eszköz csatlakoztatása”), külső szervercím hozzáadása.
- **v0.6** (kész): hang- és feliratválasztás, felirat stílusa és késleltetése, offline letöltés, sebesség, minőségkorlát,
  fejezetek, sorozatonkénti nyelvek, Spotlight; Apple TV app.
- **v0.6.3** (kész): watch party iPhone-on, iPaden és Apple TV-n.

## Licenc

MIT, mint a Kinora.
