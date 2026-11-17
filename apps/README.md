# Kinora kliensappok

Egy közös felület (`shared`) három platformra csomagolva: **Android** (telefon + Android TV, egy APK), **Samsung Tizen** (`.wgt`)
és **LG webOS** (`.ipk`). Az **iOS** (iPhone/iPad) app natív SwiftUI, külön projekt: [`ios/README.md`](ios/README.md). A mai kipróbálás lépései: [`docs/TESZTELES_MA.md`](../docs/TESZTELES_MA.md).

```
apps/
  shared/         közös TypeScript app (Vite): API-kliens, állapot, lejátszó, felület – "mobile" és "tv" mód
  android/        Capacitor-projekt (android/), build.mjs → dist/kinora.apk
  ios/            natív SwiftUI app (XcodeGen: project.yml) + KinoraKit Swift csomag – nem npm-munkaterület
  tizen/          config.xml + build.mjs → dist/kinora.wgt
  webos/          appinfo.json + build.mjs → dist/kinora_<ver>_all.ipk
  native-plugin/  a régi Capacitor-iOS KinoraNative pluginja (Swift, SPM) – már nem használja semmi; az Android változat az android/ projektben van
  branding/       logo.svg + banner.svg + brand.mjs: minden ikon, banner és splash egy forrásból
  scripts/        közös build-segédek (parancsfuttatás, zip)
```

## Építés

Egyszer: `cd apps && npm install`. Utána minden platformnak egy parancsa van (Windowson a `build.cmd`/`build.ps1` ugyanezt hívja):

| Platform | Parancs | Kimenet | Ami a géphez kell |
|---|---|---|---|
| Android | `npm run build:android` | `android/dist/kinora.apk` (debug) | Docker **vagy** JDK 21 + Android SDK 36 (`JAVA_HOME`, `ANDROID_HOME`) |
| Android release | `npm run build:release -w android` | `android/dist/kinora-release.apk` | ugyanaz + aláírókulcs env-ben (lásd lent) |
| LG webOS | `npm run build:webos` | `webos/dist/kinora_1.0.0_all.ipk` | semmi (a webOS CLI devDependency) |
| Samsung Tizen | `npm run build:tizen` | `tizen/dist/kinora.wgt`, CLI nélkül `kinora-unsigned.wgt` | Tizen Studio CLI + Samsung-tanúsítvány az aláíráshoz |
| iOS | `cd ios && xcodegen`, majd Xcode (lásd [`ios/README.md`](ios/README.md)) | `ios/Kinora.xcodeproj` | macOS + Xcode 16+ + XcodeGen |
| közös UI | `npm run build:shared`, `npm test`, `npm run e2e -w shared` | `shared/dist/` | E2E-hez Edge vagy Chrome + futó szerver |

A `shared` build egyetlen klasszikus (nem modul) `app.js`-t ad ES2015 szintaxissal és a szükséges polyfillekkel, relatív útvonalakkal,
így `file://`-ról is fut a 2018-as TV-ken (Tizen 4.0 = Chromium 56, webOS 4.x = Chromium 53). A CSS ezért csak flexboxot használ
(nincs grid, flex `gap`, `clamp()`, `aspect-ratio`). A design tokeneket a build a webkliens `style.css`-éből másolja (`scripts/sync-tokens.mjs`).

### Android aláírás (release)

A kulcs soha nem kerül a repóba (`*.jks`, `*.keystore` a `.gitignore`-ban). Kulcs készítése egyszer:

```sh
keytool -genkeypair -v -keystore kinora.jks -alias kinora -keyalg RSA -keysize 4096 -validity 10000
```

Build: `KINORA_KEYSTORE=/út/kinora.jks KINORA_KEYSTORE_PASSWORD=… KINORA_KEY_ALIAS=kinora KINORA_KEY_PASSWORD=… npm run build:release -w android`.
A CI-ban ugyanezek GitHub Secrets-ként szerepelnek (`ANDROID_KEYSTORE_BASE64`, `ANDROID_KEYSTORE_PASSWORD`, `ANDROID_KEY_ALIAS`, `ANDROID_KEY_PASSWORD`).

## Telepítés

- **Android telefon:** az APK-t fájlként átmásolod és megnyitod, vagy `adb install -r android/dist/kinora.apk`.
- **Android TV:** `adb connect <tv-ip>:5555`, majd `adb install -r …`. Az app a leanback launcherben bannerrel jelenik meg, és TV-módban indul.
- **LG webOS:** Developer Mode app a TV-n → `npx ares-setup-device` → `npm run install-tv -w webos`. A Dev Mode session 50 óránként lejár, ilyenkor meg kell hosszabbítani (Extend).
- **Samsung Tizen:** a TV-n fejlesztői mód (Apps → 12345), Samsung-tanúsítvány a Certificate Managerrel, majd `KINORA_TV_IP=… npm run install-tv -w tizen`.
- **iPhone:**
  - Ma, Mac nélkül: a webkliens PWA-ként is telepíthető (Safari → Megosztás → „Főképernyőhöz adás”).
  - Natív app: Mac + Xcode + XcodeGen kell. A Team ID a git-ignorált `ios/Config/Local.xcconfig`-ba kerül, utána `cd ios && xcodegen`, Xcode-ban megnyitni a `Kinora.xcodeproj`-ot → csatlakoztasd az iPhone-t → Run. Részletek: [`ios/README.md`](ios/README.md).
  - Az ingyenes fiókkal aláírt app **7 napig** fut, utána újra kell telepíteni. Tartósan, illetve TestFlightra vagy az App Store-ba fizetős Apple Developer Program (évi 99 USD) kell.
  - A CI (`.github/workflows/apps.yml`) macOS runneren lefuttatja a KinoraKit teszteket (`swift test`), majd XcodeGen után aláírás nélkül lefordítja a simulator buildet.

## Belépés

- **Jelszóval:** a szerver címe után felhasználónév + jelszó. Telefonon ez az alapértelmezett.
- **Kóddal (Quick Connect):** TV-n ez az alapértelmezett. A TV mutat egy 6 jegyű kódot és egy QR-kódot. Egy már bejelentkezett eszközön
  (webkliens: profil menü → Eszköz csatlakoztatása, vagy a telefonos app: Beállítások → Eszköz csatlakoztatása) beírod a kódot,
  kiválasztod a profilt, és jóváhagyod. Gyerekprofilt választva a TV ahhoz a profilhoz kötött marad; PIN-es profilnál a PIN-t a TV-n kell megadni.

A szerver címe lehet `192.168.1.10`, `192.168.1.10:8096`, `nas.local`, `http://…` vagy `https://…`. Ha nincs séma, előbb `http`, ha nincs port, előbb `8096`.
Több szerver menthető, a Beállítások → Szerverváltás menüben lehet köztük váltani.

**Szerver keresése:** Android/Android TV-n UDP-broadcast a 7359-es portra (`who is KinoraServer?`). iOS-en és TV-n HTTP-s pásztázás a saját /24-es alhálózaton
(a TV-k böngészőmotorja nem tud UDP-t küldeni; iOS-en az UDP-broadcast fogadásához külön Apple-jogosultság kellene, ezért a natív app is pásztáz).
**Dockerben** a broadcast nem jut át a bridge hálózaton: a felderítéshez `network_mode: host` kell (vagy írd be kézzel a címet). Kikapcsolás: Admin → Beállítások, vagy `KINORA_DISCOVERY=0`.

## Miért kell a cleartext (HTTP) engedélyezése?

Egy otthoni médiaszerver jellemzően sima HTTP-n érhető el egy helyi IP-címen (`http://192.168.1.10:8096`). Ezt a címet buildeléskor nem lehet tudni,
a platformok alapból viszont csak HTTPS-t engednek:

- **Android:** `network_security_config.xml` → `base-config cleartextTrafficPermitted="true"` és `usesCleartextTraffic`. A `domain-config` csak előre ismert
  hostokra szűkíthetne, egy tetszőleges LAN-IP-re nem. HTTPS-szervereknél a rendszer tanúsítványellenőrzése változatlanul érvényes.
  A Capacitor `androidScheme: "http"`, hogy a WebView ne blokkolja a HTTPS-oldalról induló HTTP-videót (mixed content).
- **iOS:** `NSAllowsLocalNetworking` (helyi és `.local` címek) és `NSLocalNetworkUsageDescription` a helyi hálózati engedélykéréshez (`ios/project.yml`).
  Távoli szerverhez HTTPS kell.
- **Tizen:** `<access origin="*">` és `internet` jogosultság a `config.xml`-ben. **webOS:** a csomagolt webapp alapból bármilyen originre hívhat.

Biztonság: az appok **Bearer tokennel** hívják az API-t (nem sütivel). A szerver csak ezekre az app-originekre engedélyez CORS-t, `Allow-Credentials` nélkül,
és az app-originről érkező kérésből eldobja a sütit. A `<video>` és `<img>` fejlécet nem tud küldeni, ezért ezek egy rövid életű (6 órás), aláírt,
elemre, felhasználóra és eszközre kötött **stream-tokent** kapnak (`?st=`), ami az eszköz kiléptetésével azonnal érvényét veszti.

## Platformismeret a lejátszóban

A lejátszó a `MediaSource.isTypeSupported`/`canPlayType` eredményét egy platformonkénti táblázattal egészíti ki (`shared/src/player/caps.ts`).
A TV-k natív lejátszója sokszor üres választ ad olyan formátumra, amit közvetlenül le tud játszani (MKV, HEVC, AC-3/E-AC-3), és így a direct play minél
gyakrabban működik. HDR10-et csak akkor jelez, ha a platform igazolja (Tizen: `webapis.avinfo.isHdrTvSupport()`, máshol `(dynamic-range: high)`).
HLS: Tizen és webOS natívan a `<video>`-ban, Androidon hls.js, iOS-en AVPlayer (v0.3). A feliratokat az app maga rajzolja ki a szerver WebVTT-jéből, mert a `<track>` a régi TV-ken megbízhatatlan.

## Tudatosan későbbre hagyva (v2)

- **Offline letöltés a telefonra:** a szerver már készít átkódolt letöltéseket (`/api/downloads`). A tárolás natív fájlba (Capacitor Filesystem) és az offline
  lejátszás a következő kör; a lejátszó URL-alapú, így egy helyi fájl-URL-re cserélhető.
- Watch party az appokból (a WebSocket jelenleg sütis hitelesítésű).
- Chromecast vezérlés. (AirPlay és kép a képben az iOS app v0.3-as körében.)
