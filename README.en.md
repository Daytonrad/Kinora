# Kinora

[Magyar](README.md) · **English**

A self-hosted media server for movies and TV shows — an alternative to Jellyfin and Plex, with a web interface in Hungarian, English and German. The name comes from a hand-held “motion-picture viewer” from the 1890s.

Kinora is a single Go binary with an embedded web client, an SQLite database, and FFmpeg for transcoding. Docker is the recommended installation method.

![Home screen](docs/screenshots/05-home.png)

## Install with Docker (recommended)

```yaml
# docker-compose.yml
services:
  kinora:
    image: ghcr.io/scotylucas/kinora:latest
    container_name: kinora
    ports: ["8096:8096"]
    environment:
      - TZ=Europe/Budapest
      - PUID=1000
      - PGID=1000
    volumes:
      - ./config:/config
      - ./cache:/cache
      - /path/to/movies:/media/movies:ro
      - /path/to/shows:/media/shows:ro
    # Intel/AMD hardware acceleration:
    # devices: ["/dev/dri:/dev/dri"]
    # group_add: ["video", "render"]
    restart: unless-stopped
```

```sh
docker compose up -d
```

Then open `http://<server-ip>:8096`. On first launch, a setup wizard asks for a server name and administrator account. Then go to **Admin → Libraries** and add `/media/movies` and `/media/shows` (the folder picker opens in `/media`).

| Container path | Purpose | Back it up? |
|---|---|---|
| `/config` | Database (`kinora.db`), posters and images | **Yes** |
| `/cache` | Transcoded segments, downloads and subtitle cache | No |
| `/media/...` | Your media, mounted **read-only** (`:ro`) — Kinora never writes here | — |

### Environment variables

All settings can also be provided through environment variables. When set this way, the corresponding field is locked in the admin interface and marked as configured through an environment variable.

| Variable | Default | Description |
|---|---|---|
| `KINORA_ADDR` | `:8096` | Address to listen on |
| `KINORA_DATA` | `/config` (native: `./data`) | Database and images |
| `KINORA_CACHE` | `/cache` (native: `<data>/cache`) | Transcoding and downloads |
| `KINORA_PUBLIC_URL` | — | Public URL for invitation links and QR codes |
| `KINORA_TMDB_KEY` | — | TMDB API key (v3 key or v4 token) |
| `KINORA_TMDB_API` | `https://api.themoviedb.org/3` | TMDB API address (for a proxy or mirror) |
| `KINORA_METADATA_LANG` | `hu-HU` | Default metadata language (clients get titles in their own interface language when it differs) |
| `KINORA_ENCODER` | `auto` | `auto`, `h264_nvenc`, `h264_qsv`, `h264_vaapi`, `libx264`, etc. |
| `KINORA_VIDEO_CODEC` | `auto` | `auto`: HEVC/AV1 output when the client reports it and the hardware encoder works; `h264`: H.264 only |
| `KINORA_VAAPI_DEVICE` | `/dev/dri/renderD128` | VA-API device |
| `KINORA_SCAN_INTERVAL_MIN` | `15` | Periodic scan interval in minutes (`0` = disabled) |
| `KINORA_SERVER_NAME` | `Kinora` | Server name |
| `KINORA_LOG_FORMAT` | text | Set to `json` for JSON logs (`docker logs` displays either format) |
| `KINORA_LOG_LEVEL` | `info` | `debug` |
| `KINORA_FFMPEG`, `KINORA_FFPROBE` | `ffmpeg`, `ffprobe` | Paths to custom FFmpeg binaries |
| `KINORA_QUICKCONNECT` | `1` | Allow six-digit code sign-in from TVs/phones (`0` = disabled) |
| `KINORA_DISCOVERY` | `1` | Discover servers on the local network (`0` = disabled) |
| `KINORA_DISCOVERY_PORT` | `7359` | Discovery UDP port (`0` = do not listen) |
| `KINORA_CORS_ORIGINS` | — | Comma-separated extra app origins allowed for token-authenticated CORS requests |
| `KINORA_ACCENT` | — | Accent color (`#rrggbb`); also used by the apps |
| `KINORA_LOGO` | — | Logo file (for example `/config/logo.png`); adopted at startup when changed (PNG/JPG/WebP/SVG, max. 2 MB) |
| `KINORA_INITIAL_SERVER_NAME`, `KINORA_INITIAL_LOGO`, `KINORA_INITIAL_MEDIA` | — | First launch only: server name, logo and initial media folders (`Name=/path;/path2`); can be changed later in the admin interface. Intended for installers. |
| `KINORA_INTRO_DETECTION`, `KINORA_INTRO_WINDOW_MIN` | `1`, `10` | Enable/disable intro and credits detection, and the number of minutes to search for an intro |
| `TZ`, `PUID`, `PGID` | `UTC`, `1000`, `1000` | Time zone and file owner IDs |

### Which image should I use?

| Tag | Features | Size (compressed / unpacked) | Choose it when |
|---|---|---|---|
| `latest` (also `vX.Y.Z`, `edge`) | Software encoding, **Intel** VA-API/Quick Sync (amd64), **NVIDIA** NVENC | ~165 MB / ~615 MB | Most setups: Intel iGPU NAS or mini PC, NVIDIA GPU, Raspberry Pi, or no GPU. |
| `latest-amd` (also `vX.Y.Z-amd`, `edge-amd`) | Same features, plus **AMD** Radeon VA-API (Mesa radeonsi) | ~225 MB / ~855 MB | AMD GPU or AMD APU (Ryzen iGPU) when you want hardware encoding. |

Both images are available for amd64 and arm64 (multi-architecture manifests). If you are unsure which to use, start with `latest` and check the encoder self-test under **Admin → Transcoding**. If you have an AMD system and VA-API is reported as unavailable, switch to `latest-amd`.

### Hardware acceleration

- **Intel / AMD (VA-API, Quick Sync):** AMD GPUs require the `latest-amd` image. Uncomment `devices: ["/dev/dri:/dev/dri"]`. The entrypoint automatically joins the group that owns `/dev/dri/*` (the render/video GID varies by system), so the `group_add` line can be omitted.
- **NVIDIA:** Install the NVIDIA driver and [NVIDIA Container Toolkit](https://docs.nvidia.com/datacenter/cloud-native/container-toolkit/), then run:
  `docker compose -f docker-compose.yml -f docker-compose.nvidia.yml up -d`
- Under **Admin → Transcoding**, Kinora actually encodes a one-second test clip with each encoder and reports which ones work, how long they take, and any error message. Automatic selection order: configured encoder if it works → first working hardware encoder → software `libx264`. With `KINORA_VIDEO_CODEC=auto`, the same hardware's HEVC (or AV1) encoder is used instead when the client reports the codec and that encoder passed the self-test; HEVC/AV1 go in fMP4 HLS segments.

### NAS tips

- **Unraid:** Docker → Add Container, repository `ghcr.io/scotylucas/kinora:latest`, port 8096, paths `/config` → `/mnt/user/appdata/kinora` and `/media` → `/mnt/user/media` (read-only). Set `PUID=99`, `PGID=100` (`nobody:users`). For an Intel iGPU, add `--device=/dev/dri` under Extra Parameters.
- **TrueNAS SCALE:** Create a Custom App and mount datasets as host paths. Set `PUID`/`PGID` to the dataset owner's IDs (usually `568:568` for `apps`). For a GPU, use **GPU Configuration → Intel/AMD**.
- **Synology (Container Manager):** Find the `PUID`/`PGID` values over SSH with `id <user>`. `/dev/dri` is available only on models with an Intel CPU (including some “+” series models). The scanner automatically skips `@eaDir` and `#recycle` folders.
- If an SMB/NFS share disconnects, Kinora **does not delete anything**: the scan stops safely, missing files are only marked as missing, and their watched status is preserved when the share returns.

### Native Linux installation (systemd)

```sh
curl -fsSL https://raw.githubusercontent.com/ScotyLucas/kinora/main/install.sh | bash
```

The installer downloads and verifies the latest release (SHA-256 and Ed25519 signature), installs FFmpeg, and creates a systemd service that runs as your user or as a dedicated `kinora` user. Kinora can access **any directory the service account can read**, and you can add more directories at any time. If it cannot read a directory, the interface explains why and provides a command to fix the permissions. `.deb` and `.rpm` packages are also available. See [`docs/LINUX.md`](docs/LINUX.md) for details about permissions and SMB/NFS mounts.

**Adding other directories later in Docker:** `docker-compose.host.yml` mounts `/home`, `/mnt`, `/media` and `/srv` read-only under `/host`:

```sh
docker compose -f docker-compose.yml -f docker-compose.host.yml up -d
```

See [`docs/LINUX.md`](docs/LINUX.md) for the security trade-offs.

### Windows

There is no dedicated Windows installer; Docker Desktop is recommended (see above). Advanced users can run a source-built `kinora.exe` as a Windows service (`kinora.exe service install --data C:\ProgramData\Kinora`, as administrator). FFmpeg must be on `PATH` or in an `ffmpeg\` folder next to the executable.

### Custom logo and server name

Upload a logo for dark and light backgrounds under **Admin → Settings → Appearance** (PNG, JPG, WebP or SVG, up to 2 MB). Kinora derives the favicon and app icons from it, and the apps show it on their sign-in and home screens. SVG files are accepted only without scripts or external references, and are served only as images. The first-launch wizard also offers an optional logo upload.

In Docker, set `KINORA_LOGO=/config/logo.png`. The Linux installer can apply a logo on first launch with `install.sh --logo`.

### Build from source

Requirements: Go 1.24+, a C compiler (SQLite requires cgo), and `ffmpeg` and `ffprobe` on `PATH`.

```sh
go build -o kinora ./cmd/kinora
./kinora --data ./data          # http://localhost:8096
```

## Why Kinora?

| Common Jellyfin pain point | How Kinora addresses it |
|---|---|
| Scanning a large library is slow and makes the UI lag | Incremental scanner: an unchanged file needs one `stat()` call; `ffprobe` runs only on new or changed files, with up to six workers. SQLite WAL mode and short transactions keep browsing responsive. A second scan of the test library takes about 200 ms. |
| Watched status is lost after a file is renamed or moved | Move detection matches a missing file with a new file of the same size, preserving its item ID and watched status. |
| A dropped SMB/NFS share makes the library disappear | If a root folder becomes unavailable or suddenly empty, scanning stops without modifying the library. Missing files are marked `missing`, not deleted. |
| Transcoder errors are hard to understand | Real encoder self-tests plus a “Why is it playing this way?” panel with plain-language reasons, selected encoder, source details, browser capabilities and the final FFmpeg error line. |
| No transcoded downloads for offline viewing | Prepare a download in the background at 1080p/720p/480p/360p or in the original quality, with progress reporting. Fast-start MP4 files are kept for seven days. |
| SyncPlay drifts or stutters | Watch parties use a server clock and NTP-like synchronization: small differences are corrected with `playbackRate` 0.95/1.05; larger ones seek. If someone buffers, everyone waits, for up to 12 seconds. Chat and emoji reactions are included. |
| No personal labels, collections or useful filtering | Custom tags, ordered manual collections, rule-based smart collections (18 fields, all/any matching), combined filters and sorting. Item pages show which collections contain each item. |
| Smart lists require a plugin | A built-in rules engine provides a live preview in the editor; collections can also be pinned to the home screen. |
| No “Because you watched…” recommendations | Fully local, explainable recommendations: “Recommended for you” (for example, “because you like: Sci-Fi, Drama”), “Because you watched: X”, “Hidden gems”, and a “Similar” row on item pages. |
| A series can only be removed from Next Up by marking it watched | Hide a series without marking it watched. It returns automatically when you start watching it again. |
| No real profiles or kids mode | Up to eight profiles per account, optional PINs and child profiles with age limits. Restrictions apply **at every endpoint**: lists, details, search, recommendations, streams, subtitles, downloads, images and watch parties. |
| Adding users is cumbersome | Invitation links with use limits and expiration dates, plus QR codes. |
| Remote access is hard to configure | A **Connect** page lists LAN and Tailscale addresses as QR codes and includes guides for Tailscale, Cloudflare Tunnel and reverse proxies. |
| Subtitle menus contain too many irrelevant languages | Set subtitle and audio language priorities per profile. The menu shows preferred languages first, with an option to show all languages. |
| No next-episode prompt | Detects credits from chapter names (credits, end titles, etc.); if unavailable, prompts during the final 30 seconds. The next episode starts after a countdown. |
| Accented Hungarian characters break in `.srt` subtitles | Non-UTF-8 external subtitles are read as CP1250, so text such as “Árvíztűrő tükörfúrógép” displays correctly. |
| Backups are unreliable | Create a consistent database snapshot with one click (`VACUUM INTO`), even while the server is running. |

## Media folder structure

```text
Movies/
  Blade Runner 2049 (2017)/Blade Runner 2049 (2017).mkv     <- folder name takes precedence
  Blade Runner 2049 (2017)/movie.nfo, poster.jpg, fanart.jpg
  Blade Runner 2049 (2017)/Blade Runner 2049 (2017).hu.srt  <- <video>.<language>[.forced].srt|vtt|ass
  The.Matrix.1999.1080p.BluRay.x264.mkv                     <- “The Matrix” (1999)
Shows/
  Breaking Bad (2008)/tvshow.nfo, poster.jpg
  Breaking Bad (2008)/Season 01/Breaking.Bad.S01E02.mkv
  Dark/Staffel 2/03 - Ghosts.mkv                            <- season folder + episode number
  Aranyélet/2. évad/Aranyelet.S02E04.mkv
  Futurama/Specials/Futurama.S00E01.mkv                     <- season 0
  Seinfeld.S03E10.The.Old.Man.mkv                           <- flat layouts also work
```

- Recognized patterns: `S01E02`, `S01E01E02` (double episode), `1x05`, and season folder + `03 - Title`. Season folder names include `Season 01`, `S1`, `Staffel 2`, `Évad 2`, `2. évad`, and `Specials`/`Extras` (= season 0).
- Skipped: hidden folders, `@eaDir`, `#recycle`, `trailers`, `featurettes`, `behind the scenes`, and `sample`/`-trailer` files under 300 MB.
- Local metadata: Kodi/Jellyfin-compatible NFO files (`movie.nfo`, `<video>.nfo`, `tvshow.nfo`). Artwork names include `poster`/`folder`/`cover`, `fanart`/`backdrop`/`background`, `<video>-poster`, `<video>-thumb`, and `season01-poster.jpg`. If no artwork exists, Kinora captures a frame at about 20% of the video.
- Optional online metadata: configure a TMDB key under **Admin → Metadata**. Local NFO data and manually corrected items are never overwritten. If a match is wrong, choose another with **Correct match** on the item page.

## Remote access

**Tailscale** (the simplest option; no port forwarding needed): a sidecar container joins your tailnet and runs Kinora on that network.

```sh
TS_AUTHKEY=tskey-... docker compose -f docker-compose.yml -f docker-compose.tailscale.yml up -d
```

You can then reach the server on your tailnet at `http://kinora:8096` (MagicDNS), while its local-network address continues to work as before.

**Cloudflare Tunnel** (custom domain and HTTPS): create a tunnel in the Zero Trust dashboard, point its public hostname to `http://kinora:8096`, then run:

```sh
CF_TUNNEL_TOKEN=... KINORA_PUBLIC_URL=https://kinora.example.com \
  docker compose -f docker-compose.yml -f docker-compose.cloudflared.yml up -d
```

Note: Cloudflare's terms may restrict large volumes of video traffic through a tunnel.

**Reverse proxy:** Watch parties use WebSockets, so forward the `Upgrade` and `Connection` headers. With Caddy, this is sufficient:
`reverse_proxy 127.0.0.1:8096`. The **Admin → Connect** page includes an Nginx example. Kinora honors the `X-Forwarded-Proto` header, so session cookies receive the `Secure` flag behind HTTPS.

## Companion apps and phone installation

- **Android (phone + Android TV), Samsung Tizen, LG webOS and iOS:** client projects are in `apps/`. See [`apps/README.md`](apps/README.md) for build and installation instructions, and [`docs/TESZTELES_MA.md`](docs/TESZTELES_MA.md) for a Windows-based, step-by-step test guide.
- **iPhone without a Mac:** install the web client as a web app (PWA) from Safari using **Share → Add to Home Screen**. It runs full-screen with its own icon.
- **Quick Connect:** a TV displays a six-digit code. Approve it on a signed-in device under the profile menu → **Connect device**, then choose a profile. The code is valid for five minutes and can be used once; attempts are rate-limited per IP and user.
- **Devices:** the Account page lists signed-in devices (name, platform, app version, IP and last activity) and lets you sign them out individually or all at once. Admins can see all devices under **Admin → Devices**.
- **Server discovery:** apps use UDP broadcast on port 7359 or scan the local subnet. Docker bridge networking does not pass broadcasts; use `network_mode: host` or enter the address manually.

Apps send the token returned by `POST /api/login` (or Quick Connect) in the `Authorization: Bearer <token>` header. CORS is allowed only for app origins (`capacitor://localhost`, `http(s)://localhost`, `null`) and token-authenticated requests, without `Access-Control-Allow-Credentials`.

Because `<video>` and `<img>` elements cannot send authorization headers, playback responses include a short-lived, signed stream token (`?st=`) in their URLs. It is scoped to the item, user and device.

## Built-in acquisition (instead of Radarr/Prowlarr/Jellyseerr)

**Admin → Acquisition:** look up a film, pick a release (or let Kinora pick by the quality profile), qBittorrent downloads it, and Kinora hardlinks it into the movie library (`Title (Year) {imdb-tt…}/Title (Year) - 1080p BluRay.mkv`) and scans it. Off by default, with no tracker or indexer included: the admin adds the Torznab sources (a tracker's own API, Jackett or Prowlarr).
Kinora and qBittorrent should see the same `/data` folder (with `downloads/` and `media/` in it) so hardlinks work and the torrent keeps seeding. If qBittorrent sees the downloads under another path, set a path mapping in the admin. Full plan and milestones (Hungarian): [docs/BESZERZES.md](docs/BESZERZES.md).

## Backup and restore

- Download a consistent database snapshot at **Admin → Settings → Download database backup**. It uses `VACUUM INTO` and is safe while the server is running.
- To restore: stop the container, copy the backup into place as `config/kinora.db`, remove `kinora.db-wal` and `kinora.db-shm`, then restart. If images in `config/images` are missing, the next scan regenerates them.

## Why this FFmpeg build?

The image uses Debian Trixie's own **FFmpeg** package (7.1, with `libzimg`/zscale tone mapping, VA-API, QSV (libmfx) and NVENC (ffnvcodec)). It also includes `intel-media-va-driver-non-free` (amd64, Intel iGPU Gen8+) and, in the `-amd` image, `mesa-va-drivers` for AMD. Why not `jellyfin-ffmpeg`?

- Debian provides the same package for amd64 and arm64, making multi-architecture builds simple and reproducible.
- Kinora uses H.264 output and the standard `zscale` + `tonemap` chain, so it does not need jellyfin-ffmpeg's custom OpenCL/Vulkan tone mapping.
- Kinora's encoder self-test shows what actually works on the host.

If you need jellyfin-ffmpeg (for example, for newer Intel Arc GPUs), install it in a custom image and set `KINORA_FFMPEG=/usr/lib/jellyfin-ffmpeg/ffmpeg` and `KINORA_FFPROBE=/usr/lib/jellyfin-ffmpeg/ffprobe`.

**Image size.** Debian's FFmpeg pulls in Mesa OpenGL drivers and LLVM through the `libavdevice → libgl1 → … → mesa-libgallium → libllvm19` dependency chain (about 140 MB). The FFmpeg command-line tool does not create an OpenGL context, so the standard (`latest`) image removes `libgl1-mesa-dri`, `mesa-libgallium`, `libllvm*` and `libz3-4` with `dpkg --force-depends`. A test encode runs immediately afterwards to verify FFmpeg still works. The trade-off is that `apt` reports missing dependencies inside this image; the container is not intended for package installation. These libraries remain in `latest-amd` because AMD's radeonsi VA-API driver needs LLVM.

The Kinora binary itself is about 10 MB.

Build your own image with `docker build --target runtime .` (standard) or `docker build --target runtime --build-arg AMD=1 .` (AMD).

## Development

```sh
go run ./cmd/kinora --data ./data --web ./web/static      # serve the web client from source; changes appear immediately
docker compose -f docker-compose.yml -f docker-compose.dev.yml up --build
```

If Go or a C compiler is not installed locally, `scripts/go.sh` runs the toolchain in a container:

```sh
scripts/mkmedia.sh testmedia          # generate a test library (requires ffmpeg)
scripts/go.sh go vet ./...
scripts/go.sh go test ./...           # unit and integration tests with real media files
scripts/smoke.sh                      # image build + container + curl walkthrough (setup → scan → playback → HLS)
BASE=http://127.0.0.1:18097 node scripts/e2e.mjs   # browser E2E (Playwright + Edge/Chrome), with screenshots
```

Project layout:

```text
cmd/kinora/        entry point, flags/env, graceful shutdown and health-check subcommand
internal/store/    SQLite schema and migrations, auth/profiles/invitations, items, rules engine, collections, recommendations
internal/scanner/  filename parsing, incremental scanner, NFO and artwork
internal/media/    ffprobe, playback decisions, encoder self-test, HLS sessions, WebVTT and downloads
internal/meta/     optional TMDB metadata
internal/party/    Watch party hub (WebSocket)
internal/server/   HTTP API, middleware and web client
web/static/        vanilla JS SPA (index.html, app.js, style.css, vendor/hls.js, vendor/qrcode-generator)
```

The web client is multilingual (Hungarian, English, German). Strings live in `web/static/i18n/<lang>.js`; Hungarian is the base, and missing keys fall back to it. The language follows the browser setting and can be switched in the profile menu, on the account page or on the sign-in screen (the choice is kept in the browser). The server’s Hungarian messages (errors, home rows, playback reasons, watch-party messages) are translated in the client through the `srv`/`srvRe` tables of the language files. For keys missing in English, the other non-Hungarian languages fall back to English first.

**Languages in the admin (Admin → Languages):**
- **Export:** download any language as a JSON file with every key. Untranslated keys contain the fallback language's text.
- **Import:** upload a translated file. If the language already exists, you're asked to confirm before it's overwritten.
- **New language:** enter a code (e.g. `fr`, `pt-br`), a name and a locale (e.g. `fr-FR`; it sets number formats and plural rules). Start from scratch or from a copy of an existing language.
- **Editor:** shows the reference text and the translation for each key. It has a search box and an "untranslated only" filter; a field left empty uses the fallback text.
- **Reset:** changes to the built-in languages (hu, en, de) are stored as overrides, which can be reset with one button. Added languages can be deleted.

Saved languages are stored in `<data>/i18n/<code>.json`, and the client loads them from `/i18n/custom.js`. The file format is `{"kinora_language": 1, "code": "fr", "name": "Français", "strings": {…}}`.
- In templates, `{0}`, `{1}`… are the arguments, and `{0:00}` pads a number to two digits.
- A plural is an object with `one`/`few`/`many`/`other` keys (plus `=0`, `=1`…).
- The format is described in full at the top of the built-in `web/static/i18n/*.js` files.

**The iPhone/iPad and Apple TV apps** use the same languages. The apps' own texts live in the `app` table of the language files; the key is the app's Hungarian text, as with `srv`, so a new language added in the admin translates the apps too, and the editor shows these rows as well. The server serves them at `GET /api/languages` (a list with versions) and `GET /api/languages/<code>`, without sign-in, with the overrides and the English fallback applied, and only the `locale`, `app`, `srv`, `srvRe` parts. The app syncs whenever it connects to a server: it downloads new or changed languages and keeps all of them on the device, even when the server can't be reached or the language was later deleted there. The language is picked in the app under Settings → Language (on Apple TV, the Settings tab); “Automatic” follows the device language and falls back to English, then Hungarian. Widgets and Top Shelf use the same language. What stays Hungarian: the fixed Siri/Shortcuts texts and the system permission prompts.

To contribute a translation to the project, turn the exported `strings` object into a new `web/static/i18n/<code>.js` file and add it to `index.html`.

### Planned for a later version

Built-in relay/NAT traversal, offline downloads in the apps, Watch parties in the apps, audio-fingerprint intro detection, trick-play preview images, music libraries, live TV, importing missing collection items from TMDB, PostgreSQL and translations of the native apps.

## Design

The interface is inspired by a cinema facade lit at night: posters sit in illuminated display cases, and the single accent color (amber) is reserved for what is currently active — the focused card, playback progress and primary action. Two interface modes share the same design tokens: the **lobby** (content-led browsing) and the **projection booth** (precise, quiet admin and editing screens).

- Every value is a design token in the `:root` block at the start of `style.css`: colors, a 4/8 spacing scale, type scale, two corner radii, a 44 px minimum touch target and motion timing.
- Fonts are self-hosted from `web/static/fonts` (SIL OFL): **Big Shoulders Display** for headings and numbers, and **Atkinson Hyperlegible** for text. Both include Hungarian characters.
- Remote and keyboard navigation: arrow keys move focus spatially through rows and grids; the focused poster frame lights up and updates the home hero. Enter opens an item; Esc goes back.
- When artwork is missing, the library uses a typographic title card (title + year) instead of an empty box.
- Screenshots of the previous interface are in `docs/screenshots/before/`.

## Screenshots

| | |
|---|---|
| ![Remote-control focus](docs/screenshots/05b-home-focus.png) | ![Library](docs/screenshots/06-library.png) |
| ![Item details](docs/screenshots/07-detail.png) | ![Why is it transcoding?](docs/screenshots/10-player-transcode-why.png) |
| ![Watch party](docs/screenshots/19-party-host.png) | ![Smart collection](docs/screenshots/14-smart-editor.png) |
| ![Encoder self-test](docs/screenshots/16-admin-transcode.png) | ![Phone](docs/screenshots/21-phone-home.png) |

## License

MIT. The bundled `hls.js` (Apache-2.0) and `qrcode-generator` (MIT) retain their own licenses in `web/static/vendor/`.
