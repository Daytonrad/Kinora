// Server → login (Quick Connect first on TV) → profile picker.
import qrcode from 'qrcode-generator';
import { h, put, icon, toast } from '../dom';
import { T } from '../i18n';
import { Api, SavedServer, resolveServer, loadServers, removeServer, setCurrentServer, upsertServer, HTTPError } from '../api';
import { discover } from '../discover';
import { state, go, route, sheet, onCleanup, persistServer, loadMe, currentServer, brandMark, Profile, render, setRestore, signOut } from '../app';
import { focusFirst } from '../focus';
import { APP_VERSION } from '../platform';
import { mergeURLs } from '../address';

function authFrame(...kids: any[]): HTMLElement {
  const root = document.getElementById('app')!;
  const box = h('div.panel-auth', null, kids);
  put(root, h('div.center.tall.auth-page', null, box));
  return box;
}

async function useServer(s: SavedServer): Promise<void> {
  state.api = new Api(s);
  state.api.onUnauthorized = () => onSessionLost();
  state.api.onAddressChange = srv => { persistServer(srv); };
  // learn the public address (Admin → Beállítások → Nyilvános URL), so the app keeps
  // working away from home; the request itself fails over if the saved address is gone
  const st = await state.api.get('/api/status').catch(() => null);
  if (st && st.public_url) {
    const urls = mergeURLs(s.url, s.urls, st.public_url);
    if (urls.join() !== mergeURLs(s.url, s.urls).join()) { s.urls = urls; persistServer(s); }
  }
  state.branding = await state.api.get('/api/branding').catch(() => null);
}

async function onSessionLost() {
  const s = state.api && state.api.server;
  if (s && s.token) {
    s.token = undefined;
    await persistServer(s);
    toast(T.sessionExpired, true);
  }
  state.me = null;
  go('#/login', true);
}

// ------------------------------------------------------------------ start

setRestore(async needToken => {
  const s = await currentServer();
  if (!s) { go('#/servers', true); return false; }
  if (!state.api || state.api.server.id !== s.id) await useServer(s);
  if (!needToken) return true;
  if (!s.token) { go('#/login', true); return false; }
  if (!state.me) await loadMe(); // offline → HTTPError(0) → offline screen
  return true;
});

route('start', async () => {
  const s = await currentServer();
  if (!s) return go('#/servers', true);
  await useServer(s);
  if (!s.token) return go('#/login', true);
  try {
    await loadMe();
  } catch (e) {
    if (e instanceof HTTPError && e.status === 0) throw e; // offline screen
    return;
  }
  go(state.me && state.me.profile ? '#/home' : '#/profiles', true);
});

// ------------------------------------------------------------------ servers

route('servers', async () => {
  const saved = await loadServers();
  const input = h('input.addr', { type: 'text', inputmode: 'url', autocomplete: 'off', autocapitalize: 'off', spellcheck: 'false',
    placeholder: T.serverAddressPh, 'aria-label': T.serverAddress, 'data-autofocus': saved.length ? null : '' }) as HTMLInputElement;
  const err = h('p.err', { role: 'alert' });
  const found = h('div.found');
  const submit = h('button.btn.primary.big', { type: 'submit' }, T.connect) as HTMLButtonElement;

  const connectTo = async (addr: string) => {
    err.textContent = '';
    submit.disabled = true;
    submit.textContent = T.connecting;
    const r = await resolveServer(addr);
    submit.disabled = false;
    submit.textContent = T.connect;
    if ('error' in r) { err.textContent = r.error; return; }
    const prev = saved.filter(x => x.id === (r.status.server_id || r.url))[0];
    const s: SavedServer = { id: r.status.server_id || r.url, url: r.url, name: r.status.server_name, token: prev && prev.token,
      deviceId: prev && prev.deviceId, username: prev && prev.username, lastUsed: Date.now() };
    await upsertServer(s);
    go('#/start');
  };

  const keypad = h('div.keypad', { 'aria-label': T.keypadHint }, ['1', '2', '3', '4', '5', '6', '7', '8', '9', '.', '0', ':', '⌫'].map(k =>
    h('button.key', { type: 'button', onclick: () => {
      if (k === '⌫') input.value = input.value.slice(0, -1); else input.value += k;
    } }, k)));

  const searchBtn = h('button.btn', { type: 'button', onclick: async () => {
    put(found, h('p.muted', null, T.searching));
    let any = false;
    const list = h('div.choices');
    await discover(s => {
      if (!any) { any = true; put(found, h('h2.sub', null, T.foundServers), list); }
      list.appendChild(h('button.choice', { onclick: () => connectTo(s.address) }, icon('server'), h('span', null, s.name, h('small', null, s.address))));
    });
    if (!any) put(found, h('p.muted', null, T.noServersFound));
  } }, icon('wifi'), T.searchServers);

  const box = authFrame(
    h('span.wordmark.big', null, T.appName),
    h('h1', null, T.addServer), h('p.lede', null, T.addServerLede),
    h('form', { onsubmit: (e: Event) => { e.preventDefault(); connectTo(input.value); } },
      h('label.field', null, h('span', null, T.serverAddress), input),
      state.mode === 'tv' && keypad,
      err,
      h('div.actions', null, submit, searchBtn)),
    found);

  if (saved.length) {
    box.appendChild(h('div.saved', null, h('h2.sub', null, T.savedServers), h('div.choices', null, saved.map((s, i) =>
      h('div.saved-row', null,
        h('button.choice', { 'data-autofocus': i === 0 ? '' : null, onclick: async () => { await setCurrentServer(s.id); go('#/start'); } },
          icon('server'), h('span', null, s.name, h('small', null, s.url + (s.username ? ' · ' + s.username : '')))),
        h('button.btn.ghost.icon', { 'aria-label': T.removeServer + ': ' + s.name, onclick: async () => { await removeServer(s.id); render(); } }, icon('x')))))));
  }
  box.appendChild(h('p.muted.small.ver', null, `${T.appName} ${APP_VERSION}`));
});

// ------------------------------------------------------------------ login

route('login', async (_p, q) => {
  const s = await currentServer();
  if (!s) return go('#/servers', true);
  if (!state.api || state.api.server.id !== s.id) await useServer(s);
  const status = await state.api!.get('/api/status');
  const qcOn = status.quickconnect_enabled !== false;
  const usePw = q.get('pw') === '1' || !qcOn;
  if (!usePw && state.mode === 'tv') return quickConnect(s, status);
  if (!usePw) return passwordLogin(s, qcOn, true);
  return passwordLogin(s, qcOn, false);
});

function header(s: SavedServer): HTMLElement {
  return h('div.auth-head', null, brandMark(), h('p.muted', null, s.name + ' · ' + s.url));
}

function passwordLogin(s: SavedServer, qcOn: boolean, offerCodeFirst: boolean) {
  const user = h('input', { type: 'text', autocomplete: 'username', autocapitalize: 'off', value: s.username || '', 'data-autofocus': '' }) as HTMLInputElement;
  const pw = h('input', { type: 'password', autocomplete: 'current-password' }) as HTMLInputElement;
  const err = h('p.err', { role: 'alert' });
  authFrame(header(s), h('h1', null, T.loginTitle),
    h('form', { onsubmit: async (e: Event) => {
      e.preventDefault();
      err.textContent = '';
      try {
        const r = await state.api!.post('/api/login', Object.assign({ username: user.value.trim(), password: pw.value }, state.api!.device()));
        s.token = r.token; s.deviceId = r.device_id; s.username = user.value.trim(); s.lastUsed = Date.now();
        await persistServer(s);
        await loadMe();
        go('#/profiles', true);
      } catch (ex: any) {
        err.textContent = ex.status === 401 ? T.badLogin : ex.message;
      }
    } }, h('label.field', null, h('span', null, T.username), user), h('label.field', null, h('span', null, T.password), pw), err,
      h('div.actions', null, h('button.btn.primary.big', { type: 'submit' }, T.login),
        qcOn && h('a.btn.big', { href: '#/qc' }, icon('key'), T.loginWithCode))),
    h('a.btn.ghost', { href: '#/servers' }, T.otherServer));
  void offerCodeFirst;
}

route('qc', async () => {
  const s = await currentServer();
  if (!s) return go('#/servers', true);
  if (!state.api) await useServer(s);
  const status = await state.api!.get('/api/status');
  return quickConnect(s, status);
});

async function quickConnect(s: SavedServer, status: any) {
  const box = authFrame(header(s), h('h1', null, T.qcTitle), h('div.spinner'));
  let timer = 0, alive = true;
  onCleanup(() => { alive = false; window.clearTimeout(timer); });
  const start = async () => {
    let ini: any;
    try { ini = await state.api!.post('/api/quickconnect/initiate', state.api!.device()); }
    catch (e: any) {
      put(box, header(s), h('h1', null, T.qcTitle), h('p.err', null, e.status === 403 ? T.qcDisabled : e.message),
        h('a.btn.primary.big', { href: '#/login?pw=1', 'data-autofocus': '' }, T.loginWithPassword));
      focusFirst();
      return;
    }
    const approveURL = s.url + (ini.approve_path || '/#/quickconnect?code=' + ini.code);
    const qr = qrcode(0, 'M');
    qr.addData(approveURL);
    qr.make();
    const qrBox = h('div.qr', { 'aria-hidden': 'true' });
    qrBox.innerHTML = qr.createSvgTag({ cellSize: 6, margin: 2, scalable: true });
    const statusLine = h('p.muted', { role: 'status' }, T.qcWaiting);
    put(box, header(s), h('h1', null, T.qcTitle),
      h('div.qc', null,
        h('div.qc-main', null,
          h('div.code-big', { 'aria-label': ini.code.split('').join(' ') }, ini.code.slice(0, 3), h('span.code-gap'), ini.code.slice(3)),
          h('p.lede', null, T.qcSteps(s.url)), h('p.muted', null, T.qcMenuHint), statusLine),
        qrBox),
      h('div.actions', null,
        h('a.btn.big', { href: '#/login?pw=1', 'data-autofocus': '' }, T.loginWithPassword),
        h('a.btn.ghost.big', { href: '#/servers' }, T.otherServer)));
    focusFirst();
    const poll = async () => {
      if (!alive) return;
      let st: any;
      try { st = await state.api!.get('/api/quickconnect/status?secret=' + encodeURIComponent(ini.secret)); }
      catch (e: any) { st = e.body || { status: e.status === 0 ? 'pending' : 'expired' }; }
      if (!alive) return;
      if (st.status === 'approved') {
        s.token = st.token; s.deviceId = st.device_id; s.username = st.user && st.user.username; s.lastUsed = Date.now();
        await persistServer(s);
        await loadMe();
        if (st.needs_pin && st.profile_id) return go('#/profiles?pin=' + st.profile_id, true);
        return go(state.me && state.me.profile ? '#/home' : '#/profiles', true);
      }
      if (st.status === 'denied' || st.status === 'expired') {
        put(statusLine, st.status === 'denied' ? T.qcDenied : T.qcExpired, ' ',
          h('button.btn.primary', { onclick: () => start() }, T.newCode));
        return;
      }
      timer = window.setTimeout(poll, (ini.poll_interval || 2) * 1000);
    };
    timer = window.setTimeout(poll, 2000);
  };
  void status;
  await start();
}

// ------------------------------------------------------------------ profiles

route('profiles', async (_p, q) => {
  if (!state.api) return go('#/start', true);
  const me = await loadMe();
  const grid = h('div.who-grid', { 'data-row': '' });
  me.profiles.forEach((p, i) => grid.appendChild(
    h('button.who', { onclick: () => selectProfile(p), 'aria-label': p.name, 'data-autofocus': (me.profile ? me.profile.id === p.id : i === 0) ? '' : null },
      h('span.face', { style: { background: p.color } }, p.name.slice(0, 1).toUpperCase(), p.has_pin && h('span.lock', null, icon('lock'))),
      h('span.who-name', null, p.name), p.is_kids && h('small', null, T.kids))));
  const root = document.getElementById('app')!;
  put(root, h('div.center.tall', null, h('div.who-page', null, brandMark(), h('h1', null, T.whoWatches), grid,
    // profiles belong to one account; another family member signs in with their own
    !me.profile_locked && h('div.actions', null, h('button.btn.ghost', { onclick: () => signOut() }, T.switchUser)))));
  const pinFor = +(q.get('pin') || 0);
  const target = me.profiles.filter(p => p.id === pinFor)[0];
  if (target) selectProfile(target);
});

async function selectProfile(p: Profile) {
  const done = async () => { await loadMe(); go('#/home', true); };
  if (!p.has_pin) {
    await state.api!.post(`/api/profiles/${p.id}/select`, {});
    return done();
  }
  const pin = h('input.pin', { type: 'password', inputmode: 'numeric', pattern: '[0-9]*', maxlength: 8, autocomplete: 'off', 'aria-label': T.pin, 'data-autofocus': '' }) as HTMLInputElement;
  const err = h('p.err', { role: 'alert' });
  const digits = h('div.keypad.pinpad', null, ['1', '2', '3', '4', '5', '6', '7', '8', '9', '⌫', '0', '✓'].map(k =>
    h('button.key', { type: 'button', onclick: () => {
      if (k === '⌫') pin.value = pin.value.slice(0, -1);
      else if (k === '✓') submit();
      else if (pin.value.length < 8) pin.value += k;
    } }, k)));
  const submit = async () => {
    try { await state.api!.post(`/api/profiles/${p.id}/select`, { pin: pin.value }); l.close(); done(); }
    catch (e: any) { err.textContent = e.status === 403 ? T.wrongPin : e.message; pin.value = ''; }
  };
  const l = sheet(T.pinFor(p.name), h('form', { onsubmit: (e: Event) => { e.preventDefault(); submit(); } }, pin, err,
    state.mode === 'tv' ? digits : h('div.actions', null, h('button.btn.primary.big', { type: 'submit' }, T.enter))));
}
