#!/usr/bin/env bash
# Kinora – natív telepítés Linuxra (systemd szolgáltatásként)
#
#   curl -fsSL https://raw.githubusercontent.com/ScotyLucas/kinora/main/install.sh | bash
#   bash install.sh [opciók]
#
# Opciók (mind elhagyható; elhagyva interaktívan kérdez):
#   --name "Máté Mozi"        a szerver neve
#   --port 8096               HTTP port
#   --logo /út/logo.png       saját logó (PNG/JPG/WebP/SVG, legfeljebb 2 MB)
#   --media /mnt/Filmek       kezdő médiamappa (többször is megadható; "Név=/út" formában névvel)
#   --run-as-me | --run-as-kinora
#                             saját felhasználóként fusson (ajánlott otthoni gépen: mindent lát, amit te),
#                             vagy dedikált "kinora" rendszerfelhasználóként
#   --version v1.2.3          adott verzió (alapból a legújabb kiadás)
#   --yes                     nem interaktív: minden kérdésre az alapértelmezés
#   --uninstall               eltávolítás (az adatokat csak külön megerősítésre törli)
#
# Mit csinál: felismeri az architektúrát (amd64/arm64), letölti a kiadást a GitHubról, ellenőrzi
# a SHA-256 összeget és az ed25519 aláírást, telepíti az ffmpeg-et a disztró csomagkezelőjével
# (apt/dnf/pacman), létrehozza a systemd szolgáltatást (Restart=always, hardening), és elindítja.
set -euo pipefail

REPO="ScotyLucas/kinora"
# Release signing public key (raw ed25519, base64). The private key exists only in the CI secrets.
# The release workflow stamps the published installer with KINORA_SIGNING_PUBKEY; this is the default.
PUBKEY_B64="__KINORA_SIGNING_PUBKEY__"

OPT=/opt/kinora
BIN=$OPT/bin
DATA=/var/lib/kinora
CACHE=/var/cache/kinora
ETC=/etc/kinora
UNIT=/etc/systemd/system/kinora.service

NAME="" PORT="" LOGO="" RUNAS="" VERSION="" YES=0 UNINSTALL=0
MEDIA=()

say()  { printf '\033[33m›\033[0m %s\n' "$*"; }
ok()   { printf '\033[32m✓\033[0m %s\n' "$*"; }
die()  { printf '\033[31m✗ %s\033[0m\n' "$*" >&2; exit 1; }

while [ $# -gt 0 ]; do
    case "$1" in
        --name) NAME="$2"; shift 2 ;;
        --port) PORT="$2"; shift 2 ;;
        --logo) LOGO="$2"; shift 2 ;;
        --media) MEDIA+=("$2"); shift 2 ;;
        --run-as-me) RUNAS=me; shift ;;
        --run-as-kinora) RUNAS=kinora; shift ;;
        --version) VERSION="$2"; shift 2 ;;
        --yes|-y) YES=1; shift ;;
        --uninstall) UNINSTALL=1; shift ;;
        -h|--help) sed -n '2,24p' "$0"; exit 0 ;;
        *) die "ismeretlen opció: $1 (súgó: --help)" ;;
    esac
done

# Questions go to the terminal even when the script itself arrives through a pipe.
ask() { # ask "kérdés" "alapértelmezés" → stdout
    local q="$1" def="$2" ans=""
    if [ "$YES" = 1 ] || ! { : >/dev/tty; } 2>/dev/null; then echo "$def"; return; fi
    printf '%s [%s]: ' "$q" "$def" >/dev/tty
    IFS= read -r ans </dev/tty || true
    echo "${ans:-$def}"
}
confirm() { # confirm "kérdés" → 0 if yes (default no)
    local a
    a="$(ask "$1 (i/n)" n)"
    case "$a" in i|I|igen|y|Y|yes) return 0 ;; *) return 1 ;; esac
}

[ "$(uname -s)" = Linux ] || die "ez a telepítő Linuxra való (Windowsra: KinoraSetup.exe, egyébként Docker)"
command -v systemctl >/dev/null || die "systemd kell (más init rendszeren használd a Docker image-et)"

# Become root, remembering who started us (that user is the default service user).
if [ "$(id -u)" != 0 ]; then
    command -v sudo >/dev/null || die "root jog kell: futtasd rootként vagy telepítsd a sudo-t"
    say "rendszergazdai jog kell, a sudo jelszót kérhet…"
    if [ -f "${BASH_SOURCE[0]:-}" ]; then exec sudo -E KINORA_INVOKER="$(id -un)" bash "${BASH_SOURCE[0]}" "$@"; fi
    tmp="$(mktemp)"; curl -fsSL "https://raw.githubusercontent.com/$REPO/main/install.sh" -o "$tmp"
    exec sudo -E KINORA_INVOKER="$(id -un)" bash "$tmp" "$@"
fi
INVOKER="${KINORA_INVOKER:-${SUDO_USER:-}}"

# ------------------------------------------------------------------ uninstall
if [ "$UNINSTALL" = 1 ]; then
    say "Kinora eltávolítása"
    systemctl disable --now kinora 2>/dev/null || true
    rm -f "$UNIT" /usr/local/bin/kinora
    systemctl daemon-reload
    rm -rf "$BIN"
    ok "szolgáltatás és program eltávolítva"
    if confirm "Törlöd az adatokat is ($DATA: adatbázis, képek; $CACHE)? Ez nem vonható vissza!"; then
        rm -rf "$DATA" "$CACHE" "$ETC" "$OPT"
        if id kinora >/dev/null 2>&1 && [ "$(getent passwd kinora | cut -d: -f7)" = /usr/sbin/nologin ]; then userdel kinora 2>/dev/null || true; fi
        ok "adatok törölve"
    else
        ok "az adatok megmaradtak: $DATA (újratelepítéskor folytatódik)"
    fi
    exit 0
fi

# ------------------------------------------------------------------ questions
if [ -z "$RUNAS" ]; then
    echo
    echo "Melyik felhasználóként fusson a Kinora?"
    echo "  1) a saját felhasználód (${INVOKER:-?}) – ajánlott otthoni gépen: minden mappát lát, amit te"
    echo "  2) dedikált \"kinora\" rendszerfelhasználó – elkülönítettebb, de a mappákra jogot kell adni"
    if [ -n "$INVOKER" ] && [ "$INVOKER" != root ]; then def=1; else def=2; fi
    [ "$(ask "Választás" "$def")" = 1 ] && RUNAS=me || RUNAS=kinora
fi
if [ "$RUNAS" = me ] && { [ -z "$INVOKER" ] || [ "$INVOKER" = root ]; }; then
    die "--run-as-me: nem tudom, ki indította a telepítőt (rootként futtatva válaszd a --run-as-kinora-t)"
fi
[ -z "$NAME" ] && NAME="$(ask "Szerver neve" "$(hostname -s 2>/dev/null || echo Kinora)")"
[ -z "$PORT" ] && PORT="$(ask "Port" 8096)"
case "$PORT" in ''|*[!0-9]*) die "érvénytelen port: $PORT" ;; esac
if [ -z "$LOGO" ] && [ "$YES" != 1 ]; then LOGO="$(ask "Logófájl útvonala (üresen hagyva az alap Kinora logó)" "")"; fi
if [ -n "$LOGO" ]; then [ -r "$LOGO" ] || die "a logó nem olvasható: $LOGO"; fi
if [ -z "${MEDIA[*]:-}" ] && [ "$YES" != 1 ]; then
    m="$(ask "Kezdő médiamappák, vesszővel elválasztva (később az adminfelületen is hozzáadhatók)" "")"
    IFS=',' read -r -a MEDIA <<<"$m"
fi

# ------------------------------------------------------------------ architecture & packages
case "$(uname -m)" in
    x86_64|amd64) ARCH=amd64 ;;
    aarch64|arm64) ARCH=arm64 ;;
    *) die "nem támogatott architektúra: $(uname -m) (amd64 és arm64 van; más esetben Docker)" ;;
esac

install_pkgs() {
    if command -v apt-get >/dev/null; then
        DEBIAN_FRONTEND=noninteractive apt-get update -qq
        DEBIAN_FRONTEND=noninteractive apt-get install -y -qq "$@"
    elif command -v dnf >/dev/null; then dnf install -y -q "$@"
    elif command -v pacman >/dev/null; then pacman -Sy --noconfirm --needed "$@"
    elif command -v zypper >/dev/null; then zypper -n install "$@"
    else return 1; fi
}
need=()
command -v ffmpeg >/dev/null || need+=(ffmpeg)
command -v curl >/dev/null || need+=(curl)
command -v setfacl >/dev/null || need+=(acl)
command -v openssl >/dev/null || need+=(openssl)
command -v fpcalc >/dev/null || { if command -v apt-get >/dev/null; then need+=(libchromaprint-tools); elif command -v pacman >/dev/null; then need+=(chromaprint); else need+=(chromaprint-tools); fi; }
if [ ${#need[@]} -gt 0 ]; then
    say "csomagok telepítése: ${need[*]}"
    if ! install_pkgs "${need[@]}"; then
        command -v ffmpeg >/dev/null || die "az ffmpeg nem telepíthető automatikusan – telepítsd kézzel, majd futtasd újra"
    fi
fi
command -v ffmpeg >/dev/null && command -v ffprobe >/dev/null || die "ffmpeg/ffprobe nem található"
ok "ffmpeg: $(ffmpeg -version | head -1 | cut -d' ' -f3)"

# ------------------------------------------------------------------ download & verify
api="https://api.github.com/repos/$REPO/releases"
if [ -n "$VERSION" ]; then rel="$api/tags/$VERSION"; else rel="$api/latest"; fi
say "kiadás lekérdezése: $rel"
json="$(curl -fsSL -H 'Accept: application/vnd.github+json' "$rel")" || die "a kiadás nem érhető el (van már kiadás a $REPO tárolóban?)"
TAG="$(printf '%s' "$json" | grep -m1 '"tag_name"' | sed -E 's/.*"tag_name": *"([^"]+)".*/\1/')"
[ -n "$TAG" ] || die "nem találom a kiadás címkéjét"
asset="kinora-linux-$ARCH"
base="https://github.com/$REPO/releases/download/$TAG"
work="$(mktemp -d)"; trap 'rm -rf "$work"' EXIT
say "letöltés: $TAG ($asset)"
curl -fsSL "$base/$asset" -o "$work/$asset" || die "letöltés sikertelen: $asset"
curl -fsSL "$base/SHA256SUMS" -o "$work/SHA256SUMS" || die "letöltés sikertelen: SHA256SUMS"
curl -fsSL "$base/SHA256SUMS.sig" -o "$work/SHA256SUMS.sig" || die "letöltés sikertelen: SHA256SUMS.sig"

# signature over SHA256SUMS (ed25519; the raw public key is wrapped into an SPKI PEM for openssl)
if [ "$PUBKEY_B64" = "__KINORA_SIGNING_PUBKEY__" ]; then
    die "ebben a telepítőben nincs aláíró kulcs – a kiadáshoz csatolt install.sh-t használd"
fi
{ printf '\x30\x2a\x30\x05\x06\x03\x2b\x65\x70\x03\x21\x00'; printf '%s' "$PUBKEY_B64" | base64 -d; } > "$work/pub.der"
openssl pkey -pubin -inform DER -in "$work/pub.der" -out "$work/pub.pem" 2>/dev/null || die "a nyilvános kulcs hibás"
base64 -d "$work/SHA256SUMS.sig" > "$work/sig.bin" 2>/dev/null || die "az aláírásfájl hibás"
openssl pkeyutl -verify -pubin -inkey "$work/pub.pem" -rawin -in "$work/SHA256SUMS" -sigfile "$work/sig.bin" >/dev/null 2>&1 \
    || die "AZ ALÁÍRÁS ÉRVÉNYTELEN – a letöltött kiadás nem a Kinora kiadója által aláírt. Nem telepítem."
ok "aláírás rendben (ed25519)"
want="$(grep " $asset\$" "$work/SHA256SUMS" | cut -d' ' -f1)"
got="$(sha256sum "$work/$asset" | cut -d' ' -f1)"
[ -n "$want" ] && [ "$want" = "$got" ] || die "SHA-256 eltérés: $asset"
ok "SHA-256 rendben"

# ------------------------------------------------------------------ user & files
if [ "$RUNAS" = me ]; then
    SVC_USER="$INVOKER"
else
    SVC_USER=kinora
    id kinora >/dev/null 2>&1 || useradd --system --home-dir "$DATA" --shell /usr/sbin/nologin kinora
fi
SVC_GROUP="$(id -gn "$SVC_USER")"
suppl=""
for g in video render; do getent group "$g" >/dev/null && suppl="$suppl $g"; done   # hardware transcoding
[ -n "$suppl" ] && suppl="SupplementaryGroups=${suppl# }"

mkdir -p "$BIN" "$DATA" "$CACHE" "$ETC"
[ -x "$BIN/kinora" ] && cp -p "$BIN/kinora" "$BIN/kinora.prev"
install -m 0755 "$work/$asset" "$BIN/kinora.new" && mv -f "$BIN/kinora.new" "$BIN/kinora"
ln -sf "$BIN/kinora" /usr/local/bin/kinora
cat > "$BIN/kinora-guard" <<'GUARD'
#!/bin/sh
# Runs before every start (ExecStartPre). After a self-update the new binary must confirm
# that it is healthy; if it crashed twice or did not confirm within 90 s, roll back.
marker=/var/lib/kinora/update/pending
[ -f "$marker" ] || exit 0
bin=/opt/kinora/bin
attempts=$(sed -n 's/^attempts=//p' "$marker"); attempts=${attempts:-0}
started=$(sed -n 's/^started=//p' "$marker"); started=${started:-0}
now=$(date +%s)
if [ "$attempts" -ge 2 ] || [ $((now - started)) -gt 90 ]; then
    if [ -x "$bin/kinora.prev" ]; then
        mv -f "$bin/kinora.prev" "$bin/kinora"
        echo "result=rolled_back" > /var/lib/kinora/update/last
        echo "kinora-guard: the update did not become healthy – previous version restored" >&2
    fi
    rm -f "$marker"
    exit 0
fi
sed -i "s/^attempts=.*/attempts=$((attempts + 1))/" "$marker" 2>/dev/null || echo "attempts=$((attempts + 1))" >> "$marker"
GUARD
chmod 0755 "$BIN/kinora-guard"

# the service owns its data, cache and binary folder (self-update replaces the binary there)
chown -R "$SVC_USER:$SVC_GROUP" "$DATA" "$CACHE" "$BIN"

media_env=""
for m in ${MEDIA[@]+"${MEDIA[@]}"}; do
    m="$(echo "$m" | sed 's/^ *//;s/ *$//')"; [ -n "$m" ] || continue
    media_env="${media_env:+$media_env;}$m"
done
logo_env=""
if [ -n "$LOGO" ]; then
    ext="${LOGO##*.}"; install -m 0644 -o "$SVC_USER" "$LOGO" "$DATA/installer-logo.$ext"
    logo_env="$DATA/installer-logo.$ext"
fi

cat > "$ETC/kinora.env" <<ENV
# Kinora settings (systemd EnvironmentFile). After editing: sudo systemctl restart kinora
KINORA_INSTALL=systemd
KINORA_ADDR=:$PORT
KINORA_DATA=$DATA
KINORA_CACHE=$CACHE
KINORA_MEDIA_ROOT=/
# First start only (afterwards the admin interface is the place to change them):
KINORA_INITIAL_SERVER_NAME=$NAME
KINORA_INITIAL_LOGO=$logo_env
KINORA_INITIAL_MEDIA=$media_env
ENV
chmod 0644 "$ETC/kinora.env"

cat > "$UNIT" <<UNIT
[Unit]
Description=Kinora médiaszerver
Documentation=https://github.com/$REPO
After=network-online.target remote-fs.target
Wants=network-online.target

[Service]
Type=simple
User=$SVC_USER
Group=$SVC_GROUP
$suppl
EnvironmentFile=$ETC/kinora.env
ExecStartPre=$BIN/kinora-guard
ExecStart=$BIN/kinora
Restart=always
RestartSec=3
TimeoutStopSec=20
# hardening: everything read-only except data, cache and the binary folder
NoNewPrivileges=true
ProtectSystem=strict
ProtectHome=read-only
ReadWritePaths=$DATA $CACHE $BIN
PrivateTmp=true
ProtectKernelTunables=true
ProtectKernelModules=true
ProtectControlGroups=true
RestrictSUIDSGID=true
LockPersonality=true
Nice=0

[Install]
WantedBy=multi-user.target
UNIT

systemctl daemon-reload
systemctl enable kinora >/dev/null 2>&1
systemctl restart kinora
say "indulás…"
for _ in $(seq 1 30); do
    if curl -fsS "http://127.0.0.1:$PORT/api/status" >/dev/null 2>&1; then break; fi
    sleep 1
done
curl -fsS "http://127.0.0.1:$PORT/api/status" >/dev/null 2>&1 || { journalctl -u kinora -n 30 --no-pager; die "a Kinora nem indult el (napló fent)"; }
ok "Kinora $TAG fut ($SVC_USER felhasználóként)"

ip="$(hostname -I 2>/dev/null | awk '{print $1}')"
echo
echo "  Nyisd meg: http://${ip:-localhost}:$PORT  – az első indításkor létrehozod az admin fiókot."
echo "  Napló:     journalctl -u kinora -f"
echo "  Beállítás: $ETC/kinora.env   Adatok: $DATA"
echo "  Eltávolítás: sudo bash install.sh --uninstall"
if [ "$RUNAS" = kinora ]; then
    echo
    echo "  A dedikált \"kinora\" felhasználónak olvasási jog kell a médiamappákhoz. Az admin felületen a"
    # shellcheck disable=SC1111
    echo "  Könyvtárak oldalon a „Jogosultság ellenőrzése” gomb megadja a pontos parancsot, pl.:"
    echo "    sudo setfacl -R -m u:kinora:rX /home/$INVOKER/Filmek && sudo setfacl -m u:kinora:x /home/$INVOKER"
fi
