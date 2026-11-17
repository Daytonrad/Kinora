// Server theme (theme plugins, milestone 8) and server branding accent.
// The server serves the active theme's token overrides as CSS; the app links it once per server.
import type { Api } from './api';
import { state } from './app';

let applied = '';

export function applyServerTheme(a: Api): void {
  const key = a.base + '|' + (state.me && state.me.profile ? state.me.profile.id : 0);
  if (key === applied) return;
  applied = key;
  const old = document.getElementById('server-theme');
  if (old && old.parentNode) old.parentNode.removeChild(old);
  if (!state.me || !state.me.profile) return;
  const mode = document.documentElement.className.indexOf('mode-tv') >= 0 ? 'tv' : 'mobile';
  const link = document.createElement('link');
  link.id = 'server-theme';
  link.rel = 'stylesheet';
  link.href = `${a.base}/api/themes/active.css?mode=${mode}&profile=${state.me.profile.id}${a.mediaToken ? '&st=' + encodeURIComponent(a.mediaToken) : ''}`;
  link.onerror = () => { if (link.parentNode) link.parentNode.removeChild(link); }; // older server: no themes
  document.head.appendChild(link);
  const accent = state.branding && state.branding.accent;
  if (accent && /^#[0-9a-fA-F]{6}$/.test(accent)) document.documentElement.style.setProperty('--amber', accent);
}
