# Biztonság / Security

**Magyar** · [English](#english)

## Sérülékenység bejelentése

Biztonsági hibát **ne nyilvános issue-ban** jelents. Használd a GitHub privát bejelentését:
[Security → Report a vulnerability](https://github.com/ScotyLucas/Kinora/security/advisories/new). A bejelentést csak a
karbantartók látják.

Írd le benne:

- az érintett verziót (Admin → Vezérlőpult, vagy `kinora version`) és a telepítés módját (Docker, Linux, Windows, app);
- mit lehet elérni a hibával, és milyen feltételek kellenek hozzá (például bejelentkezett felhasználó, gyerekprofil,
  a helyi hálózat);
- a lépéseket a reprodukáláshoz, és ha van, egy kis példát.

Visszaigazoljuk a bejelentést, a javítás menetéről a bejelentésben tájékoztatunk, és a javított kiadás jegyzetében –
ha kéred – megnevezünk. Kérjük, addig ne hozd nyilvánosságra a hibát, amíg a javítás ki nem jön.

## Támogatott verziók

Biztonsági javítás a **legújabb kiadásba** kerül. Régebbi verzióra frissítéssel lehet áttérni (a Docker-image
`latest` címkéje és a beépített frissítés-ellenőrzés a legújabbat mutatja).

## Ami a szoftver működéséből adódik

- Az első admin fiókot az hozza létre, aki elsőként megnyitja a szervert: a kezdő beállítást azelőtt végezd el, hogy a
  szervert kívülről elérhetővé tennéd.
- A Jellyfin-kompatibilis API alapból ki van kapcsolva; a Jellyfin- és *arr-import csak adminnak érhető el.
- A belső biztonsági áttekintés és az ismert, még nyitott tételek: [docs/AUDIT.md](docs/AUDIT.md).

---

## English

### Reporting a vulnerability

Please **do not** report security issues in a public issue. Use GitHub's private reporting:
[Security → Report a vulnerability](https://github.com/ScotyLucas/Kinora/security/advisories/new); only the maintainers
see it.

Include the affected version (Admin → Dashboard, or `kinora version`) and how Kinora is installed; what the issue allows
and what it needs (a signed-in user, a kids profile, the local network…); and steps to reproduce, with a small example
if you have one.

We acknowledge the report, keep you informed about the fix in the report, and – if you wish – credit you in the release
notes of the fixed version. Please keep the issue private until the fix is released.

### Supported versions

Security fixes go into the **latest release**; older versions are fixed by updating.

### By design

Whoever opens a new server first creates the first admin account, so finish the setup before exposing the server.
The Jellyfin-compatible API is off by default, and imports are admin-only. The internal security review and the known
open items: [docs/AUDIT.md](docs/AUDIT.md).
