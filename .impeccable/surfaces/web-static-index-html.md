---
version: 1
slug: "web-static-index-html"
primary_target: "web/static/index.html"
related_targets: ["web/static/app.js","web/static/style.css"]
---

# Kinora web client – surface brief

Scope: the whole SPA (web/static/index.html + app.js + style.css). Visitor mode: Experience for browsing surfaces (home, library, detail, player); Operate for admin, editors, forms, settings. Two registers on one token set: "előcsarnok" (foyer: imagery leads, UI recedes) and "vetítőfülke" (projection booth: quiet, dense, rules not boxes).

Audience: household members in the living room on a TV browser with a remote (3 m), the same people on a phone one-handed, laptop users browsing/editing, invited non-technical relatives joining parties. Job: find what to watch and start it in a few steps; admin keeps the server running.

Constraints (binding): functions, API and copy unchanged; all strings stay in app.js `T`; self-hosted fonts only; no CDN; E2E (scripts/e2e.mjs) stays green; hero composition of the home page stays (big title, reason line, two actions); the owner's brief: evening cinema, warm not neon, dark but not black, no blue-purple, one amber accent, no card-in-card, no gradient buttons, no glow, no emoji icons, no needless shadows, design tokens everywhere, big focus ring, 44 px targets.

## Direction contract

THESIS: The home page is the lit cinema facade at night: posters sit in illuminated poster cases, the amber is the marquee bulb, and the only thing that glows is what is live now (the focused case, the playing title, progress, the one primary action). It refuses the category default of a grey-black card grid under a hero with a red button, and refuses the "dark admin kit" of outlined rounded boxes.

OWN-WORLD: Ground is lacquered foyer black with a warm undertone (#141210 → #1f1a16 → #2a2420 steps), never blue. Text is warm off-white (#efe7da) and a muted plaster (#9d9387). One accent, marquee amber (#f0b44c), reserved for primary action, progress, focus, "now". Rules (1 px, #332c26) divide, nothing is boxed; depth only by overlap (text on its own solid plate over imagery), no shadows, no glow, no gradients except the single legibility scrim on backdrops. Display face: Big Shoulders Display (variable, condensed signage grotesque) for titles, row headings, codes and numbers; text face: Atkinson Hyperlegible for everything read at 3 m. Radius: 2 steps (2 px for chips/inputs, 6 px for poster cases). Spacing 4/8 scale; type scale 13/15/17/20/26/34/48/72+. Motion: 160 ms out, 240 ms in, one reveal per page.

STORY: The viewer arrives at a lit facade: the film of the evening fills the frame, its title reads from across the room, one amber button says play. They move along the poster cases with a remote; the case under the beam lights up and the facade shows that film. They press play; the controls fall away; the next episode is announced on a ticket at the credits. The admin steps behind the facade into a quiet booth: rules, tables, tabular figures, no decoration.

FIRST VIEWPORT: Full-bleed backdrop at 70 % viewport height, dimmed by one left-to-right scrim. Top-left: wordmark in display face, nav as plain words on the facade (no bar background), search a hairline field, profile initial as a small square. Lower-left of the hero: reason line (amber, small), title at 72–104 px display face, meta line (year · rating · runtime · genres), 3-line synopsis in text face, then Play (amber filled) + Details (hairline). Immediately below, the first poster row: heading on a hairline rule, 2:3 cases with a 2 px case frame; the focused case lifts 4 % and gains a 3 px amber frame with a halo-free offset ring. Primary action: Play, lower-left, always inside the first viewport.

FORM: Marquee & poster case, candidate 1 of my ordered grounded list (the roll assigned candidate 4, "Esti moziműsor"; the user chose the IMPECCABLE'S PICK card). seed:5bbf7460e258c9e3. Raises carried from the hand: hero title outranks all (illuminated initial), rules not containers (design annual), D-pad first-class (phosphor terminal), accent scarcity (ANSI BBS), depth by overlap only (papercut). Signature interaction: the beam follows focus – focusing a poster case with arrows lights the case and swaps the facade's backdrop and title to it; keyboard model: roving tabindex, arrows within rows/grids, Enter opens, Back/Escape returns.

FINISH: unreviewed and undocumented is unfinished; this build ends with the finish review, the verdict, DESIGN.md, and every shipping raster carrying its provenance

## Unresolved
- Whether the facade backdrop swap on focus is too busy on a phone (touch has no focus); on touch the hero stays static.
- Avatar colours: keep a muted 8-colour set from the foyer palette instead of saturated random colours.
