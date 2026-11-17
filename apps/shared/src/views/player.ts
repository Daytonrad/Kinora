// Player: direct play or HLS (native on TVs/iOS, hls.js on Android), resume,
// progress every 10 s, audio/subtitle/quality choice, "why" panel, skip intro/credits,
// next-episode card, ±10 s, remote-friendly control bar, PiP, keep-awake.
import Hls from 'hls.js';
import { h, put, icon, toast, fmtTime } from '../dom';
import { T } from '../i18n';
import { state, api, route, choose, sheet, onCleanup, go, Item } from '../app';
import { getPlatform, isNative, KinoraNative, prefGet } from '../platform';
import { detectCaps, nativeHLS } from '../player/caps';
import { parseVTT, activeCues, Cue } from '../player/subs';
import { focusEl, focusFirst } from '../focus';

interface Stream { type: string; type_index: number; codec: string; lang: string; title: string; channels?: number; forced: boolean; text_based: boolean; external?: boolean; default?: boolean }
interface Segment { type: 'intro' | 'recap' | 'credits' | 'preview'; start: number; end: number; source?: string }
interface PlayResp {
  mode: string; url: string; session?: string; reasons: string[]; audio_idx: number; streams: Stream[]; duration: number;
  credits_start?: number | null; start_offset: number; encoder?: string; encoder_label?: string; error?: string;
  subtitles: { idx: number; lang: string; url: string; text_based: boolean; forced: boolean }[];
  segments?: Segment[]; source: Record<string, any>; item: { id: number; title: string; series_title?: string; season_num?: number; episode_num?: number; kind: string };
}

const LANGS: Record<string, string> = { hun: 'Magyar', eng: 'Angol', ger: 'Német', fre: 'Francia', spa: 'Spanyol', ita: 'Olasz', jpn: 'Japán', rus: 'Orosz', pol: 'Lengyel', cze: 'Cseh', slo: 'Szlovák', rum: 'Román', kor: 'Koreai', chi: 'Kínai' };
const langName = (l: string) => LANGS[l] || l || T.unknownLang;

route('play', async (parts, q) => {
  const id = +parts[1];
  const profile = state.me!.profile!;
  const platform = getPlatform();
  const caps = detectCaps(platform);
  const tv = state.mode === 'tv';
  state.playerActive = true;
  onCleanup(() => { state.playerActive = false; });

  const video = h('video', { playsinline: '', 'webkit-playsinline': '', preload: 'auto', 'x-webkit-airplay': 'allow' }) as HTMLVideoElement;
  const subLayer = h('div.subs', { 'aria-live': 'off' });
  const title = h('div.p-title');
  const timeCur = h('span.p-time');
  const timeTot = h('span.p-time');
  const played = h('i.played'), buffered = h('i.buffered'), knob = h('i.knob');
  const rail = h('div.rail', { role: 'slider', tabindex: '0', 'aria-label': 'Idővonal', 'aria-valuemin': '0' }, buffered, played, knob);
  const playBtn = h('button.p-btn.p-main', { 'aria-label': T.pause, onclick: () => togglePlay() }, icon('pause'));
  const skipBtn = h('button.btn.primary.big.skip', { onclick: () => doSkip() });
  const nextCard = h('div.next-card');
  const center = h('div.p-center');
  const chrome = h('div.p-chrome', null,
    h('div.p-top', null, h('button.p-btn', { 'aria-label': T.back, onclick: () => leave() }, icon('back')), title),
    h('div.p-bottom', null,
      h('div.p-timeline', null, timeCur, rail, timeTot),
      h('div.p-controls', null,
        h('button.p-btn', { 'aria-label': T.rewind, onclick: () => seekBy(-10) }, icon('r10')),
        playBtn,
        h('button.p-btn', { 'aria-label': T.forward, onclick: () => seekBy(10) }, icon('f10')),
        h('span.p-spacer'),
        h('button.p-btn', { 'aria-label': T.audio, onclick: () => audioMenu() }, icon('audio')),
        h('button.p-btn', { 'aria-label': T.subtitles, onclick: () => subMenu() }, icon('sub')),
        h('button.p-btn', { 'aria-label': T.quality, onclick: () => qualityMenu() }, icon('quality')),
        h('button.p-btn', { 'aria-label': T.why, onclick: () => whyPanel() }, icon('info')),
        (platform === 'android' || platform === 'ios') && h('button.p-btn', { 'aria-label': T.pip, onclick: () => pip() }, icon('pip')))));
  const stage = h('div.player' + (tv ? '.tv' : ''), null, video, subLayer, center, chrome, skipBtn, nextCard);
  put(document.getElementById('app')!, stage);
  skipBtn.style.display = 'none';
  nextCard.style.display = 'none';

  // ---------------------------------------------------------------- state
  let info: PlayResp | null = null;
  let offset = 0;             // item time of video.currentTime 0 (HLS sessions started at "start")
  let audioIdx: number | null = null;
  let subIdx: number | null = null;
  let burnIdx: number | null = null;
  let maxBitrate = profile.max_bitrate || 0;
  let force = '';
  let cues: Cue[] = [];
  let hls: any = null;
  let session = '';
  let lastReport = 0;
  let startedAt = Date.now();
  let retriedToken = false;
  let ended = false;
  let nextItem: Item | null = null;
  let nextTimer = 0;
  let nextDismissed = false;
  const skippedSeg: Record<number, boolean> = {};
  const skipMode = prefGet('skipIntro', 'manual');
  const pos = () => offset + (video.currentTime || 0);
  const duration = () => (info && info.duration) || (video.duration && isFinite(video.duration) ? offset + video.duration : 0);

  // ---------------------------------------------------------------- start / restart
  async function start(at: number) {
    put(center, h('div.spinner'));
    const body: any = { caps, start: at, max_bitrate: maxBitrate, prev_session: session || undefined };
    if (audioIdx !== null) body.audio_idx = audioIdx;
    if (burnIdx !== null) body.burn_sub_idx = burnIdx;
    if (force) body.force = force;
    let r: PlayResp;
    try { r = await api().post<PlayResp>(`/api/items/${id}/playback`, body); }
    catch (e: any) { return fail(e.message, e.body && e.body.reasons); }
    info = r;
    session = r.session || '';
    audioIdx = r.audio_idx;
    title.textContent = r.item.kind === 'episode' ? `${r.item.series_title} · ${T.ep(r.item.season_num, r.item.episode_num)} · ${r.item.title}` : r.item.title;
    rail.setAttribute('aria-valuemax', String(Math.round(r.duration || 0)));
    if (hls) { hls.destroy(); hls = null; }
    const url = api().abs(r.url);
    if (r.mode === 'direct') {
      offset = 0;
      video.src = url;
      const seekTo = at;
      const once = () => { video.removeEventListener('loadedmetadata', once); if (seekTo > 0) video.currentTime = seekTo; };
      video.addEventListener('loadedmetadata', once);
    } else {
      offset = r.start_offset || 0;
      if (!nativeHLS(platform) && Hls.isSupported()) {
        hls = new Hls({ maxBufferLength: 30, backBufferLength: 30, manifestLoadingMaxRetry: 8, manifestLoadingRetryDelay: 700, levelLoadingMaxRetry: 8, fragLoadingMaxRetry: 6 });
        hls.on(Hls.Events.ERROR, (_e: any, d: any) => { if (d && d.fatal) fail(T.playErr + ' (' + d.details + ')', r.reasons); });
        hls.loadSource(url);
        hls.attachMedia(video);
      } else {
        video.src = url;
      }
    }
    video.play().catch(() => { /* autoplay blocked until a key press */ });
    if (subIdx === null && burnIdx === null) chooseInitialSubtitle();
    loadSubtitle();
    if (r.item.kind === 'episode' && !nextItem) {
      api().get<{ next: Item | null }>(`/api/items/${id}/next`).then(n => { nextItem = n.next; }).catch(() => { /* none */ });
    }
  }

  function fail(msg: string, reasons?: string[]) {
    put(center, h('div.p-error', null, h('h2', null, T.playErr), h('p', null, msg), reasons && h('p.muted.small', null, reasons.join(' · ')),
      h('div.actions', null,
        h('button.btn.primary', { onclick: () => start(pos()), 'data-autofocus': '' }, T.retry),
        force !== 'transcode' && h('button.btn', { onclick: () => { force = 'transcode'; start(pos()); } }, T.tryTranscode),
        h('button.btn', { onclick: () => leave() }, T.back))));
    showChrome(true);
    focusFirst();
  }

  // ---------------------------------------------------------------- subtitles
  function chooseInitialSubtitle() {
    if (!info) return;
    const langs = (profile.sub_langs || '').split(',').filter(Boolean);
    const subs = info.streams.filter(s => s.type === 'subtitle' && s.text_based);
    if (profile.sub_mode === 'off' || !subs.length) return;
    const audio = info.streams.filter(s => s.type === 'audio' && s.type_index === audioIdx)[0];
    const forced = subs.filter(s => s.forced && langs.indexOf(s.lang) >= 0)[0];
    const foreign = !audio || !langs.length || audio.lang !== langs[0];
    if (profile.sub_mode === 'always' || foreign) {
      for (const l of langs) { const s = subs.filter(x => x.lang === l && !x.forced)[0]; if (s) { subIdx = s.type_index; return; } }
    }
    if (forced) subIdx = forced.type_index;
  }

  async function loadSubtitle() {
    cues = [];
    subLayer.textContent = '';
    if (subIdx === null || !info) return;
    const s = info.subtitles.filter(x => x.idx === subIdx)[0];
    if (!s) return;
    try {
      const r = await fetch(api().abs(s.url));
      if (r.ok) cues = parseVTT(await r.text());
    } catch { /* subtitle optional */ }
  }

  let lastSubKey = '';
  function renderSubs() {
    const act = cues.length ? activeCues(cues, pos()) : [];
    const key = act.map(c => c.start).join(',');
    if (key === lastSubKey) return;
    lastSubKey = key;
    put(subLayer, act.map(c => h('span.cue', null, c.text.split('\n').map((l, i) => [i ? h('br') : null, l]))));
  }

  // ---------------------------------------------------------------- chrome visibility
  let hideTimer = 0;
  function showChrome(sticky = false) {
    stage.classList.add('show');
    window.clearTimeout(hideTimer);
    if (!sticky && !video.paused) hideTimer = window.setTimeout(() => {
      if (document.querySelector('.layer')) return showChrome();
      stage.classList.remove('show');
      if (tv && document.activeElement && chrome.contains(document.activeElement)) (document.activeElement as HTMLElement).blur();
    }, 4000);
  }
  stage.addEventListener('mousemove', () => showChrome());
  stage.addEventListener('touchstart', (e) => {
    const t = e.target as HTMLElement;
    if (t === video || t === subLayer || t === stage) { if (stage.classList.contains('show')) { stage.classList.remove('show'); } else showChrome(); }
  }, { passive: true } as any);

  // ---------------------------------------------------------------- transport
  function togglePlay() { if (video.paused) video.play().catch(() => { /* */ }); else video.pause(); showChrome(); }
  function seekTo(t: number) {
    const d = duration();
    t = Math.max(0, d ? Math.min(t, d - 2) : t);
    if (info && info.mode !== 'direct') {
      const rel = t - offset;
      const sk = video.seekable;
      if (rel >= 0 && sk.length && rel <= sk.end(sk.length - 1)) { video.currentTime = rel; return; }
      report();
      start(t); // outside the transcoded range: new session from t
      return;
    }
    video.currentTime = t;
  }
  function seekBy(s: number) { seekTo(pos() + s); showChrome(); }

  // ---------------------------------------------------------------- progress
  function report(endedNow = false) {
    if (!info) return;
    const p = pos();
    if (p < 1 && !endedNow) return;
    api().post(`/api/items/${id}/progress`, { position: p, ended: endedNow }).catch(() => { /* offline: next report wins */ });
  }

  // ---------------------------------------------------------------- skip intro / credits
  function currentSegment(): Segment | null {
    if (!info || !info.segments) return null;
    const p = pos();
    for (const s of info.segments) if ((s.type === 'intro' || s.type === 'recap' || s.type === 'credits') && p >= s.start && p < s.end - 1) return s;
    return null;
  }
  let shownSeg: Segment | null = null;
  function updateSkip() {
    const seg = skipMode === 'off' ? null : currentSegment();
    if (seg && seg.type === 'credits' && nextItem) { skipBtn.style.display = 'none'; shownSeg = null; return; } // next-episode card takes over
    if (seg && skipMode === 'auto' && !skippedSeg[seg.start]) {
      skippedSeg[seg.start] = true;
      seekTo(seg.end);
      toast(T.skipped);
      return;
    }
    if (seg !== shownSeg) {
      shownSeg = seg;
      if (seg) {
        skipBtn.textContent = seg.type === 'credits' ? T.skipCredits : seg.type === 'recap' ? T.skipRecap : T.skipIntro;
        skipBtn.style.display = '';
        if (tv) focusEl(skipBtn); // on TV the skip button takes focus automatically
      } else {
        skipBtn.style.display = 'none';
      }
    }
  }
  function doSkip() { const s = currentSegment(); if (s) { skippedSeg[s.start] = true; seekTo(s.end); skipBtn.style.display = 'none'; shownSeg = null; } }

  // ---------------------------------------------------------------- next episode
  function creditsStart(): number {
    if (!info) return Infinity;
    const seg = info.segments && info.segments.filter(s => s.type === 'credits')[0];
    if (seg) return seg.start;
    if (info.credits_start) return info.credits_start;
    return duration() ? duration() - 30 : Infinity;
  }
  function updateNext() {
    if (!nextItem || nextDismissed) return;
    const show = pos() >= creditsStart();
    if (show && nextCard.style.display === 'none') {
      let n = 10;
      const autoplay = profile.autoplay_next;
      const label = h('span.next-in', null, autoplay ? T.nextIn(n) : '');
      const nowBtn = h('button.btn.primary', { onclick: () => playNext() }, icon('play'), T.playNow);
      put(nextCard, h('div.next-meta', null, h('span.reason', null, T.nextEp), h('strong', null, nextItem.title),
        h('span.muted', null, T.ep(nextItem.season_num, nextItem.episode_num)), label),
        h('div.actions', null, nowBtn, h('button.btn', { onclick: () => { nextDismissed = true; nextCard.style.display = 'none'; window.clearInterval(nextTimer); } }, T.cancel)));
      nextCard.style.display = '';
      if (tv) focusEl(nowBtn);
      if (autoplay) nextTimer = window.setInterval(() => { n--; label.textContent = T.nextIn(n); if (n <= 0) playNext(); }, 1000);
    } else if (!show && nextCard.style.display !== 'none') {
      nextCard.style.display = 'none';
      window.clearInterval(nextTimer);
    }
  }
  function playNext() {
    window.clearInterval(nextTimer);
    report(true);
    if (nextItem) go(`#/play/${nextItem.id}`, true);
  }

  // ---------------------------------------------------------------- menus
  async function audioMenu() {
    if (!info) return;
    const list = info.streams.filter(s => s.type === 'audio');
    const v = await choose<number>(T.audio, list.map(a => [a.type_index, `${langName(a.lang)} – ${a.codec}${a.channels ? ' ' + a.channels + 'ch' : ''}${a.title ? ' (' + a.title + ')' : ''}`] as [number, string]), audioIdx === null ? undefined : audioIdx);
    if (v !== null && v !== audioIdx) { audioIdx = v; start(pos()); }
  }
  async function subMenu() {
    if (!info) return;
    const list = info.streams.filter(s => s.type === 'subtitle');
    const cur = burnIdx !== null ? burnIdx : subIdx === null ? -1 : subIdx;
    const v = await choose<number>(T.subtitles, ([[-1, T.subOff]] as [number, string][]).concat(list.map(s =>
      [s.type_index, `${langName(s.lang)}${s.forced ? ' (kényszerített)' : ''}${s.text_based ? '' : ' (beégetve)'}`] as [number, string])), cur);
    if (v === null || v === cur) return;
    const s = list.filter(x => x.type_index === v)[0];
    if (v === -1) { subIdx = null; if (burnIdx !== null) { burnIdx = null; start(pos()); } else loadSubtitle(); return; }
    if (s && !s.text_based) { burnIdx = v; subIdx = null; start(pos()); return; }
    subIdx = v;
    if (burnIdx !== null) { burnIdx = null; start(pos()); } else loadSubtitle();
  }
  async function qualityMenu() {
    const v = await choose<number>(T.quality, T.bitrates.map(([b, l]) => [b, b ? l : T.qAuto] as [number, string]), maxBitrate);
    if (v === null || v === maxBitrate) return;
    maxBitrate = v;
    force = v ? 'transcode' : '';
    start(pos());
  }
  function whyPanel() {
    if (!info) return;
    const src = info.source || {};
    sheet(T.why, h('div.why', null,
      h('h3', null, T.whyTitle[info.mode] || info.mode),
      h('ul', null, (info.reasons || []).map(r => h('li', null, r))),
      info.encoder && h('p', null, T.encoder + ': ' + (info.encoder_label || info.encoder)),
      h('p.muted.small', null, [src.container, src.video_codec, src.audio_codec, src.width && src.height ? src.width + '×' + src.height : '', src.hdr].filter(Boolean).join(' · ')),
      h('p.muted.small', null, 'Kliens: ' + caps.video.join(', ') + ' / ' + caps.audio.join(', ') + ' / ' + caps.containers.join(', '))));
  }
  function pip() {
    const v = video as any;
    if (getPlatform() === 'ios' && v.webkitSetPresentationMode) return v.webkitSetPresentationMode('picture-in-picture');
    if (isNative) KinoraNative.enterPip().catch(() => { /* not supported */ });
    else if (v.requestPictureInPicture) v.requestPictureInPicture().catch(() => { /* */ });
  }

  // ---------------------------------------------------------------- events
  const tick = () => {
    const p = pos(), d = duration();
    timeCur.textContent = fmtTime(p);
    timeTot.textContent = d ? '-' + fmtTime(d - p) : '';
    const pct = d ? Math.min(100, p / d * 100) : 0;
    played.style.width = pct + '%';
    knob.style.left = pct + '%';
    rail.setAttribute('aria-valuenow', String(Math.round(p)));
    try {
      const b = video.buffered;
      if (b.length && d) buffered.style.width = Math.min(100, (offset + b.end(b.length - 1)) / d * 100) + '%';
    } catch { /* */ }
    renderSubs();
    updateSkip();
    updateNext();
    if (Date.now() - lastReport > 10000 && !video.paused) { lastReport = Date.now(); report(); }
  };
  video.addEventListener('timeupdate', tick);
  video.addEventListener('playing', () => { put(center); put(playBtn, icon('pause')); playBtn.setAttribute('aria-label', T.pause); showChrome(); });
  video.addEventListener('pause', () => { put(playBtn, icon('play')); playBtn.setAttribute('aria-label', T.play); showChrome(true); report(); });
  video.addEventListener('waiting', () => put(center, h('div.spinner')));
  video.addEventListener('ended', () => {
    if (ended) return;
    ended = true;
    report(true);
    if (nextItem && profile.autoplay_next && !nextDismissed) playNext(); else leave();
  });
  video.addEventListener('error', () => {
    const err = video.error;
    if (hls) return;
    // a stream token lives 6 h: after a long pause get a fresh one instead of failing
    if (Date.now() - startedAt > 5 * 3600 * 1000 && !retriedToken) {
      retriedToken = true;
      startedAt = Date.now();
      start(pos());
      return;
    }
    fail(T.playErr + (err ? ` (kód: ${err.code})` : ''), info ? info.reasons : undefined);
  });
  rail.addEventListener('click', (e: MouseEvent) => {
    const r = rail.getBoundingClientRect();
    seekTo((e.clientX - r.left) / r.width * duration());
  });

  // remote / keyboard
  state.keyHandler = (a: string) => {
    if (document.querySelector('.layer')) return false; // menus use normal navigation
    const chromeShown = stage.classList.contains('show');
    const inChrome = document.activeElement && chrome.contains(document.activeElement);
    switch (a) {
      case 'playpause': togglePlay(); return true;
      case 'play': video.play().catch(() => { /* */ }); showChrome(); return true;
      case 'pause': video.pause(); return true;
      case 'stop': leave(); return true;
      case 'ff': case 'next': seekBy(30); return true;
      case 'rw': case 'prev': seekBy(-10); return true;
      case 'back':
        if (chromeShown && inChrome) { stage.classList.remove('show'); (document.activeElement as HTMLElement).blur(); return true; }
        leave(); return true;
      case 'left': case 'right':
        if (document.activeElement === rail || (!chromeShown && !skipVisible() && !nextVisible())) { seekBy(a === 'left' ? -10 : 10); return true; }
        showChrome(); return false;
      case 'ok':
        if (!chromeShown && !skipVisible() && !nextVisible()) { togglePlay(); focusEl(playBtn); return true; }
        showChrome(); return false;
      case 'up': case 'down':
        if (!chromeShown) { showChrome(); focusEl(playBtn); return true; }
        showChrome(); return false;
    }
    return false;
  };
  const skipVisible = () => skipBtn.style.display !== 'none';
  const nextVisible = () => nextCard.style.display !== 'none';

  // keep the screen on; optionally keep audio in the background (phones)
  const w = window as any;
  try { if (getPlatform() === 'tizen' && w.webapis && w.webapis.appcommon) w.webapis.appcommon.setScreenSaver(w.webapis.appcommon.AppCommonScreenSaverState.SCREEN_SAVER_OFF); } catch { /* */ }
  if (isNative) KinoraNative.keepAwake({ on: true }).catch(() => { /* */ });
  const onVis = () => { if (document.hidden && prefGet('bgAudio', '0') !== '1' && !(document as any).pictureInPictureElement) video.pause(); };
  document.addEventListener('visibilitychange', onVis);
  const onUnload = () => report();
  window.addEventListener('pagehide', onUnload);

  let left = false;
  function leave() {
    if (left) return;
    left = true;
    history.back();
  }
  onCleanup(() => {
    report(ended);
    window.clearInterval(nextTimer);
    window.clearTimeout(hideTimer);
    document.removeEventListener('visibilitychange', onVis);
    window.removeEventListener('pagehide', onUnload);
    if (hls) hls.destroy();
    video.pause();
    video.removeAttribute('src');
    try { video.load(); } catch { /* */ }
    if (session) api().del('/api/hls/' + session).catch(() => { /* */ });
    if (isNative) KinoraNative.keepAwake({ on: false }).catch(() => { /* */ });
    try { if (getPlatform() === 'tizen' && w.webapis && w.webapis.appcommon) w.webapis.appcommon.setScreenSaver(w.webapis.appcommon.AppCommonScreenSaverState.SCREEN_SAVER_ON); } catch { /* */ }
  });

  const startAt = +(q.get('start') || 0);
  await start(startAt);
  showChrome();
  if (tv) focusEl(playBtn);
});
