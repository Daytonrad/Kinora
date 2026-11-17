// Platform detection, remote-control keys, secure storage and app exit for
// Android/Android TV (Capacitor), iOS (Capacitor), Samsung Tizen, LG webOS and the browser.
import { Capacitor, registerPlugin } from '@capacitor/core';
import { App } from '@capacitor/app';
import { SecureStorage } from '@aparajita/capacitor-secure-storage';

export type Platform = 'android' | 'androidtv' | 'ios' | 'tizen' | 'webos' | 'web';
export type Mode = 'mobile' | 'tv';

declare const __APP_VERSION__: string;
export const APP_VERSION: string = typeof __APP_VERSION__ !== 'undefined' ? __APP_VERSION__ : '1.0.0';
// Oldest server API level this app understands, and the level it was written for.
export const CLIENT_API = 2;
export const MIN_SERVER_API = 2;

export interface DiscoveredServer { name: string; address: string; id: string; version: string; public_url?: string }

// Native helper implemented in apps/android (KinoraNativePlugin.java) and apps/ios (KinoraNativePlugin.swift).
interface KinoraNativePlugin {
  info(): Promise<{ tv: boolean; model: string }>;
  discover(opts: { timeoutMs: number; port: number }): Promise<{ servers: DiscoveredServer[] }>;
  enterPip(): Promise<void>;
  keepAwake(opts: { on: boolean }): Promise<void>;
}
export const KinoraNative = registerPlugin<KinoraNativePlugin>('KinoraNative');

const w = window as any;
const ua = navigator.userAgent;
export const isNative = Capacitor.isNativePlatform();

let platform: Platform = 'web';
if (typeof w.tizen !== 'undefined' || /Tizen/i.test(ua)) platform = 'tizen';
else if (typeof w.webOS !== 'undefined' || typeof w.PalmSystem !== 'undefined' || /Web0S|webOS/i.test(ua)) platform = 'webos';
else if (isNative) platform = Capacitor.getPlatform() === 'ios' ? 'ios' : 'android';

export let deviceModel = '';

/** Resolves the platform (Android TV needs a native call). Call once at startup. */
export async function initPlatform(): Promise<void> {
  if (platform === 'android') {
    try {
      const i = await KinoraNative.info();
      if (i.tv) platform = 'androidtv';
      deviceModel = i.model || '';
    } catch { /* plugin missing: treat as phone */ }
  }
  if (platform === 'tizen') {
    try {
      const reg = w.tizen.tvinputdevice;
      for (const k of ['MediaPlayPause', 'MediaPlay', 'MediaPause', 'MediaStop', 'MediaFastForward', 'MediaRewind',
        'MediaTrackPrevious', 'MediaTrackNext', 'ColorF0Red', 'ColorF1Green', 'ColorF2Yellow', 'ColorF3Blue', 'Info', 'Caption']) {
        try { reg.registerKey(k); } catch { /* key not on this model */ }
      }
    } catch { /* not running as a packaged app */ }
    try { deviceModel = w.webapis && w.webapis.productinfo ? w.webapis.productinfo.getRealModel() : 'Samsung TV'; } catch { deviceModel = 'Samsung TV'; }
  }
  if (platform === 'webos') deviceModel = 'LG TV';
  if (platform === 'ios') deviceModel = /iPad/.test(ua) ? 'iPad' : 'iPhone';
}

export const getPlatform = (): Platform => platform;
export const isTVPlatform = (): boolean => platform === 'tizen' || platform === 'webos' || platform === 'androidtv';

export function deviceName(): string {
  if (deviceModel) return deviceModel;
  return T_PLATFORM[platform];
}
const T_PLATFORM: Record<Platform, string> = { android: 'Android telefon', androidtv: 'Android TV', ios: 'iPhone', tizen: 'Samsung TV', webos: 'LG TV', web: 'Böngésző' };

// ------------------------------------------------------------------ keys

export type Action = 'up' | 'down' | 'left' | 'right' | 'ok' | 'back' | 'playpause' | 'play' | 'pause' | 'stop' | 'ff' | 'rw' | 'next' | 'prev' | 'red' | 'green' | 'yellow' | 'blue' | 'info' | null;

/** Maps a key event from any remote / keyboard to an action. */
export function keyAction(e: KeyboardEvent): Action {
  switch (e.keyCode) {
    case 37: return 'left';
    case 38: return 'up';
    case 39: return 'right';
    case 40: return 'down';
    case 13: return 'ok';
    case 10009: case 461: case 27: return 'back'; // Tizen, webOS, keyboard
    case 8: return isTyping(e.target) ? null : 'back';
    case 10252: case 179: return 'playpause';
    case 415: return 'play';
    case 19: return 'pause';
    case 413: case 178: return 'stop';
    case 417: case 228: return 'ff';
    case 412: case 227: return 'rw';
    case 10233: case 176: return 'next';
    case 10232: case 177: return 'prev';
    case 403: return 'red';
    case 404: return 'green';
    case 405: return 'yellow';
    case 406: return 'blue';
    case 457: case 10135: return 'info';
    case 32: return isTyping(e.target) ? null : 'playpause';
  }
  switch (e.key) {
    case 'MediaPlayPause': return 'playpause';
    case 'GoBack': case 'BrowserBack': return 'back';
  }
  return null;
}

export function isTyping(t: EventTarget | null): boolean {
  const el = t as HTMLElement | null;
  return !!el && (el.tagName === 'INPUT' || el.tagName === 'TEXTAREA' || el.isContentEditable);
}

/** Android's hardware back button arrives through Capacitor, not as a key event. */
export function onNativeBack(cb: () => void): void {
  if (isNative) App.addListener('backButton', () => cb());
}

export function exitApp(): void {
  try {
    if (platform === 'tizen') return w.tizen.application.getCurrentApplication().exit();
    if (platform === 'webos') { if (w.webOS && w.webOS.platformBack) return w.webOS.platformBack(); return window.close(); }
    if (isNative) { App.exitApp(); return; }
  } catch { /* fall through */ }
  window.close();
}

// ------------------------------------------------------------------ storage
// Server list and tokens: Keychain (iOS) / Keystore-backed storage (Android) on phones,
// localStorage on TVs and in the browser.

export async function storeGet(key: string): Promise<string | null> {
  if (isNative) {
    try { return await SecureStorage.getItem(key); } catch { return null; }
  }
  try { return localStorage.getItem(key); } catch { return null; }
}

export async function storeSet(key: string, value: string | null): Promise<void> {
  if (isNative) {
    try { if (value === null) await SecureStorage.removeItem(key); else await SecureStorage.setItem(key, value); } catch { /* ignore */ }
    return;
  }
  try { if (value === null) localStorage.removeItem(key); else localStorage.setItem(key, value); } catch { /* storage blocked */ }
}

/** Small non-secret preferences (UI mode, skip-intro mode…): always localStorage. */
export function prefGet(key: string, def: string): string {
  try { const v = localStorage.getItem('kinora.pref.' + key); return v === null ? def : v; } catch { return def; }
}
export function prefSet(key: string, v: string): void {
  try { localStorage.setItem('kinora.pref.' + key, v); } catch { /* ignore */ }
}

export function uiMode(): Mode {
  const q = /[?&]mode=(tv|mobile)/.exec(location.search);
  if (q) return q[1] as Mode;
  const p = prefGet('uiMode', 'auto');
  if (p === 'tv' || p === 'mobile') return p;
  return isTVPlatform() ? 'tv' : 'mobile';
}
