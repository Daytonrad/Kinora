# Kinora Windowson (Windows Server is), Docker nélkül

A Kinora natívan, Windows-szolgáltatásként fut. Docker Desktop nem kell, ezért Windows Serveren is működik.
A videókártyát közvetlenül éri el: NVIDIA (NVENC), Intel (QSV) és AMD (AMF) hardveres kódolás is használható.

## Telepítés

Rendszergazdai PowerShellben:

```powershell
$u = "https://github.com/ScotyLucas/kinora/releases/latest/download/kinora-windows-amd64.zip"
Invoke-WebRequest $u -OutFile kinora.zip
Invoke-WebRequest "$u.sha256" -OutFile kinora.zip.sha256
Unblock-File kinora.zip
Expand-Archive kinora.zip kinora-setup
powershell -ExecutionPolicy Bypass -File .\kinora-setup\install.ps1 -ZipPath kinora.zip -Media "Filmek=D:\Filmek;Sorozatok=D:\Sorozatok"
```

Ezután a szerver itt érhető el: `http://<szerver-ip>:8096`.

Az `install.ps1` a következőket végzi el:
- ellenőrzi a csomag SHA-256 összegét (`-ZipPath` nélkül maga tölti le a legújabb kiadást);
- a programot a `C:\Program Files\Kinora`, az adatokat a `C:\ProgramData\Kinora` mappába teszi;
- telepíti és elindítja a **Kinora** szolgáltatást (automatikus, késleltetett indítás, összeomlás után újraindul);
- megnyitja a tűzfalon a 8096/TCP (webes felület) és a 7359/UDP (az appok szerverkeresése) portot, tartományi és magánhálózati profilon;
- felvesz egy **Kinora frissítés** nevű ütemezett feladatot.

Paraméterek: `-InstallDir`, `-DataDir`, `-Port`, `-Media` (csak az első indításkor számít),
`-UpdateTime` (alapból `04:00`), `-NoAutoUpdate`, `-ZipPath` (helyi zip a letöltés helyett).
Újrafuttatva frissíti a programot, a meglévő adatokhoz nem nyúl.

## Automatikus frissítés

Az ütemezett feladat naponta egyszer (04:00 és 04:30 között) SYSTEM-ként futtatja az `update.ps1`-et:

- **csak kiadott verzióra frissít** (GitHub Release, `v*.*.*`), a `main` ágra pusholt változtatásokra nem;
- ellenőrzi a letöltött csomag SHA-256 összegét;
- leállítja a szolgáltatást, és elmenti a programot meg az adatbázist (`kinora.db*`) a `C:\Program Files\Kinora\backup` mappába;
- kicseréli a programot, elindítja, és megvárja, hogy a `/api/status` válaszoljon;
- **ha az új verzió 90 másodpercen belül nem válaszol, visszaállítja** a régi programot és az adatbázist, majd újraindítja.

A frissítés naplója: `C:\Program Files\Kinora\update.log`. A szerver naplója: `C:\ProgramData\Kinora\kinora.log`.

Kézi frissítés (rendszergazdaként):

```powershell
& "C:\Program Files\Kinora\update.ps1"          # csak ha van újabb kiadás
& "C:\Program Files\Kinora\update.ps1" -Force   # a legújabbat akkor is újratelepíti
```

## Médiamappák és hálózati meghajtók

A szolgáltatás a **LocalSystem** fiókkal fut. Helyi meghajtót (`D:\Filmek`) gond nélkül olvas.
Hálózati megosztáshoz UNC útvonalat adj meg (`\\nas\Filmek`), ne meghajtóbetűt, mert a csatlakoztatott
hálózati meghajtók (`Z:`) csak a bejelentkezett felhasználónak látszanak. A megosztásnak pedig engedélyeznie kell
a szerver számítógépfiókját (`TARTOMÁNY\SZERVER$`).

Az adatmappát (`-DataDir`) mindig helyi lemezre tedd, mert az SQLite adatbázis hálózati meghajtón nem megbízható.

## Eltávolítás

```powershell
& "C:\Program Files\Kinora\kinora.exe" service uninstall
Unregister-ScheduledTask -TaskName "Kinora frissítés" -Confirm:$false
Get-NetFirewallRule -DisplayName "Kinora*" | Remove-NetFirewallRule
Remove-Item -Recurse "C:\Program Files\Kinora"
# az adatok megmaradnak itt: C:\ProgramData\Kinora
```

## A csomag tartalma

A `kinora-windows-amd64.zip` minden `v*.*.*` tagnél automatikusan elkészül, és felkerül a GitHub Release-be
(`.github/workflows/ci.yml`, `windows` job). Tartalma: `kinora.exe` (benne a webes felület), `ffmpeg\ffmpeg.exe` és
`ffprobe.exe` (BtbN 8.1-es GPL build), valamint az `install.ps1` és az `update.ps1`.
