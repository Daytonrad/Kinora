# Push-közvetítő (iOS)

A hivatalos iOS app APNs-értesítéseit egy közvetítő (relay) juttatja el az Apple-hez. A Kinora szerverek a felhasználók
gépén futnak, az APNs-kulcs viszont a hivatalos app fejlesztői fiókjához tartozik, ezért azt nem kaphatja meg egyetlen
szerver sem. A közvetítő kódja külön repóban van (`gitlab.com/Zsozsi/kinorarelay`, Cloudflare Worker), ez a dokumentum a szerver, az app és
a közvetítő közötti szerződés.

## Alapelvek

- **Szerver nem lát APNs-tokent.** Az app a tokent közvetlenül a közvetítőnek adja, és cserébe egy **handle**-t kap
  (véletlen, 32 bájtos azonosító). A Kinora szerver csak a handle-t ismeri.
- **Handle csak egy szerverhez érvényes.** A handle egy (eszközkulcs, szerver) párhoz tartozik; más szerver nem küldhet rá.
- **Csak a hivatalos app regisztrálhat.** Az eszközregisztrációt App Attest igazolja (`TEAMID.bundleid`). Fork, szkript
  vagy szimulátor nem kap handle-t (kivétel: sandbox fejlesztői kapcsoló, lent).
- **Kötött formájú értesítés.** A szerver nem adhat meg `aps` mezőt. A közvetítő mindig ugyanazt a tartalék szöveget
  (lokalizációs kulcsot) küldi `mutable-content: 1`-gyel, mellette a titkosított csomagot. Hang, kritikus értesítés,
  csendes push nincs.
- **A közvetítő nem látja a tartalmat.** A csomagot a szerver az eszközzel közös AES-256-GCM kulccsal titkosítja; az app
  Notification Service Extensionje fejti vissza.
- **Korlátok és vészfék.** Handle-enként és szerverenként óránkénti keret; szerver és eszköz azonnal letiltható.

## Szereplők és titkok

| Ki | Mit tárol | Mit nem lát |
|---|---|---|
| iOS app | App Attest kulcs (Secure Enclave), handle-k szerverenként, szerverenkénti AES-kulcs (Keychain, app group) | – |
| Kinora szerver | `server_id` + `server_secret` (a közvetítőtől), eszközönként handle + AES-kulcs | APNs-token |
| Közvetítő | szerverek (`server_id`, secret hash), eszközök (App Attest kulcs, APNs-token, környezet), handle → (eszköz, szerver) | értesítés tartalma, AES-kulcs |

## Folyamat

1. **Szerverregisztráció** (egyszer, a push bekapcsolásakor): `POST /v1/servers` → `server_id`, `server_secret`.
   A szerver ezt a saját adatbázisában tárolja. Önmagában semmire nem jogosít, handle nélkül nem lehet küldeni.
2. **Eszközregisztráció** (az appban, párosításkor vagy tokenváltozáskor):
   1. `GET /v1/challenge` → egyszer használható `challenge` (5 percig érvényes).
   2. Új kulcsnál `DCAppAttestService.attestKey(keyId, clientDataHash: SHA256(clientData))`, ismert kulcsnál
      `generateAssertion(keyId, clientDataHash:)`.
   3. `POST /v1/devices` → `handle`.
3. **Párosítás a szerverrel:** az app a saját Kinora szerverének elküldi a handle-t és egy frissen generált
   AES-256 kulcsot (`POST /api/push/ios`, a felhasználó saját munkamenetével). A szerver a `server_id`-t az app
   kérésére előre közli (`GET /api/push/ios` → `relay_url`, `server_id`), mert a handle ehhez kötődik.
4. **Küldés:** a szerver eseménynél `POST /v1/send` aláírt kéréssel, a handle-lel és a titkosított csomaggal.
5. **Megszűnt token:** ha az APNs `410`-et vagy `BadDeviceToken`-t ad, a közvetítő törli az eszközt, és a
   `/v1/send` `410 {"error":"gone"}`-nal válaszol; a szerver ekkor törli a handle-t.

## Végpontok

Minden kérés és válasz JSON (`content-type: application/json`), base64 mezők **base64url** kódolásúak, kitöltés nélkül.
Hiba esetén: `{"error": "<kód>"}` megfelelő HTTP-státusszal.

### `POST /v1/servers`

Kérés: `{}` (vagy `{"name": "..."}`, legfeljebb 64 karakter, csak tájékoztató).

Válasz `201`: `{"server_id": "srv_…", "server_secret": "<base64url, 32 bájt>"}`.

A közvetítő a secretet nem tárolja: `HMAC-SHA256(SERVER_SECRET_KEY, "server:" + server_id)`-ből számolja újra. Ha a
`SERVER_SECRET_KEY` cserélődik, minden szervernek újra kell regisztrálnia.

IP-címenként óránként legfeljebb 10 regisztráció.

### `GET /v1/challenge`

Válasz `200`: `{"challenge": "<base64url, 32 bájt>", "expires_in": 300}`.

### `POST /v1/devices`

```json
{
  "key_id": "<App Attest keyId, base64>",
  "server_id": "srv_…",
  "apns_token": "<hex>",
  "environment": "production | sandbox",
  "challenge": "<a kapott challenge>",
  "attestation": "<base64url, csak új kulcsnál>",
  "assertion": "<base64url, ismert kulcsnál>"
}
```

A `clientData` amit az app hash-el (UTF-8, pontosan ebben a formában):

```
kinora-relay-v1\n<challenge>\n<server_id>\n<apns_token>\n<environment>
```

A közvetítő ellenőrzi:

- `attestation`: CBOR `apple-appattest` formátum, a tanúsítványlánc az Apple App Attest Root CA-ig, a nonce
  (`SHA256(authData || clientDataHash)`) egyezik a levéltanúsítvány `1.2.840.113635.100.8.2` kiterjesztésével,
  `rpIdHash == SHA256(APP_ID)`, a `keyId == SHA256(nyilvános kulcs)`, a számláló 0, az `aaguid`
  `appattestdevelop` (sandbox) vagy `appattest` + 8 nulla bájt (production) – ez a `environment` mezővel egyezik.
- `assertion`: a tárolt nyilvános kulccsal az ECDSA-aláírás a `SHA256(authData || clientDataHash)`-en,
  `rpIdHash == SHA256(APP_ID)`, a számláló nagyobb a tároltnál.

Válasz `200`: `{"handle": "<base64url, 32 bájt>"}`. Ugyanarra a (kulcs, szerver) párra mindig ugyanaz a handle;
új APNs-token frissíti a tárolt tokent.

**Fejlesztői kapcsoló:** ha a közvetítőben `ALLOW_UNATTESTED_SANDBOX=true`, akkor `environment: "sandbox"` mellett
igazolás nélkül is regisztrálhat egy eszköz (szimulátor). Élesben kikapcsolva marad.

### `POST /v1/send` (szerver → közvetítő)

Fejlécek:

| Fejléc | Érték |
|---|---|
| `x-kinora-server` | `server_id` |
| `x-kinora-timestamp` | Unix idő másodpercben; ±300 mp-en kívül elutasítva |
| `x-kinora-signature` | hex `HMAC-SHA256(server_secret, timestamp + "\n" + METÓDUS + "\n" + útvonal + "\n" + nyers body)` |

Body:

```json
{
  "handle": "<base64url>",
  "payload": "<base64url: CryptoKit AES.GCM combined = nonce(12) || ciphertext || tag(16)>",
  "collapse_id": "opcionális, legfeljebb 64 karakter"
}
```

A `payload` dekódolva legfeljebb 2048 bájt. Válaszok: `200 {"ok":true}`, `401 unauthorized`, `403 disabled`,
`404 unknown_handle`, `410 gone`, `413 payload_too_large`, `429 rate_limited` (`retry-after` fejléccel),
`502 apns_error`.

A közvetítő az APNs-nek ezt küldi (`apns-push-type: alert`, `apns-priority: 10`, `apns-expiration`: +1 óra):

```json
{
  "aps": {
    "alert": { "title-loc-key": "PUSH_FALLBACK_TITLE", "loc-key": "PUSH_FALLBACK_BODY" },
    "mutable-content": 1
  },
  "k": "<payload>",
  "s": "<server_id>"
}
```

### `DELETE /v1/devices/<handle>` (szerver → közvetítő)

Ugyanazzal az aláírással, mint a `/v1/send` (`DELETE`, a saját útvonalával, üres body). A handle-t és – ha más szerverhez már nem tartozik – az
eszközt törli. Válasz `200 {"ok":true}`.

## A titkosított csomag

A titkosítatlan tartalom JSON, legfeljebb ~1,5 KB:

```json
{ "v": 1, "title": "…", "body": "…", "href": "/item/…", "image": "/api/…", "tag": "…" }
```

Az `image` relatív út a saját szerveren; az NSE a saját munkamenetével tölti le. A szerverenkénti AES-kulcsot az app
generálja, a szerverrel párosításkor osztja meg, és Keychainben tárolja (app group, hogy az NSE is elérje). A
csomagban lévő `s` mező alapján választja ki a kulcsot.

## Korlátok

| Mire | Alapérték | Beállítás |
|---|---|---|
| küldés handle-enként | 30 / óra | `LIMIT_HANDLE_HOURLY` |
| küldés szerverenként | 600 / óra | `LIMIT_SERVER_HOURLY` |
| szerverregisztráció IP-nként | 10 / óra | `LIMIT_REGISTER_HOURLY` |
| eszközregisztráció IP-nként | 60 / óra | `LIMIT_DEVICE_HOURLY` |

## Vészfék

- Szerver letiltása: `POST /v1/admin/servers/<server_id>/disable` (`authorization: Bearer <ADMIN_TOKEN>`).
- Eszköz letiltása: `POST /v1/admin/handles/<handle>/disable`.
- Minden küldés leállítása: `SEND_DISABLED=true` változó és újratelepítés.

## A Kinora szerver oldala (még nincs kész)

- Beállítás: push ki/be, közvetítő URL (alapértelmezés a hivatalos közvetítő), átírható saját közvetítőre.
- Tárolás: `server_id`, `server_secret`, és felhasználónként/eszközönként handle + AES-kulcs.
- Küldés ugyanabból az eseményrendszerből, mint a Web Push; `410`-re a handle törlése.
