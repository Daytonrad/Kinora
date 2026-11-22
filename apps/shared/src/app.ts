// App state, router, layers (sheets/dialogs), back handling and shared UI pieces.
import { h, put, icon, toast, Child } from './dom';
import { T } from './i18n';
import { Api, SavedServer, loadServers, currentServerId, saveServers, setCurrentServer, serverURLs } from './api';
import { isLocalHost } from './address';
import { uiMode, Mode, exitApp, keyAction, onNativeBack, isTyping } from './platform';
import { move, focusFirst, focusEl, scope } from './focus';

export interface Profile { id: number; name: string; color: string; has_pin: boolean; is_kids: boolean; sub_langs: string; audio_langs: string; sub_mode: string; autoplay_next: boolean; max_bitrate: number; skip_intro?: string }
export interface Me { user: { id: number; username: string; is_admin: boolean }; profile: Profile | null; profiles: Profile[]; profile_locked: boolean; device_id: string; media_token: string; server_name: string; version: string }
export interface Item {
  id: number; kind: string; title: string; year?: number; overview?: string; series_title?: string; series_id?: number;
  season_num?: number; episode_num?: number; runtime_sec?: number; content_rating?: string; community_rating?: number;
  has_poster: boolean; has_backdrop: boolean; genres?: string[]; unplayed_count?: number; episode_count?: number;
  user_data: { position: number; played: boolean; favorite: boolean; stars: number | null; hidden: boolean };
  video_codec?: string; audio_codec?: string; width?: number; height?: number; hdr?: string; container?: string;
}

export const state = {
  mode: 'mobile' as Mode,
  api: null as Api | null,
  me: null as Me | null,
  branding: null as null | { server_name: string; accent?: string; logo_dark?: string; logo_light?: string },
  cleanup: [] as (() => void)[],
  playerActive: false,
  keyHandler: null as null | ((a: string, e: KeyboardEvent) => boolean), // view-level key hook (player)
};

export const api = (): Api => state.api as Api;
export const onCleanup = (f: () => void) => state.cleanup.push(f);

// ------------------------------------------------------------------ routing

export type View = (parts: string[], q: URLSearchParams) => Promise<void> | void;
const routes: Record<string, View> = {};
export function route(name: string, v: View) { routes[name] = v; }

export function go(hash: string, replace = false): void {
  if (location.hash === hash) { render(); return; }
  if (replace) location.replace(hash); else location.hash = hash;
}

export function parseHash(): { parts: string[]; q: URLSearchParams } {
  const raw = location.hash.slice(1) || '/';
  const i = raw.indexOf('?');
  const path = i >= 0 ? raw.slice(0, i) : raw;
  return { parts: path.split('/').filter(Boolean), q: new URLSearchParams(i >= 0 ? raw.slice(i + 1) : '') };
}

// Restores api + session after a cold start on a deep link (set by views/auth.ts).
let restoreSession: (needToken: boolean) => Promise<boolean> = async () => true;
export function setRestore(f: (needToken: boolean) => Promise<boolean>) { restoreSession = f; }
const NO_SESSION = ['start', 'servers'];
const NO_TOKEN = ['login', 'qc'];

let rendering = 0;
export async function render(): Promise<void> {
  const my = ++rendering;
  for (const f of state.cleanup.splice(0)) { try { f(); } catch { /* ignore */ } }
  closeAllLayers();
  state.keyHandler = null;
  const { parts, q } = parseHash();
  const name = parts[0] || 'start';
  const v = routes[name] || routes.start;
  try {
    if (NO_SESSION.indexOf(name) < 0 && (!state.api || (!state.me && NO_TOKEN.indexOf(name) < 0))) {
      if (!(await restoreSession(NO_TOKEN.indexOf(name) < 0))) return;
      if (my !== rendering) return;
    }
    await v(parts, q);
  } catch (e: any) {
    if (my !== rendering) return;
    if (e && e.status === 0) return offline();
    if (e && e.status === 428) return go('#/profiles', true);
    if (e && e.status !== 401) toast(e.message || String(e), true);
  }
  if (my === rendering && state.mode === 'tv' && !state.playerActive) {
    const a = document.activeElement;
    if (!a || a === document.body || !document.getElementById('app')!.contains(a)) focusFirst();
  }
}

// ------------------------------------------------------------------ layers (sheets & dialogs)

export interface Layer { el: HTMLElement; close: () => void; prevFocus: Element | null }
const layers: Layer[] = [];

/** Opens a sheet (mobile: bottom sheet, TV: centred panel). Returns its close function. */
export function sheet(title: string | null, body: Child, opts: { wide?: boolean; onClose?: () => void } = {}): Layer {
  const prevFocus = document.activeElement;
  const panel = h('div.sheet' + (opts.wide ? '.wide' : ''), { role: 'dialog', 'aria-modal': 'true', 'aria-label': title || '' },
    title && h('h2.sheet-title', null, title), body);
  const el = h('div.layer', { onclick: (e: Event) => { if (e.target === el) layer.close(); } }, panel);
  const layer: Layer = {
    el, prevFocus,
    close: () => {
      const i = layers.indexOf(layer);
      if (i < 0) return;
      layers.splice(i, 1);
      el.parentNode && el.parentNode.removeChild(el);
      opts.onClose && opts.onClose();
      if (prevFocus && (prevFocus as HTMLElement).focus && document.body.contains(prevFocus)) focusEl(prevFocus as HTMLElement);
    },
  };
  layers.push(layer);
  document.body.appendChild(el);
  window.setTimeout(() => focusFirst(), 0);
  return layer;
}

export function closeTopLayer(): boolean {
  const l = layers[layers.length - 1];
  if (!l) return false;
  l.close();
  return true;
}
function closeAllLayers() { while (layers.length) layers[layers.length - 1].close(); }

export function confirmBox(text: string, yes: string, no: string): Promise<boolean> {
  return new Promise(resolve => {
    let answered = false;
    const done = (v: boolean) => { if (!answered) { answered = true; l.close(); resolve(v); } };
    const l = sheet(null, h('div', null, h('p.lede', null, text), h('div.actions', null,
      h('button.btn', { onclick: () => done(false), 'data-autofocus': '' }, no),
      h('button.btn.primary', { onclick: () => done(true) }, yes))), { onClose: () => { if (!answered) { answered = true; resolve(false); } } });
  });
}

/** Choice list in a sheet; resolves with the chosen value or null. */
export function choose<V>(title: string, options: [V, string][], current?: V): Promise<V | null> {
  return new Promise(resolve => {
    let picked = false;
    const l = sheet(title, h('div.choices', null, options.map(([v, label]) =>
      h('button.choice' + (v === current ? '.on' : ''), { onclick: () => { picked = true; l.close(); resolve(v); }, 'data-autofocus': v === current ? '' : null },
        v === current && icon('check'), h('span', null, label)))),
      { onClose: () => { if (!picked) resolve(null); } });
  });
}

// ------------------------------------------------------------------ back & keys

export async function back(): Promise<void> {
  if (closeTopLayer()) return;
  const { parts } = parseHash();
  const top = parts[0] || 'home';
  if (top === 'home' || top === 'servers' || top === 'login' || top === 'start') {
    if (await confirmBox(T.exitConfirm, T.exit, T.stay)) exitApp();
    return;
  }
  if (top === 'profiles' && state.me && state.me.profile) return go('#/home');
  if (history.length > 1) history.back(); else go('#/home');
}

export function installKeys(): void {
  document.addEventListener('keydown', e => {
    const a = keyAction(e);
    if (!a) return;
    if (state.keyHandler && state.keyHandler(a, e)) { e.preventDefault(); return; }
    if (a === 'back') { e.preventDefault(); back(); return; }
    if (state.mode !== 'tv' && !document.querySelector('.layer')) return;
    if (a === 'up' || a === 'down' || a === 'left' || a === 'right') {
      const t = e.target as HTMLElement;
      if (isTyping(t) && (a === 'left' || a === 'right')) {
        const inp = t as HTMLInputElement;
        try {
          const atStart = inp.selectionStart === 0, atEnd = inp.selectionEnd === (inp.value || '').length;
          if ((a === 'left' && !atStart) || (a === 'right' && !atEnd)) return; // move the caret
        } catch { /* number inputs have no selection */ }
      }
      if (t && t.tagName === 'SELECT' && (a === 'up' || a === 'down')) return;
      if (move(a)) e.preventDefault();
    }
  });
  onNativeBack(() => back());
}

// ------------------------------------------------------------------ shell

const NAV: [string, string, string][] = [['home', 'home', T.home], ['library', 'grid', T.library], ['search', 'search', T.search], ['settings', 'gear', T.settings]];

/** Page frame: TV gets a left rail, phones a bottom tab bar. Returns the content element. */
export function shell(active: string, ...content: Child[]): HTMLElement {
  const root = document.getElementById('app')!;
  const main = h('main.main', null, content);
  const me = state.me;
  const p = me && me.profile;
  const nav = h('nav.nav' + (state.mode === 'tv' ? '.rail' : '.tabs'), { 'aria-label': T.appName },
    state.mode === 'tv' && h('div.rail-mark', null, brandMark()),
    NAV.map(([key, ic, label]) => h('a.nav-item' + (key === active ? '.on' : ''), { href: key === 'library' ? '#/library' : '#/' + key, 'aria-label': label },
      icon(ic), h('span', null, label))),
    state.mode === 'tv' && p && h('a.nav-item.nav-profile', { href: '#/profiles', 'aria-label': p.name },
      h('span.face.small', { style: { background: p.color } }, p.name.slice(0, 1).toUpperCase()), h('span', null, p.name)));
  put(root, h('div.frame.' + state.mode, null, nav, main));
  return main;
}

export function brandMark(): HTMLElement {
  const b = state.branding;
  if (b && b.logo_dark && state.api) return h('img.brand-logo', { src: state.api.abs(b.logo_dark), alt: b.server_name || T.appName });
  return h('span.wordmark', null, (b && b.server_name) || T.appName);
}

// ------------------------------------------------------------------ cards

export function itemSub(it: Item): string {
  if (it.kind === 'episode') return `${it.series_title || ''} · ${T.ep(it.season_num, it.episode_num)}`;
  const bits: string[] = [];
  if (it.year) bits.push(String(it.year));
  if (it.kind === 'series' && it.unplayed_count) bits.push(`${it.unplayed_count} új`);
  return bits.join(' · ');
}

function programmeCard(it: Item): HTMLElement {
  return h('div.programme', null, h('span.pt', null, it.kind === 'episode' ? it.series_title || it.title : it.title),
    h('span.py', null, it.year ? String(it.year) : it.kind === 'episode' ? T.ep(it.season_num, it.episode_num) : ''));
}

function art(it: Item, kind: 'poster' | 'thumb'): HTMLElement {
  const has = kind === 'poster' ? it.has_poster : true;
  const box = h('div.art');
  if (!has) { box.appendChild(programmeCard(it)); return box; }
  const img = h('img', { alt: '', loading: 'lazy', src: api().image(it.id, kind) }) as HTMLImageElement;
  img.onerror = () => { put(box, programmeCard(it)); };
  box.appendChild(img);
  return box;
}

export function card(it: Item, style: 'poster' | 'landscape', opts: { reason?: string; onFocus?: (it: Item) => void } = {}): HTMLElement {
  const href = `#/item/${it.id}`;
  const ud = it.user_data || ({} as Item['user_data']);
  const pct = it.runtime_sec && ud.position > 0 && !ud.played ? Math.min(100, ud.position / it.runtime_sec * 100) : 0;
  const el = h('a.card.' + style, { href, 'aria-label': it.title + (itemSub(it) ? ', ' + itemSub(it) : '') },
    h('div.case', null, art(it, style === 'poster' ? 'poster' : 'thumb'),
      pct > 0 && h('div.progress', null, h('i', { style: { width: pct.toFixed(1) + '%' } })),
      ud.played && h('span.tick', null, icon('check'))),
    h('div.cap', null, h('span.ct', null, it.kind === 'episode' && style === 'landscape' ? it.series_title || it.title : it.title),
      h('span.cs', null, it.kind === 'episode' && style === 'landscape' ? `${T.ep(it.season_num, it.episode_num)} · ${it.title}` : itemSub(it)),
      opts.reason && h('span.cr', null, opts.reason)));
  if (opts.onFocus) el.addEventListener('focus', () => opts.onFocus!(it));
  return el;
}

export function shelf(title: string, items: Item[], style: 'poster' | 'landscape', opts: { reasons?: Record<string, string>; onFocus?: (it: Item) => void } = {}): HTMLElement {
  return h('section.shelf', null, h('h2.shelf-title', null, title),
    h('div.track', { 'data-row': '' }, items.map(it => card(it, style, { reason: opts.reasons && opts.reasons[it.id], onFocus: opts.onFocus }))));
}

export function spinner(): HTMLElement { return h('div.spinner', { role: 'progressbar', 'aria-label': T.loading }); }

// ------------------------------------------------------------------ session helpers

export async function currentServer(): Promise<SavedServer | null> {
  const id = await currentServerId();
  const list = await loadServers();
  return list.filter(s => s.id === id)[0] || null;
}

/** Ends the session on the server and forgets its token here, then shows the login. */
export async function signOut(): Promise<void> {
  try { await api().post('/api/logout'); } catch { /* already gone */ }
  const s = state.api!.server;
  s.token = undefined;
  await persistServer(s);
  state.me = null;
  go('#/login', true);
}

export async function persistServer(s: SavedServer): Promise<void> {
  const list = await loadServers();
  const i = list.map(x => x.id).indexOf(s.id);
  if (i >= 0) list[i] = s; else list.unshift(s);
  await saveServers(list);
  await setCurrentServer(s.id);
  if (state.api && state.api.server.id === s.id) state.api.server = s;
}

export async function loadMe(): Promise<Me> {
  const me = await api().get<Me>('/api/me');
  state.me = me;
  api().mediaToken = me.media_token;
  return me;
}

// "Server unreachable" screen with retry and server switch.
export function offline(): void {
  const s = state.api && state.api.server;
  const root = document.getElementById('app')!;
  put(root, h('div.center.tall', null, h('div.panel-auth', null,
    icon('wifi'), h('h1', null, T.offlineTitle), h('p.lede', null, T.offlineLede(s ? s.name + ' (' + s.url + ')' : '')),
    s && h('p.muted.small', null, T.offlineTried(serverURLs(s).join(', '))),
    s && serverURLs(s).every(u => isLocalHost(u.replace(/^https?:\/\//, '').split(/[:/]/)[0])) && h('p.muted', null, T.offlineAway),
    h('div.actions', null,
      h('button.btn.primary.big', { 'data-autofocus': '', onclick: () => render() }, T.retry),
      h('a.btn.big', { href: '#/servers' }, T.switchServer)))));
  if (state.mode === 'tv') focusFirst();
}

export function setMode(m: Mode) {
  state.mode = m;
  document.documentElement.className = 'mode-' + m;
}

export function initMode() { setMode(uiMode()); }

export { scope };
