// Settings: playback preferences (profile), app preferences, devices,
// "connect a device" (approve a TV's Quick Connect code), profile/server switch, logout.
import { h, put, icon, toast } from '../dom';
import { T } from '../i18n';
import { state, api, route, shell, choose, go, confirmBox, loadMe, persistServer, render, sheet, Profile } from '../app';
import { prefGet, prefSet, APP_VERSION, getPlatform } from '../platform';
import { setCurrentServer } from '../api';

function row(label: string, value: string, onClick: () => void): HTMLElement {
  return h('button.set-row', { onclick: onClick }, h('span.set-label', null, label), h('span.set-value', null, value));
}

function section(title: string, ...kids: any[]): HTMLElement {
  return h('section.set-section', null, h('h2.sub', null, title), kids);
}

route('settings', async () => {
  const main = shell('settings');
  const me = await loadMe();
  const p = me.profile as Profile;
  const canEdit = !p.is_kids && !me.profile_locked;

  const editProfile = async (patch: Record<string, any>) => {
    await api().patch('/api/profiles/' + p.id, patch);
    await loadMe();
    toast(T.saved);
    render();
  };
  const askText = (label: string, value: string, hint: string): Promise<string | null> => new Promise(resolve => {
    const inp = h('input', { type: 'text', value, autocapitalize: 'off', 'data-autofocus': '' }) as HTMLInputElement;
    let done = false;
    const l = sheet(label, h('form', { onsubmit: (e: Event) => { e.preventDefault(); done = true; l.close(); resolve(inp.value); } },
      h('label.field', null, h('span', null, label), inp, h('span.hint', null, hint)),
      h('div.actions', null, h('button.btn.primary.big', { type: 'submit' }, T.save))), { onClose: () => { if (!done) resolve(null); } });
  });

  const devicesBox = h('div');
  const loadDevices = async () => {
    const r = await api().get<{ devices: any[] }>('/api/devices');
    put(devicesBox, r.devices.map(d => h('div.dev-row', null,
      icon(d.platform === 'tizen' || d.platform === 'webos' || d.platform === 'androidtv' ? 'tv' : d.platform === 'web' ? 'server' : 'phone'),
      h('div.dev-text', null, h('strong', null, d.name || T.platforms[d.platform] || d.platform, d.current ? ' · ' + T.thisDevice : ''),
        h('span.muted.small', null, [T.platforms[d.platform] || d.platform, d.app_version && 'v' + d.app_version, d.ip, T.ago(Date.now() / 1000 - d.last_seen)].filter(Boolean).join(' · '))),
      !d.current && canEdit && h('button.btn.danger', { onclick: async () => { await api().del('/api/devices/' + d.id); loadDevices(); } }, T.signOut))),
      canEdit && r.devices.length > 1 && h('button.btn', { onclick: async () => { await api().del('/api/devices'); loadDevices(); } }, T.signOutOthers));
  };

  const s = state.api!.server;
  put(main, h('div.page.settings', null,
    h('div.page-head', null, h('h1', null, T.settings)),
    section(T.account,
      h('div.who-line', null, h('span.face.small', { style: { background: p.color } }, p.name.slice(0, 1).toUpperCase()),
        h('span', null, `${p.name} · ${me.user.username}`)),
      !me.profile_locked && row(T.switchProfile, '', () => go('#/profiles')),
      canEdit && row(T.connectDevice, '', () => go('#/connect-device'))),
    canEdit && section(T.playback,
      row(T.subMode, T.subModes[p.sub_mode] || p.sub_mode, async () => {
        const v = await choose(T.subMode, Object.keys(T.subModes).map(k => [k, T.subModes[k]] as [string, string]), p.sub_mode);
        if (v) editProfile({ sub_mode: v });
      }),
      row(T.subLangs, p.sub_langs, async () => { const v = await askText(T.subLangs, p.sub_langs, T.langHint); if (v !== null) editProfile({ sub_langs: v }); }),
      row(T.audioLangs, p.audio_langs, async () => { const v = await askText(T.audioLangs, p.audio_langs, T.langHint); if (v !== null) editProfile({ audio_langs: v }); }),
      row(T.maxBitrate, (T.bitrates.filter(b => b[0] === p.max_bitrate)[0] || [0, p.max_bitrate + ' kbps'])[1], async () => {
        const v = await choose<number>(T.maxBitrate, T.bitrates, p.max_bitrate);
        if (v !== null) editProfile({ max_bitrate: v });
      }),
      row(T.autoplayNext, p.autoplay_next ? T.on : T.off, () => editProfile({ autoplay_next: !p.autoplay_next }))),
    section(T.appName,
      row(T.skipIntroMode, T.skipModes[p.skip_intro || prefGet('skipIntro', 'manual')], async () => {
        const cur = p.skip_intro || prefGet('skipIntro', 'manual');
        const v = await choose(T.skipIntroMode, Object.keys(T.skipModes).map(k => [k, T.skipModes[k]] as [string, string]), cur);
        if (!v) return;
        prefSet('skipIntro', v);
        if (canEdit) editProfile({ skip_intro: v }); else render();
      }),
      row(T.uiMode, T.uiModes[prefGet('uiMode', 'auto')], async () => {
        const v = await choose(T.uiMode, Object.keys(T.uiModes).map(k => [k, T.uiModes[k]] as [string, string]), prefGet('uiMode', 'auto'));
        if (v) { prefSet('uiMode', v); location.reload(); }
      }),
      (getPlatform() === 'android' || getPlatform() === 'ios') && row(T.bgAudio, prefGet('bgAudio', '0') === '1' ? T.on : T.off, () => { prefSet('bgAudio', prefGet('bgAudio', '0') === '1' ? '0' : '1'); render(); })),
    section(T.devices, devicesBox),
    section(T.server,
      h('p.muted', null, `${s.name} · ${s.url} · ${T.version} ${me.version}`),
      row(T.switchServer, '', async () => { await setCurrentServer(null); go('#/servers'); }),
      row(T.logout, '', async () => {
        if (!(await confirmBox(T.logout + '?', T.logout, T.cancel))) return;
        try { await api().post('/api/logout'); } catch { /* already gone */ }
        s.token = undefined;
        await persistServer(s);
        state.me = null;
        go('#/login', true);
      })),
    h('p.muted.small.ver', null, `${T.appName} ${APP_VERSION} · ${T.platforms[getPlatform()] || getPlatform()}`)));
  loadDevices();
});

// ------------------------------------------------------------------ approve another device's code (phone → TV)

route('connect-device', async (_p, q) => {
  const main = shell('settings');
  const me = await loadMe();
  const box = h('div.panel-auth');
  put(main, h('div.page', null, box));
  const inp = h('input.pin', { type: 'text', inputmode: 'numeric', maxlength: 7, autocomplete: 'off', value: q.get('code') || '', 'aria-label': T.qcEnterCode, 'data-autofocus': '' }) as HTMLInputElement;
  const err = h('p.err', { role: 'alert' });
  const ask = () => put(box, h('h1', null, T.connectDevice), h('p.lede', null, T.qcEnterCode),
    h('form', { onsubmit: (e: Event) => { e.preventDefault(); lookup(); } }, inp, err, h('div.actions', null, h('button.btn.primary.big', { type: 'submit' }, T.next))));
  const lookup = async () => {
    err.textContent = '';
    let info: any;
    try { info = await api().post('/api/quickconnect/lookup', { code: inp.value }); } catch (e: any) { err.textContent = e.message; return; }
    let chosen = me.profile ? me.profile.id : 0;
    const opts = h('div.choices');
    const draw = () => put(opts, ([[0, T.qcAnyProfile]] as [number, string][]).concat(me.profiles.map(p => [p.id, p.name + (p.is_kids ? ' (' + T.kids + ')' : '')] as [number, string]))
      .map(([id, label]) => h('button.choice' + (id === chosen ? '.on' : ''), { onclick: () => { chosen = id; draw(); } }, id === chosen && icon('check'), h('span', null, label))));
    draw();
    put(box, h('h1', null, T.connectDevice), h('p.lede', null, T.qcWants(info.device_name)), h('div.code-big', null, info.code),
      h('h2.sub', null, T.qcProfile), opts,
      h('div.actions', null,
        h('button.btn.primary.big', { onclick: async () => {
          await api().post('/api/quickconnect/approve', { code: info.code, profile_id: chosen });
          put(box, h('h1', null, T.connectDevice), h('p.lede', null, T.qcApproved), h('a.btn', { href: '#/home' }, T.home));
        } }, T.approve),
        h('button.btn.big', { onclick: async () => { await api().post('/api/quickconnect/approve', { code: info.code, action: 'deny' }); ask(); } }, T.deny)));
  };
  ask();
});
