## Mit változtat? / What does it change?

<!-- Röviden: mit és miért. Ha egy issue-t zár le: "Closes #123". / Briefly: what and why. If it closes an issue: "Closes #123". -->

## Hogyan tesztelted? / How was it tested?

- [ ] `go vet ./...` és `go test -race ./...` / and
- [ ] `scripts/smoke.sh` (ha a szerver változott / if the server changed)
- [ ] `cd apps && npm test` (ha az appok változtak / if the apps changed)
- [ ] kézzel / by hand: <!-- melyik kliensen, milyen lépésekkel / which client, which steps -->

## Ellenőrzőlista / Checklist

- [ ] Új funkcióhoz van teszt. / New features come with tests.
- [ ] Új végpont a meglévő hitelesítésen és jogosultságokon megy át (gyerekprofil, könyvtár-láthatóság). / New endpoints go through the existing auth and permission checks (kids profiles, library visibility).
- [ ] Az új szövegek magyarul, angolul és németül is megvannak. / New texts exist in Hungarian, English and German.
- [ ] A dokumentáció frissítve, ha kell. / Documentation updated where needed.
- [ ] Nincs új függőség, vagy indokoltam. / No new dependency, or it is justified above.
- [ ] Ha AI-eszközt használtam, jeleztem, és értem a kódot. / If I used an AI tool, I said so and I understand the code.
