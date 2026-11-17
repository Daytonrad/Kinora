---
name: Kinora
description: Self-hosted film and series server; the web client is a lit cinema facade at night.
colors:
  foyer-black: "#141210"
  foyer-lacquer: "#1a1613"
  foyer-panel: "#221d19"
  foyer-raised: "#2b2520"
  rule: "#352e28"
  rule-hi: "#4a4139"
  plaster-bright: "#efe7da"
  plaster: "#cfc5b6"
  plaster-muted: "#9d9387"
  marquee-amber: "#f0b44c"
  amber-ink: "#2b1c00"
  amber-dim: "#a67a2e"
  danger: "#e8705f"
  ok: "#8fc27a"
  screen-black: "#000000"
typography:
  display:
    fontFamily: "Big Shoulders Display, Arial Narrow, sans-serif-condensed, sans-serif"
    fontSize: "clamp(48px, 7vw, 104px)"
    fontWeight: 800
    lineHeight: 0.92
    letterSpacing: "0em"
  headline:
    fontFamily: "Big Shoulders Display, Arial Narrow, sans-serif-condensed, sans-serif"
    fontSize: "48px"
    fontWeight: 700
    lineHeight: 0.92
    letterSpacing: "0.005em"
  title:
    fontFamily: "Big Shoulders Display, Arial Narrow, sans-serif-condensed, sans-serif"
    fontSize: "26px"
    fontWeight: 700
    lineHeight: 0.92
    letterSpacing: "0.005em"
  subhead:
    fontFamily: "Big Shoulders Display, Arial Narrow, sans-serif-condensed, sans-serif"
    fontSize: "20px"
    fontWeight: 600
    lineHeight: 0.92
    letterSpacing: "0.03em"
  body:
    fontFamily: "Atkinson Hyperlegible, Segoe UI, Roboto, system-ui, sans-serif"
    fontSize: "15px"
    fontWeight: 400
    lineHeight: 1.5
  body-large:
    fontFamily: "Atkinson Hyperlegible, Segoe UI, Roboto, system-ui, sans-serif"
    fontSize: "17px"
    fontWeight: 400
    lineHeight: 1.6
  numeral:
    fontFamily: "Atkinson Hyperlegible, Segoe UI, Roboto, system-ui, sans-serif"
    fontSize: "17px"
    fontWeight: 700
    lineHeight: 1.5
    fontFeature: "tnum"
  label:
    fontFamily: "Atkinson Hyperlegible, Segoe UI, Roboto, system-ui, sans-serif"
    fontSize: "13px"
    fontWeight: 400
    lineHeight: 1.5
rounded:
  r1: "2px"
  r2: "6px"
spacing:
  s1: "4px"
  s2: "8px"
  s3: "12px"
  s4: "16px"
  s5: "24px"
  s6: "32px"
  s7: "48px"
  s8: "64px"
  s9: "96px"
components:
  button-primary:
    backgroundColor: "{colors.marquee-amber}"
    textColor: "{colors.amber-ink}"
    rounded: "{rounded.r1}"
    padding: "0 16px"
    height: "44px"
  button-primary-hover:
    backgroundColor: "{colors.plaster-bright}"
    textColor: "{colors.amber-ink}"
  button-hairline:
    backgroundColor: "transparent"
    textColor: "{colors.plaster-bright}"
    rounded: "{rounded.r1}"
    padding: "0 16px"
    height: "44px"
  button-ghost:
    backgroundColor: "transparent"
    textColor: "{colors.plaster}"
    rounded: "{rounded.r1}"
    padding: "0 16px"
    height: "44px"
  button-danger:
    backgroundColor: "transparent"
    textColor: "{colors.danger}"
    rounded: "{rounded.r1}"
    height: "44px"
  chip:
    backgroundColor: "transparent"
    textColor: "{colors.plaster}"
    typography: "{typography.body}"
    padding: "0 12px"
    height: "44px"
  chip-on:
    textColor: "{colors.marquee-amber}"
  badge:
    backgroundColor: "transparent"
    textColor: "{colors.plaster}"
    typography: "{typography.label}"
    rounded: "{rounded.r1}"
    padding: "0 8px"
  input:
    backgroundColor: "{colors.foyer-lacquer}"
    textColor: "{colors.plaster-bright}"
    rounded: "{rounded.r1}"
    padding: "8px 12px"
    height: "44px"
  poster-case:
    backgroundColor: "{colors.foyer-panel}"
    rounded: "{rounded.r2}"
    width: "clamp(124px, 13vw, 176px)"
  nav-link:
    textColor: "{colors.plaster-muted}"
    height: "44px"
  nav-link-on:
    textColor: "{colors.plaster-bright}"
  modal:
    backgroundColor: "{colors.foyer-panel}"
    rounded: "{rounded.r2}"
    padding: "24px"
    width: "560px"
---

# Design System: Kinora

## Overview

**Creative North Star: "The Lit Facade at Night"**

The Kinora web client is a cinema front after dark. The ground is lacquered foyer wall with a warm undertone, the posters sit in illuminated cases, and one amber bulb marks whatever is live right now: the case under the beam, the primary action, playback progress, the active place in the navigation. Everything else stays at rest in plaster tones. Imagery leads; the interface steps back until a decision is needed.

One token set carries two registers. The **foyer** (home, library, detail, player) lets backdrops and posters fill the frame, sets titles in a condensed signage face large enough to read from three metres, and moves with a remote. The **booth** (admin, editors, settings, forms) is the projection booth behind the facade: hairline rules instead of panels, tables, tabular figures, no decoration. Both are built from the same colors, scales and rules; the booth is simply the foyer with the lights on the work.

Depth comes only from overlap: text sits on its own solid ground or behind the single legibility scrim over imagery. Nothing glows, nothing is boxed for the sake of boxing, and there are no decorative gradients. Missing artwork is a designed state: a printed programme card, not a grey placeholder.

**Key Characteristics:**
- Warm near-black ground in four steps, never blue, never pure black outside the player screen.
- One accent, marquee amber, reserved for what is live.
- Condensed display face for titles; hyperlegible text face for everything read, including small numerals.
- Hairline rules divide; containers are rare and flat.
- Remote-first: 44 px targets, a 3 px offset amber ring, arrow-key spatial navigation with the facade following focus.
- Token-only styling: literals survive only where a token cannot (breakpoints, gradient angles, rotation, 100vh).

## Colors

A warm, low-key palette of lacquer and plaster lit by a single amber bulb.

### Primary
- **Marquee Amber** (marquee-amber): the bulb. Focus rings and the lit case frame, the primary action, progress bars and the timeline, the active nav underline, selected chips and options, the hero reason line, the wordmark, toast markers. Text on it is **Amber Ink** (amber-ink), a deep brown-black, never white.
- **Dim Amber** (amber-dim): the bulb seen through glass. Secondary "why" lines on cards, counts inside a selected chip, the hot badge border, environment-locked setting notes, kids-profile captions. Never used for action.

### Neutral
- **Foyer Black** (foyer-black): page ground, the html background, the theme colour, the ink on avatar faces.
- **Foyer Lacquer** (foyer-lacquer): field interiors, the party side panel, code blocks.
- **Foyer Panel** (foyer-panel): poster case interiors and programme cards, modals, menus and player pop-overs.
- **Foyer Raised** (foyer-raised): hover fill on menu rows and options, toast ground, empty progress tracks.
- **Rule** (rule): the 1 px divider everywhere: shelf heads, table rows, panel tops, side-info lists, the resting case frame.
- **Rule High** (rule-hi): hairline control borders (buttons, inputs, badges), table header underline, the top rule and foot rule of the programme card, pop-over borders, the scrollbar thumb.
- **Plaster Bright** (plaster-bright): headings, titles, button labels, the hover state of text links.
- **Plaster** (plaster): default body text.
- **Plaster Muted** (plaster-muted): secondary text, labels, placeholders, idle nav, meta separators.
- **Screen Black** (screen-black): only the player's projection surround.

### Semantic
- **Danger** (danger): destructive buttons, error toasts, failed table states, the player error frame.
- **OK** (ok): healthy table states, always paired with a dot and text, never colour alone.

### Avatar set
Profile faces use a fixed muted foyer set of eight (`#c97c5d`, `#b5a27a`, `#8fa37a`, `#7fa6a0`, `#c2b4a3`, `#a3826b`, `#b07a7a`, `#8a9a8f`), with foyer-black initials. The set deliberately contains no amber and no blue-purple, so a profile face never competes with the bulb.

### Named Rules
**The One Bulb Rule.** Amber marks only what is live: focus and the lit case, the primary action, progress, the active nav item, the reason line. A resting poster case carries a hairline Rule frame (programme cards add a Rule High top rule), never amber.

**The Warm Ground Rule.** The ground is the four foyer steps. Pure black appears only behind the video; blue or purple tints appear nowhere.

**The Single Scrim Rule.** The only gradients are the legibility scrims over imagery (hero, detail backdrop, the top bar fade, player chrome), all built from the scrim channel (`20 18 16`) at varying alpha. Buttons, cards and surfaces are flat.

## Typography

**Display Font:** Big Shoulders Display, variable 100–900, self-hosted (with Arial Narrow, condensed sans fallback)
**Body Font:** Atkinson Hyperlegible 400/700, self-hosted (with Segoe UI, Roboto, system-ui)
**Mono:** ui-monospace / Consolas, booth-only for paths and logs

**Character:** A tall, condensed cinema-signage grotesque for anything that is a name or a heading, against a text face designed for low-vision legibility for everything that is read. Both carry full Hungarian accents (ő, ű).

### Hierarchy
- **Display** (800, clamp 48–104 px, line-height 0.92): the facade title on home and detail. One per view.
- **Headline** (700, 48 px, 0.92): page headings, the profile chooser, the auth wordmark.
- **Title** (700, 26 px, 0.92): shelf headings, panel headings, modal titles, the player title, the wordmark in the top bar (800, 0.04em tracking).
- **Subhead** (600, 20 px, 0.03em tracking): the amber reason line, filter group heads, the "below" section heads on detail, the detail subtitle. Booth section heads use it in plaster-muted with a hairline under.
- **Body** (400, 15 px, 1.5): default reading text; long synopsis at 17 px / 1.6, capped at 64ch (hero synopsis 58ch, clamped to three lines).
- **Numeral** (Atkinson 700, tabular figures, 13–26 px): every small or medium number: badges, hero meta, detail facts, chip counts, admin ledger figures.
- **Label** (400, 13 px): field labels, hints, table headers, card sub-captions.

Scale steps: 13 / 15 / 17 / 20 / 26 / 34 / 48 / clamp(48, 7vw, 104). Above 1900 px viewport the base text scales by 1.18 for TV browsers.

### Named Rules
**The Three-Metre Numeral Rule.** Numbers at small and medium sizes are set in Atkinson Hyperlegible 700 with tabular figures. Big Shoulders' "1" reads as a bar from the sofa, so the display face carries numerals only when they are large: the year at the foot of a programme card, and codes such as the party code and PIN entry.

**The Signage Rule.** The display face is for names and headings, never for running text, buttons or labels.

## Layout

Edge-to-edge foyer with a fluid gutter (clamp 16–56 px). The hero and detail backdrops break out of the gutter to full bleed at min(70vh, 720px) and sit under a translucent top bar; content anchors lower-left with a 680 px copy column.

Shelves are horizontal tracks of poster cases (clamp 124–176 px wide, 2:3) or landscape cases (clamp 228–328 px, 16:9) with 12 px gaps and proximity scroll-snap; the library grid auto-fills the same poster width with 24 px row gaps. Detail pages put a 200 px poster beside the title block; library and admin use a 240 px / 200 px sticky side column that collapses into a toggle or a horizontal scroller below 860 px. The top bar wraps its nav to a scrolling second line below 760 px; the player stacks the party panel under the video below 900 px.

Spacing follows a 4/8 scale (4, 8, 12, 16, 24, 32, 48, 64, 96). Spacing in markup goes through token utilities (stack, spread, narrow, mt1–mt7 and similar), never inline pixel styles; runtime geometry in script reads tokens.

### Named Rules
**The Token-Only Rule.** Every length, colour, duration and size comes from a `:root` token. The only literals allowed outside `:root` are media-query breakpoints, gradient angles, rotation degrees and `100vh`.

## Elevation & Depth

Flat by default. Depth is overlap: titles over backdrops behind the scrim, pop-overs and modals on Foyer Panel with a Rule High hairline, the player chrome over the picture. There are no drop shadows anywhere. The only `box-shadow` in the system is the focus ring itself, a two-ring offset (a foyer-black gap ring, then the amber ring) used on the lit poster case and the profile face. Focus also lifts the case by 4 % (`scale(1.04)`).

### Named Rules
**The No-Glow Rule.** Nothing casts a shadow or a halo. Light is signalled by amber colour on edges, never by blur.

## Shapes

Two radii only: 2 px (r1) for controls (buttons, fields, badges, menus, toasts, avatars) and 6 px (r2) for cases and sheets (poster cases, detail poster, modals, player pop-overs, profile faces). Circles are reserved for small status marks: radio buttons, the watched tick, toast and status dots, the timeline knob. Borders are hairline (1 px) for controls and dividers and a 2 px frame for poster cases and tab/nav underlines.

## Components

### Buttons
Rectangular and plain, labels in Atkinson 700.
- **Shape:** squared with barely softened corners (2 px), 44 px tall, 16 px side padding; the big variant is 52 px with 24 px padding at 17 px.
- **Primary:** amber fill, amber-ink label. Hover turns the fill plaster-bright. One per view: Play / Continue on the facade, the confirming action in a sheet.
- **Hairline (default):** transparent with a Rule High border and plaster-bright label; hover lifts the border to plaster-muted.
- **Ghost:** borderless plaster label; gains a Rule High border on hover.
- **Danger:** borderless danger label; danger border on hover.
- **Icon:** a 44 px square. **On** state: amber border and amber label.
- **Focus:** the global 3 px amber outline, 3 px offset.

### Chips
- **Style:** text on the wall, no container: plaster label at 15 px, a transparent 2 px underline, a muted tabular count in Atkinson.
- **State:** selected turns label and underline amber, the count amber-dim. Removable chips carry a 44 px hit area around a 16 px cross that turns danger on hover.

### Badges
A 13 px Atkinson 700 tabular label inside a 1 px Rule High hairline at 2 px radius. The hot variant is amber text on an amber-dim hairline. On the facade meta line and detail facts the badge drops its hairline and joins the dot-separated line at 17 px.

### Cards / Poster Cases
- **Corner Style:** 6 px.
- **Background:** Foyer Panel behind the artwork.
- **Border:** a 2 px Rule frame at rest; amber on hover, focus or when lit.
- **Lit state:** scale 1.04 plus the offset two-ring amber focus ring; the home facade swaps its backdrop and title to the lit case.
- **Caption:** title in plaster-bright 700 at 15 / 1.3, sub-caption in 13 px plaster-muted tabular; an amber-dim "why" line for recommendations.
- **Marks:** a 4 px amber progress bar on a scrim track along the foot; a 24 px amber watched tick; a 44 px "more" button on a scrim plate, shown on hover/focus and always on touch.

### Programme Card (no-artwork case)
The signature missing-artwork state: the case prints itself like a cinema programme. Uppercase title in Big Shoulders 800, sized by container query (clamp 20–48 px at 16cqi), under a Rule High top hairline; a second hairline at the foot introduces the year in the display face at clamp 20–48 px (22cqi), plaster-muted, tabular. Because the card prints its own title and year, the caption hides the title (kept for screen readers) and the caption year.

### Inputs / Fields
- **Style:** Foyer Lacquer interior, Rule High hairline, 2 px radius, 44 px minimum height, amber caret. Labels sit above in 13 px plaster-muted.
- **Focus:** border and a 3 px outline both turn amber, zero offset.
- **Checkbox / radio:** custom 24 px marks with a 2 px Rule High frame; checked fills amber with an amber-ink tick or dot.
- **Disabled:** 50 % opacity.

### Navigation
Words on the wall, no bar: the top bar is transparent over a scrim fade (solid Foyer Black with a Rule underline on non-facade pages). Wordmark in amber display 800 at 26 px. Links in Atkinson 700 plaster-muted, 44 px tall, with a 2 px underline that turns amber on the active route. Search is a hairline field on a scrim. The profile is a 44 px square avatar. Admin navigation is a ruled vertical list (amber label when active) that becomes an underlined horizontal scroller on narrow screens.

### Booth Ledger and Tables
Booth sections are separated by a top Rule, never boxed. Ledger figures are Atkinson 700 tabular at 26 px over 13 px muted labels. Tables have 13 px muted headers over a Rule High line, Rule-divided rows, flush-left first column, and status shown as dot plus colour plus text.

### Player
The only true-black surface. Chrome fades in over top and bottom scrims and hides when idle. Timeline: a 4 px rail (6 px on hover/focus) with buffered range in plaster-muted, played range and a 14 px knob in amber, chapter ticks in foyer-black. Pop-overs (quality, subtitles, "why this way") are Foyer Panel sheets with ruled display-face section heads. The party panel is a Foyer Lacquer side column with the join code in amber display at 34 px.

## Do's and Don'ts

### Do:
- **Do** reserve marquee amber for what is live: focus and the lit case, the one primary action, progress, the active nav item, the reason line.
- **Do** set every small or medium number in Atkinson Hyperlegible 700 with tabular figures.
- **Do** divide with 1 px Rule hairlines and let content sit on the ground; use a container only for floating layers (modals, menus, pop-overs, toasts).
- **Do** give every interactive target at least 44 px (hit token) and the 3 px amber offset focus ring; lift focused cases by 4 %.
- **Do** keep arrow-key / D-pad spatial navigation working on every new surface, with the facade following focus where there is one.
- **Do** design the no-artwork state as a programme card, not a placeholder.
- **Do** honour prefers-reduced-motion: no lift, no transitions, slowed spinner, instant reactions.
- **Do** use the motion tokens: 160 ms out, 240 ms in, one shared ease curve.
- **Do** pull every value from a `:root` token; script geometry reads tokens too.

### Don't:
- **Don't** put amber on a resting poster case, a profile colour, or decoration.
- **Don't** use blue, purple, or pure black outside the player screen.
- **Don't** add drop shadows, glows, or gradient fills to buttons or surfaces; the scrim is the only gradient.
- **Don't** nest a card inside a card or box booth content in outlined panels.
- **Don't** set small numerals, buttons or running text in Big Shoulders Display.
- **Don't** use emoji as icons; icons are inline 24-unit SVG strokes.
- **Don't** use a radius other than 2 px or 6 px (circles only for small status marks).
- **Don't** write inline pixel styles or literal colours outside `:root`.
