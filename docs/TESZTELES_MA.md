# Kinora appok kipróbálása – ma, Windowsról

Lépésről lépésre: **Android telefon**, **Android TV**, **iPhone** (webes appként), **LG webOS TV** és **Samsung Tizen TV**.
Minden parancs Windows PowerShellben vagy a Git Bashben fut, a repó gyökeréből (`C:\Users\Máté\Desktop\Kinora`), hacsak nincs másképp jelölve.

> Rövid változat: 1) frissítsd a szervert (0. lépés), 2) a telefonra másold fel az `apps\android\dist\kinora.apk`-t,
> 3) a tévén nyomd meg a „Belépés kóddal” gombot, és a telefonon hagyd jóvá.

---

## 0. A szerver frissítése (mindenhez kell)

Az appok olyan új szerverfunkciókat használnak, amelyek a GitHubon lévő régi image-ben még nincsenek benne
(eszközök, Quick Connect, stream-tokenek, CORS). Ezért a szervert a repó mostani kódjából kell elindítani:

```powershell
cd C:\Users\Máté\Desktop\Kinora
docker compose -f docker-compose.yml -f docker-compose.dev.yml up -d --build
```

Ellenőrzés: a böngészőben a `http://localhost:8096/api/status` válaszában szerepeljen a `"product":"kinora"` és az `"api_version":2`.

**A szerver IP-címe**, amit az appokba be kell írni: PowerShellben `ipconfig`, és a Wi-Fi vagy Ethernet adapter
„IPv4 Address” sora (pl. `192.168.1.23`). A `172.x.x.x` címek a Docker és a WSL belső hálózatai, azok nem jók.

**Windows tűzfal:** a Docker Desktop általában maga megnyitja a 8096-os portot. Ha a telefon mégsem éri el a szervert,
rendszergazdai PowerShellben:

```powershell
New-NetFirewallRule -DisplayName "Kinora 8096" -Direction Inbound -Protocol TCP -LocalPort 8096 -Action Allow -Profile Private
```

A Wi-Fi hálózatod Windowsban legyen „Magánhálózat” (Beállítások → Hálózat és internet → Wi-Fi → a hálózat → Hálózati profil típusa).

**„Szerver keresése” gomb:** Windowson a Docker Desktop nem engedi át a hálózati broadcastot, ezért ott ez a gomb nem fogja
megtalálni a szervert. Írd be kézzel az IP-címet; a port 8096 magától hozzáadódik. Linuxon `network_mode: host` mellett működik.

---

## 1. Mit kell telepíteni előtte?

| Platform | Mi kell a PC-re | Már megvan? |
|---|---|---|
| mind | Docker Desktop, Node.js 22 | igen (a gépen ellenőrizve) |
| Android telefon | semmi (az APK fájlként is átmásolható); `adb` csak USB/Wi-Fi telepítéshez | `adb`: **nincs** |
| Android TV | `adb` (Android platform-tools) | **nincs** |
| iPhone | semmi: Safari a telefonon | – |
| LG webOS TV | webOS CLI (a repóban van: `apps\node_modules`), LG fejlesztői fiók | CLI: igen; fiók: neked kell |
| Samsung Tizen TV | Tizen Studio CLI + Samsung tanúsítvány (Samsung-fiók kell) | **nincs** |

Az APK-építéshez **nem** kell JDK vagy Android Studio: a build Dockerben fut (`kinora-android-build` image).

### `adb` telepítése (Android platform-tools)

1. Töltsd le: <https://developer.android.com/tools/releases/platform-tools> → „Download SDK Platform-Tools for Windows”.
2. Csomagold ki ide: `C:\platform-tools`.
3. Add hozzá a PATH-hoz (PowerShell, egyszer):
   ```powershell
   [Environment]::SetEnvironmentVariable("Path", $env:Path + ";C:\platform-tools", "User")
   ```
4. Nyiss új PowerShell ablakot, és próbáld ki: `adb version`.

---

## 2. Az appok elkészítése

Egyszer, az első build előtt:

```powershell
cd C:\Users\Máté\Desktop\Kinora\apps
npm install
```

Utána platformonként egy parancs (mindegyik az `apps\<platform>\dist\` mappába ír):

```powershell
apps\android\build.cmd     # → apps\android\dist\kinora.apk        (telefon + Android TV, egy APK)
apps\webos\build.cmd       # → apps\webos\dist\kinora_1.0.0_all.ipk
apps\tizen\build.cmd       # → apps\tizen\dist\kinora.wgt  (Tizen CLI nélkül: kinora-unsigned.wgt)
```

Az első Android-build 5–10 perc (a Gradle letölti a függőségeit), a későbbiek kb. 1 perc.

---

## 3. Android telefon

**A) Fájlként, kábel nélkül (a legegyszerűbb):**
1. Másold át az `apps\android\dist\kinora.apk`-t a telefonra (USB-kábel, Google Drive, e-mail…).
2. A telefonon koppints rá a Fájlok appban. Ha kéri: „Ismeretlen alkalmazások telepítésének engedélyezése” ennek az appnak → Engedélyezés.
3. Telepítés → Megnyitás.

**B) USB-kábellel:**
1. Telefon: Beállítások → A telefonról → hétszer koppints a „Buildszám”-ra → Fejlesztői beállítások → USB-hibakeresés BE.
2. Kösd a géphez, és a telefonon fogadd el a „USB-hibakeresés engedélyezése?” kérdést.
3. `adb devices` → látszik a telefon? Akkor: `apps\android\build.cmd --install` (vagy `adb install -r apps\android\dist\kinora.apk`).

**Első indítás:**
1. Írd be a szerver IP-címét, pl. `192.168.1.23` → Csatlakozás.
2. Felhasználónév + jelszó → Profil kiválasztása.
3. Próbáld ki: kezdőlap, könyvtár, egy film lejátszása, a lejátszóban felirat/hang/minőség és a „Miért így megy?” panel.

---

## 4. Android TV (pl. Chromecast with Google TV, Nvidia Shield, Sony/Philips Android TV)

Ugyanaz az APK. Az app magától TV-módba vált, és a TV kezdőképernyőjén Kinora bannerrel jelenik meg.

1. TV: Beállítások → Rendszer → Névjegy → hétszer kattints a „Android TV operációs rendszer buildje” sorra → megjelenik a Fejlesztői beállítások menü.
2. Fejlesztői beállítások → **USB-hibakeresés BE**, és ha van: **Hálózati hibakeresés / ADB over network BE**.
3. A TV IP-címe: Beállítások → Hálózat → (a hálózatod) → IP-cím.
4. PC-n:
   ```powershell
   adb connect 192.168.1.50:5555     # a TV IP-címe; a TV-n fogadd el a kérdést („Mindig engedélyezés”)
   adb install -r apps\android\dist\kinora.apk
   ```
5. Indítás: a TV kezdőképernyőjén Alkalmazások → Kinora.

Belépés **kóddal** (ajánlott, nem kell gépelni):
1. A TV-n: szerver IP-címe (a képernyőn van számbillentyűzet) → **Belépés kóddal**: megjelenik egy 6 jegyű kód és egy QR-kód.
2. A telefonon olvasd be a QR-kódot (vagy a böngészőben nyisd meg a `http://<szerver-ip>:8096` címet → profil menü → **Eszköz csatlakoztatása**), írd be a kódot.
3. Válaszd ki, melyik profillal lépjen be a TV → **Jóváhagyás**. A TV pár másodpercen belül belép.

A telefonos Kinora appban is ugyanígy jóváhagyhatod: Beállítások → Eszköz csatlakoztatása.

---

## 5. iPhone (webes appként, Safariból)

Natív iOS appot Windowson nem lehet fordítani (Mac + Xcode kell, lásd `apps/README.md`). Ma a webkliens **telepíthető webes appként** fut:

1. iPhone-on nyisd meg a **Safarit** (nem Chrome-ot): `http://192.168.1.23:8096` (a szerver IP-címe).
2. Lent középen **Megosztás** gomb (négyzet felfelé nyíllal) → görgess le → **Főképernyőhöz adás** → Hozzáadás.
3. A főképernyőn megjelenik a Kinora ikon; innen indítva teljes képernyőn, címsor nélkül fut.
4. Lépj be, és próbáld ki a lejátszást (az iPhone HLS-t natívan játszik le).

---

## 6. LG webOS TV

### 6.1 Egyszeri előkészítés

1. **LG fejlesztői fiók:** regisztrálj a <https://webostv.developer.lge.com> oldalon (ingyenes).
2. **Developer Mode app a TV-re:** a TV-n nyisd meg az LG Content Store-t, keresd meg a **„Developer Mode”** appot, és telepítsd.
3. Indítsd el a Developer Mode appot, lépj be az LG fejlesztői fiókkal, kapcsold be a **Dev Mode Status: ON**-t. A TV újraindul.
4. Újraindítás után nyisd meg újra a Developer Mode appot, és kapcsold be a **Key Server: ON**-t. Itt látod a TV IP-címét is.
5. A PC-n add hozzá a TV-t (a webOS CLI már a repóban van, ezért `npx`-szel fut):
   ```powershell
   cd C:\Users\Máté\Desktop\Kinora\apps
   npx ares-setup-device
   ```
   Válaszd: **add** → device name: `tv` → IP address: a TV IP-címe → port: `9922` → ssh user: `prisoner` → description bármi → authentication: `password`, a jelszót hagyd üresen → Save.
6. Kulcs letöltése a TV-ről (a Developer Mode appban látszó **Passphrase**-zel, pl. `A1B2C3`):
   ```powershell
   npx ares-novacom --device tv --getkey
   ```
   Írd be a passphrase-t. Ellenőrzés: `npx ares-device-info -d tv`.

### 6.2 Telepítés és indítás

```powershell
apps\webos\build.cmd --install
```

Ez elkészíti az `apps\webos\dist\kinora_1.0.0_all.ipk`-t, telepíti (`ares-install -d tv …`), és elindítja (`ares-launch`).
Kézzel: `npx ares-install -d tv apps\webos\dist\kinora_1.0.0_all.ipk`, majd `npx ares-launch -d tv io.github.scotylucas.kinora`.

### 6.3 A Dev Mode lejár

A fejlesztői munkamenet **50 óra** után lejár, és a fejlesztőként telepített appok eltűnnek a TV-ről. Hosszabbítás:
nyisd meg a TV-n a Developer Mode appot → **Extend** gomb (a hátralévő idő a „Remain Session” mezőben látszik).
Ha már lejárt: lépj be újra a Developer Mode appba, kapcsold be a Dev Mode-ot, és telepítsd újra az appot (`apps\webos\build.cmd --install`).

---

## 7. Samsung Tizen TV

A Samsung TV csak **Samsung-tanúsítvánnyal aláírt** appot telepít, ezért itt kell a legtöbb előkészítés.

### 7.1 Tizen Studio CLI telepítése

1. Töltsd le: <https://developer.tizen.org/development/tizen-studio/download> → **„Tizen Studio … with CLI”** → Windows 64 bit (a CLI-s telepítő elég, IDE nem kell).
2. Telepítsd ide: `C:\tizen-studio`. Ha a telepítő Javát hiányol, telepítsd az Eclipse Temurin 17-et
   (<https://adoptium.net>, „JDK 17 – Windows x64 .msi”, a telepítőben „Set JAVA_HOME” bekapcsolva), majd futtasd újra.
3. Indítsd el a **Package Manager**-t (`C:\tizen-studio\package-manager\package-manager.exe`) → **Extension SDK** fül →
   telepítsd: **„Samsung Certificate Extension”** és **„TV Extensions-…”** (a legújabbat).
4. Add a PATH-hoz:
   ```powershell
   [Environment]::SetEnvironmentVariable("Path", $env:Path + ";C:\tizen-studio\tools;C:\tizen-studio\tools\ide\bin", "User")
   ```
   Új PowerShell ablakban: `tizen version` és `sdb version`.

### 7.2 A TV fejlesztői módja

1. A TV-n nyisd meg az **Alkalmazások** (Apps) képernyőt.
2. A távirányítón nyomd be: **1 2 3 4 5** (újabb, számgomb nélküli távirányítón: a képernyőn megjelenő számbillentyűzettel).
3. A megjelenő ablakban: **Developer mode: ON**, és **Host PC IP**: a géped IP-címe (lásd 0. lépés) → OK.
4. **Indítsd újra a TV-t** (ki-be kapcsolás a távirányítóval nem elég: tartsd lenyomva a bekapcsoló gombot, amíg újraindul, vagy húzd ki 30 másodpercre).
5. Újraindítás után az Apps képernyő tetején megjelenik a „Developer Mode” felirat.

### 7.3 Csatlakozás és a Samsung-tanúsítvány

1. A TV IP-címe: Beállítások → Általános → Hálózat → Hálózati állapot → IP-beállítások.
2. Csatlakozás:
   ```powershell
   sdb connect 192.168.1.60        # a TV IP-címe
   sdb devices                     # pl. 192.168.1.60:26101   device   UE55...
   ```
3. Tanúsítvány (egyszer kell): indítsd el a **Certificate Manager**-t (`C:\tizen-studio\tools\certificate-manager\certificate-manager.exe`) →
   **+** → **Samsung** → Device type: **TV** → Certificate profile name: **`kinora`** → Create a new author certificate (adj meg nevet és jelszót) →
   lépj be a **Samsung-fiókoddal** → Distributor certificate: hagyd az alapértelmezettet (Public), a **DUID**-nál a csatlakoztatott TV-d automatikusan
   megjelenik (ha nem: az `sdb devices` szerint csatlakoztatva kell lennie) → Finish.
4. A profil neve `kinora` legyen; ha mást adsz meg, állítsd be: `$env:KINORA_TIZEN_PROFILE="profilnev"`.

### 7.4 Csomagolás, telepítés, indítás

```powershell
$env:KINORA_TV_IP="192.168.1.60"
apps\tizen\build.cmd --install
```

Ez aláírja a `.wgt`-t (`tizen package -t wgt -s kinora`), telepíti (`tizen install`), és elindítja (`tizen run -p KinoraTV01.Kinora`).
Kézzel ugyanez:

```powershell
tizen package -t wgt -s kinora -- apps\tizen\www
tizen install -n apps\tizen\dist\kinora.wgt -t 192.168.1.60:26101
tizen run -p KinoraTV01.Kinora -t 192.168.1.60:26101
```

Ezután az app a TV Apps képernyőjén is megtalálható.

---

## 8. Ha valami nem megy

| Tünet | Megoldás |
|---|---|
| „Nem érem el a szervert (…)” | Ugyanazon a Wi-Fin van a telefon/TV és a PC? Jó IP-t írtál (nem `172.x`)? Tűzfal (0. lépés)? Böngészőből a telefonon megnyílik a `http://<ip>:8096`? |
| „A(z) … címen nem Kinora-szerver válaszol” | Rossz port vagy egy másik szolgáltatás fut ott. A Kinora alapportja 8096. |
| „A szerver túl régi ehhez az apphoz” | A 0. lépés kimaradt: építsd újra a szervert a repóból. |
| A kód nem jelenik meg / „ki van kapcsolva” | Admin → Beállítások → „Belépés kóddal (Quick Connect)” legyen Bekapcsolva. |
| „Ismeretlen vagy lejárt kód” | A kód 5 percig él, és csak egyszer használható. A TV-n: „Új kód kérése”. |
| „Túl sok kódbeírási kísérlet” | 5 percen belül 10 próbálkozás után vár a szerver. Várj 5 percet. |
| A videó nem indul | A lejátszóban az ⓘ gomb („Miért így megy?”) megmutatja, mit választott a szerver. Ha direct play hibázik: „Próbáld átkódolva”. |
| Android: „Az alkalmazás nincs telepítve” | Egy régebbi, más aláírással telepített Kinora van fent: töröld, és telepítsd újra. |
| Android TV: `adb connect` „failed to authenticate” | A TV-n fogadd el a hibakeresési kérdést; ha nem jön fel: Fejlesztői beállítások → USB-hibakeresési engedélyek visszavonása, majd újra `adb connect`. |
| webOS: `ares-install` „Unknown device” vagy időtúllépés | Fut a Developer Mode app, és a Key Server ON? Lejárt a session (6.3)? IP változott → `npx ares-setup-device` → modify. |
| webOS: „Error: … ssh … key” | A kulcs újragenerálódott: `npx ares-novacom --device tv --getkey` újra a friss passphrase-zel. |
| Tizen: `sdb connect` „unable to connect” | A Developer mode „Host PC IP” nem a géped mostani IP-címe, vagy a TV-t nem indítottad újra a bekapcsolás után. |
| Tizen: „install failed … signature / certificate” | A tanúsítvány nem ehhez a TV-hez készült (DUID). Certificate Manager → a `kinora` profil → distributor tanúsítvány újra, csatlakoztatott TV-vel. |
| Tizen: „kinora-unsigned.wgt” készült | Nem találta a Tizen CLI-t: PATH (7.1/4), vagy `$env:TIZEN_STUDIO="C:\tizen-studio"`. |
| TV-n nincs hang AC3/EAC3 filmnél | A TV-k többsége csak HDMI ARC-on át adja tovább; a „Hang” menüben válassz AAC sávot, vagy a minőségmenüben egy átkódolt minőséget. |

Visszajelzéshez hasznos: a lejátszó „Miért így megy?” panelje és a szerver naplója (`docker logs kinora --tail 100`).
