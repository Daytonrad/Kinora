# Kinora natívan Linuxon

A natív telepítés a legegyszerűbb út, ha a Kinora ugyanazon a gépen fut, ahol a filmek vannak. Bármelyik mappát látja, amit
a szolgáltatás felhasználója olvashat, és új mappát bármikor hozzáadhatsz az adminfelületen, újraindítás és konfigurációszerkesztés nélkül.

## Telepítés

```sh
curl -fsSL https://raw.githubusercontent.com/ScotyLucas/kinora/main/install.sh | bash
```

vagy letöltve, nem interaktívan:

```sh
bash install.sh --run-as-me --name "Nappali" --port 8096 --logo ~/logo.png --media "Filmek=/mnt/tar/Filmek" --media /mnt/tar/Sorozatok --yes
```

A telepítő:

1. felismeri az architektúrát (amd64/arm64), és letölti a legfrissebb kiadást (`--version v1.2.3` adott verzióhoz);
2. ellenőrzi a `SHA256SUMS` aláírását (ed25519, a nyilvános kulcs a telepítőben van) és a bináris SHA-256 összegét; hibás aláírásnál nem telepít;
3. az `ffmpeg`, `acl`, `openssl` és a chromaprint (`fpcalc`) csomagot a disztró csomagkezelőjével telepíti, ha hiányzik (apt, dnf, pacman, zypper);
4. megkérdezi, ki legyen a **szolgáltatás felhasználója**:
   - **saját felhasználó** (ajánlott otthoni gépen): mindent lát, amit te, nincs jogosultság-állítgatás;
   - **dedikált `kinora` rendszerfelhasználó**: elkülönítettebb, de a médiamappákra jogot kell adni (lásd lent);
5. létrehozza a systemd szolgáltatást, engedélyezi és elindítja.

| Hely | Tartalom |
|---|---|
| `/opt/kinora/bin/kinora` | a program (`/usr/local/bin/kinora` erre mutat); `kinora.prev` az előző verzió visszaállításhoz |
| `/etc/kinora/kinora.env` | beállítások (port, mappák, `KINORA_*` változók); módosítás után `sudo systemctl restart kinora` |
| `/var/lib/kinora` | adatbázis, képek – **ezt mentsd** |
| `/var/cache/kinora` | átkódolás, letöltések, ujjlenyomatok |

A szolgáltatás hardeningje: `NoNewPrivileges`, `ProtectSystem=strict` (minden csak olvasható, kivéve az adat-, cache- és programmappát),
`ProtectHome=read-only`, `PrivateTmp`, kernelvédelmek, `Restart=always`. A `/opt/kinora/bin` azért írható, hogy az adminfelületről
indított frissítés le tudja cserélni a programot. Napló: `journalctl -u kinora -f`.

**Eltávolítás:** `sudo bash install.sh --uninstall`. Az adatokat csak külön megerősítésre törli.

**Csomagból:** a kiadásokhoz `.deb` és `.rpm` is tartozik (`sudo apt install ./kinora_1.2.3_amd64.deb`). Ez mindig a `kinora`
rendszerfelhasználóval fut; saját felhasználóhoz: `sudo systemctl edit kinora` → `[Service]` `User=te` `Group=te`.

## Mappák hozzáadása és jogosultságok

Az **Admin → Könyvtárak → Könyvtár hozzáadása → Tallózás** natív telepítésnél a gyökérből (`/`) indul, gyorsgombokkal
(`/home`, `/mnt`, `/media`, `/srv` és a saját mappád). Az a mappa, amelybe a szolgáltatás nem léphet be, lakattal jelenik meg.

Ha egy mappát nem tud olvasni, a Kinora nem ad hozzá csendben egy üres könyvtárat, hanem kimondja, mi a baj:

> A(z) kinora felhasználónak nincs olvasási joga ehhez: /home/mate/Filmek

és megadja a pontos javító parancsot. A szülőmappához csak „belépési” jog (`x`) kell, a mappához olvasás (`rX`, rekurzívan), a `-d` pedig
az ezután létrejövő fájlokra is érvényes:

```sh
sudo setfacl -m u:kinora:x /home/mate
sudo setfacl -R -m u:kinora:rX /home/mate/Filmek
sudo setfacl -R -d -m u:kinora:rX /home/mate/Filmek
```

Vagy csoporton keresztül (a mappa csoportjába veszi a kinora felhasználót):

```sh
sudo usermod -aG mate kinora
sudo chmod -R g+rX /home/mate/Filmek
sudo systemctl restart kinora
```

A **„Jogosultság ellenőrzése”** gomb a mappaválasztóban és könyvtáranként is megvan; ugyanezt adja a `GET /api/admin/permcheck?path=…` végpont.
A `setfacl` az `acl` csomagban van.

## Docker: utólag hozzáadható mappák

Dockerben a konténer csak azt látja, amit csatoltál. Hogy később a compose fájl szerkesztése nélkül is bármelyik mappát
hozzáadhasd, a `docker-compose.host.yml` csak olvashatóan becsatolja a hoston szokásos helyeket a `/host` alá:

```sh
docker compose -f docker-compose.yml -f docker-compose.host.yml up -d
```

| Hoston | Konténerben |
|---|---|
| `/home` | `/host/home` |
| `/mnt` | `/host/mnt` |
| `/media` | `/host/media` |
| `/srv` | `/host/srv` |
| (opcionálisan `/`) | `/host` |

**Biztonsági mérlegelés:** a konténer (a `PUID` felhasználóval) mindent **olvashat** ezek alatt, amit a hoston a `PUID` olvashat,
például más felhasználók világ-olvasható fájljait a `/home` alatt. Írni nem tud (`:ro`), és a Kinora sosem ír a médiamappákba.
Ha ez túl tág, csak a szükséges mappákat csatold, vagy használd a natív telepítést, mert az egyszerűbb: ott a systemd unit hardeningje
és a szolgáltatás felhasználójának jogai pontosan meghatározzák, mit lát.
Dockerben a javító parancsok a hoston érvényes útvonalat és a `PUID` számot használják (pl. `sudo setfacl -R -m u:1000:rX /home/mate/Filmek`).

## Hálózati megosztások (SMB/NFS)

A megosztást **a hoston** csatold fel, ne a Kinorán belül. A `x-systemd.automount` miatt a rendszer akkor is elindul, ha a NAS épp ki van kapcsolva,
és a megosztás az első hozzáféréskor csatolódik.

`/etc/fstab`, SMB (Windows-megosztás, Samba, Synology):

```
//nas.local/Filmek  /mnt/nas/Filmek  cifs  credentials=/etc/kinora/nas.cred,uid=kinora,gid=kinora,file_mode=0440,dir_mode=0550,iocharset=utf8,vers=3.0,_netdev,nofail,x-systemd.automount,x-systemd.idle-timeout=600  0 0
```

(`/etc/kinora/nas.cred`: `username=…` és `password=…` sorok, `chmod 600`.) NFS:

```
nas.local:/volume1/Filmek  /mnt/nas/Filmek  nfs  ro,_netdev,nofail,x-systemd.automount,x-systemd.idle-timeout=600  0 0
```

Utána: `sudo systemctl daemon-reload && sudo mount -a`, majd az adminfelületen add hozzá a `/mnt/nas/Filmek` mappát.
SMB-nél a jogokat a `uid=`/`gid=`/`file_mode=` adja, nem a `setfacl`.

**Leválasztott megosztásnál a Kinora nem töröl.** Ha a könyvtár gyökere nem érhető el, vagy hirtelen üres lett, a szkennelés leáll, és nem nyúl
semmihez. Egy eltűnt fájl csak „hiányzó” jelölést kap, és amikor visszajön, megmarad a megnézett állapota.
