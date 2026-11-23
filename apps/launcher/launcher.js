/* Kinora launcher – the start screen of the packaged apps (Samsung Tizen, LG webOS, Titan OS, Android phone + TV).
   It only finds the server: saved servers, an address typed in, or a search of the home network. Then it opens the
   server's own web client in app mode (?app=<platform>), so the apps always show what the server's web client can do.
   Plain ES5 without a build step: it also runs on 2018 TV engines (Chromium 53/56), where it hands over to the
   older bundled app (legacy/), because the web client needs Chromium 63+. */
(function () {
  'use strict';
  var CFG = window.KINORA_LAUNCHER || { platform: 'web', version: '' };
  var native = window.KinoraAndroid || null; // Android: MainActivity's JavaScript interface
  var offline = window.KinoraOffline || null; // Android: offline downloads (KinoraOffline.java)
  var platform = CFG.platform;
  if (platform === 'android' && native && native.isTv()) platform = 'androidtv';
  var TV = { tizen: true, webos: true, titan: true, androidtv: true }[platform] || /[?&]tv=1/.test(location.search);
  if (TV) document.documentElement.className += ' tv';

  // ------------------------------------------------------------------ too old for the web client
  if (CFG.legacy && !(window.Intl && Intl.PluralRules)) { location.replace('legacy/index.html'); return; }

  // ------------------------------------------------------------------ strings
  var STR = {
    hu: { title: 'Szerverek', add: 'Szerver hozzáadása', search: 'Keresés a hálózaton', searching: 'Keresés a hálózaton…', none: 'Nem találtam Kinora szervert ezen a hálózaton. Írd be a címét.',
      address: 'A szerver címe', addrHint: 'Pl. 192.168.1.10, nas.local vagy https://kinora.pelda.hu. Port nélkül a 8096-ot próbálom.', connect: 'Csatlakozás',
      connecting: 'Csatlakozás: {0}…', cancel: 'Mégse', remove: 'Eltávolítás', back: 'Vissza', unreachable: 'Nem érem el: {0}.',
      privateAddr: 'Ez otthoni hálózati cím: csak ugyanarról a hálózatról érhető el.', notKinora: 'Ezen a címen nem Kinora szerver válaszol: {0}.',
      setup: 'Ezt a szervert még be kell állítani: nyisd meg a címét egy böngészőben.', found: 'Talált szerverek', saved: 'Mentett szerverek', exitHint: 'Kilépés: Vissza gomb',
      offline: 'Letöltések offline', offlineRow: '{0} – {1}', offlineHint: 'Lejátszható szerver nélkül is. A megnézett részeket a következő csatlakozáskor küldöm el.',
      del: 'Törlés', noDownloads: 'Nincs offline lejátszható letöltés.', playFailed: 'Ezt a fájlt nem sikerült lejátszani.' },
    en: { title: 'Servers', add: 'Add a server', search: 'Search the network', searching: 'Searching the network…', none: 'No Kinora server found on this network. Type its address.',
      address: 'Server address', addrHint: 'E.g. 192.168.1.10, nas.local or https://kinora.example.com. Without a port 8096 is tried.', connect: 'Connect',
      connecting: 'Connecting to {0}…', cancel: 'Cancel', remove: 'Remove', back: 'Back', unreachable: 'Cannot reach {0}.',
      privateAddr: 'This is a home network address: it only works from the same network.', notKinora: 'Something answers at {0}, but it is not a Kinora server.',
      setup: 'This server is not set up yet: open its address in a browser.', found: 'Found on the network', saved: 'Saved servers', exitHint: 'Exit: Back button',
      offline: 'Offline downloads', offlineRow: '{0} – {1}', offlineHint: 'These play without the server. What you watch is sent the next time you connect.',
      del: 'Delete', noDownloads: 'No download to play offline.', playFailed: 'This file could not be played.' },
    de: { title: 'Server', add: 'Server hinzufügen', search: 'Im Netzwerk suchen', searching: 'Suche im Netzwerk…', none: 'Kein Kinora-Server in diesem Netzwerk gefunden. Gib die Adresse ein.',
      address: 'Serveradresse', addrHint: 'Z. B. 192.168.1.10, nas.local oder https://kinora.example.de. Ohne Port wird 8096 versucht.', connect: 'Verbinden',
      connecting: 'Verbinde mit {0}…', cancel: 'Abbrechen', remove: 'Entfernen', back: 'Zurück', unreachable: '{0} ist nicht erreichbar.',
      privateAddr: 'Das ist eine Heimnetz-Adresse: Sie funktioniert nur im selben Netzwerk.', notKinora: 'Unter {0} antwortet kein Kinora-Server.',
      setup: 'Dieser Server ist noch nicht eingerichtet: Öffne seine Adresse in einem Browser.', found: 'Im Netzwerk gefunden', saved: 'Gespeicherte Server', exitHint: 'Beenden: Zurück-Taste',
      offline: 'Offline-Downloads', offlineRow: '{0} – {1}', offlineHint: 'Diese laufen auch ohne Server. Was du ansiehst, wird bei der nächsten Verbindung übertragen.',
      del: 'Löschen', noDownloads: 'Kein Download, der offline abgespielt werden kann.', playFailed: 'Diese Datei ließ sich nicht abspielen.' },
  };
  var lang = 'en';
  var langs = navigator.languages || [navigator.language || 'en'];
  for (var li = 0; li < langs.length; li++) { var c = String(langs[li]).slice(0, 2).toLowerCase(); if (STR[c]) { lang = c; break; } }
  document.documentElement.lang = lang;
  function t(k, a, b) { var s = STR[lang][k] || STR.en[k] || k; if (a !== undefined) s = s.replace('{0}', a); return b === undefined ? s : s.replace('{1}', b); }

  // ------------------------------------------------------------------ storage
  var KEY = 'kinora.launcher.servers';
  function load() { try { return JSON.parse(localStorage.getItem(KEY) || '[]') || []; } catch (e) { return []; } }
  function store(list) { try { localStorage.setItem(KEY, JSON.stringify(list.slice(0, 12))); } catch (e) { /* storage blocked */ } }
  function session(k, v) { try { if (v === undefined) return sessionStorage.getItem(k); sessionStorage.setItem(k, v); } catch (e) { return null; } return null; }

  // ------------------------------------------------------------------ addresses (same rules as apps/shared/src/address.ts)
  function isLocalHost(host) {
    var h = host.replace(/^\[|\]$/g, '').toLowerCase();
    return /^\d{1,3}(\.\d{1,3}){3}$/.test(h) || h.indexOf(':') >= 0 || h === 'localhost' || /\.(local|lan|home|internal)$/.test(h) || h.indexOf('.') < 0;
  }
  function candidateURLs(input) {
    var s = (input || '').replace(/\s+/g, '').replace(/\/+$/, '');
    if (!s) return [];
    var m = /^(https?):\/\/(.+)$/i.exec(s);
    var scheme = m ? m[1].toLowerCase() : '', rest = m ? m[2] : s;
    var hostPort = rest.split('/')[0], path = rest.slice(hostPort.length);
    var hasPort = /^\[.*\]:\d+$/.test(hostPort) || (/:\d+$/.test(hostPort) && hostPort.split(':').length === 2);
    if (scheme) {
      if (hasPort) return [scheme + '://' + rest];
      return scheme === 'https' ? ['https://' + rest, 'https://' + hostPort + ':8096' + path] : ['http://' + hostPort + ':8096' + path, 'http://' + rest];
    }
    var host = hasPort ? hostPort.replace(/:\d+$/, '') : hostPort;
    if (hasPort) return isLocalHost(host) ? ['http://' + hostPort + path, 'https://' + hostPort + path] : ['https://' + hostPort + path, 'http://' + hostPort + path];
    return isLocalHost(host) ? ['http://' + hostPort + ':8096' + path, 'http://' + hostPort + path, 'https://' + hostPort + path]
      : ['https://' + hostPort + path, 'http://' + hostPort + ':8096' + path, 'http://' + hostPort + path];
  }
  function merge() {
    var out = [];
    for (var i = 0; i < arguments.length; i++) {
      var l = arguments[i];
      l = Array.isArray(l) ? l : [l];
      for (var j = 0; j < l.length; j++) { var v = String(l[j] || '').replace(/\/+$/, ''); if (v && out.indexOf(v) < 0) out.push(v); }
    }
    return out.slice(0, 8);
  }
  function hostOf(u) { return u.replace(/^https?:\/\//, '').split(/[/]/)[0]; }

  // An https page may not read an http server (mixed content): Titan OS hosts this launcher on https,
  // so there an http address is opened without checking it first.
  var cannotProbe = function (url) { return location.protocol === 'https:' && /^http:/.test(url); };

  function probe(base, ms) {
    return new Promise(function (resolve, reject) {
      var done = false;
      var timer = setTimeout(function () { if (!done) { done = true; reject(new Error('timeout')); } }, ms || 4000);
      fetch(base + '/api/status', { cache: 'no-store' }).then(function (r) {
        if (!r.ok) throw new Error('status ' + r.status);
        return r.json().then(function (j) {
          if (!j || (j.product !== 'kinora' && typeof j.setup_needed !== 'boolean')) throw new Error('not kinora');
          var i = r.url ? r.url.indexOf('/api/status') : -1; // a redirect (http → https) moved the server
          return { url: i > 0 ? r.url.slice(0, i) : base, status: j };
        });
      }).then(function (v) { if (!done) { done = true; clearTimeout(timer); resolve(v); } },
        function (e) { if (!done) { done = true; clearTimeout(timer); reject(e); } });
    });
  }

  // ------------------------------------------------------------------ opening a server
  var opening = 0; // generation: Cancel or Back abandons a running attempt
  function launcherURL() { return /^https?:$/.test(location.protocol) ? location.href.split('#')[0] : ''; }
  function model() {
    try { if (native) return native.model(); } catch (e) { /* old app */ }
    try { if (window.webapis && webapis.productinfo) return webapis.productinfo.getRealModel(); } catch (e) { /* not a Samsung TV */ }
    return '';
  }
  function go(url) {
    session('kinora.launched', '1');
    var q = '?app=' + platform + '&ver=' + encodeURIComponent(CFG.version || '') + '&launcher=' + encodeURIComponent(launcherURL()) + '&model=' + encodeURIComponent(model());
    location.href = url + '/' + q + '#/';
  }
  function remember(s) {
    var list = load().filter(function (x) { return x.id !== s.id; });
    list.unshift(s);
    store(list);
    // the app's offline downloads accept the server's pages only on the addresses opened from here
    if (offline) { try { offline.registerServer(String(s.id), s.name || '', JSON.stringify(merge(s.url, s.urls))); } catch (e) { /* old app */ } }
  }
  /** Tries every address of a saved server (home first, then outside), then opens the first that answers. */
  function open(s) {
    var gen = ++opening;
    showBusy(t('connecting', s.name || hostOf(s.url)));
    var urls = merge(s.url, s.urls);
    var i = 0;
    (function next() {
      if (gen !== opening) return;
      if (i >= urls.length) { showList(t('unreachable', s.name || hostOf(s.url)) + (isLocalHost(hostOf(s.url).replace(/:\d+$/, '')) ? ' ' + t('privateAddr') : '')); return; }
      var u = urls[i++];
      if (cannotProbe(u)) { s.url = u; s.lastUsed = Date.now(); remember(s); go(u); return; }
      probe(u, i === 1 ? 5000 : 4000).then(function (r) {
        if (gen !== opening) return;
        if (r.status.server_id && s.id && s.id.indexOf('http') !== 0 && r.status.server_id !== s.id) return next(); // another server there now
        s.url = r.url; s.urls = merge(r.url, s.urls, r.status.public_url); s.name = r.status.server_name || s.name; s.lastUsed = Date.now();
        remember(s);
        go(r.url);
      }, next);
    })();
  }
  /** A typed address: find which of its candidate URLs is a Kinora server. */
  function connect(input) {
    var cands = candidateURLs(input);
    if (!cands.length) return;
    var gen = ++opening;
    showBusy(t('connecting', input));
    if (cands.every(cannotProbe)) { open({ id: cands[0], url: cands[0], urls: cands, name: hostOf(cands[0]) }); return; }
    var i = 0, sawSomething = false;
    (function next() {
      if (gen !== opening) return;
      if (i >= cands.length) {
        var host = hostOf(cands[0]).replace(/:\d+$/, '');
        showAdd(input, sawSomething ? t('notKinora', input) : t('unreachable', input) + (isLocalHost(host) ? ' ' + t('privateAddr') : ''));
        return;
      }
      var u = cands[i++];
      if (cannotProbe(u)) return next();
      probe(u).then(function (r) {
        if (gen !== opening) return;
        if (r.status.setup_needed) { showAdd(input, t('setup')); return; }
        var s = { id: r.status.server_id || r.url, url: r.url, urls: merge(r.url, r.status.public_url), name: r.status.server_name || hostOf(r.url), lastUsed: Date.now() };
        remember(s);
        go(r.url);
      }, function (e) { if (e && e.message === 'not kinora') sawSomething = true; next(); });
    })();
  }

  // ------------------------------------------------------------------ searching the home network
  // Android: UDP broadcast through the native plugin. TVs cannot send UDP: they ask for their own IP and
  // probe <subnet>.1–254:8096 over HTTP. Titan OS (https page) can do neither.
  function nativeDiscover() {
    var C = window.Capacitor;
    var p = C && C.Plugins && C.Plugins.KinoraNative;
    if (!p) return Promise.resolve([]);
    return p.discover({ timeoutMs: 2500, port: 7359 }).then(function (r) { return r.servers || []; }, function () { return []; });
  }
  function ownIP() {
    return new Promise(function (resolve) {
      setTimeout(function () { resolve(null); }, 1500);
      try { if (window.webapis && webapis.network && webapis.network.getIp) { var ip = webapis.network.getIp(); if (ip) return resolve(ip); } } catch (e) { /* no permission */ }
      try {
        if (window.webOS && webOS.service) {
          webOS.service.request('luna://com.palm.connectionmanager', { method: 'getStatus', parameters: {},
            onSuccess: function (r) { resolve((r.wired && r.wired.ipAddress) || (r.wifi && r.wifi.ipAddress) || null); }, onFailure: function () { resolve(null); } });
        }
      } catch (e) { /* not webOS */ }
    });
  }
  function scan(prefix, onFound) {
    var hosts = [];
    for (var i = 1; i < 255; i++) hosts.push(prefix + i);
    var nextIdx = 0;
    function worker() {
      if (nextIdx >= hosts.length) return Promise.resolve();
      var url = 'http://' + hosts[nextIdx++] + ':8096';
      return probe(url, 900).then(function (r) { onFound({ name: r.status.server_name, address: r.url, id: r.status.server_id || r.url, public_url: r.status.public_url }); }, function () {}).then(worker);
    }
    var pool = [];
    for (var k = 0; k < 32; k++) pool.push(worker());
    return Promise.all(pool);
  }
  var canSearch = !cannotProbe('http://x') && (!!native || platform === 'tizen' || platform === 'webos');
  function search(onFound) {
    var seen = {};
    var emit = function (s) { var k = s.id || s.address; if (!seen[k]) { seen[k] = true; onFound(s); } };
    return nativeDiscover().then(function (list) {
      list.forEach(emit);
      if (list.length) return;
      return ownIP().then(function (ip) {
        if (!ip || !/^\d+\.\d+\.\d+\.\d+$/.test(ip)) return;
        return scan(ip.replace(/\d+$/, ''), emit);
      });
    });
  }

  // ------------------------------------------------------------------ screens
  var app = document.getElementById('app');
  function el(tag, cls, text) {
    var e = document.createElement(tag);
    if (cls) e.className = cls;
    if (text != null) e.textContent = text;
    return e;
  }
  function button(cls, text, onclick) { var b = el('button', 'btn ' + (cls || ''), text); b.type = 'button'; b.onclick = onclick; return b; }
  function frame(title) {
    app.innerHTML = '';
    var box = el('div', 'box');
    box.appendChild(el('div', 'logo', 'KINORA'));
    if (title) box.appendChild(el('h1', '', title));
    app.appendChild(box);
    return box;
  }
  function msg(box, text) { if (text) { var m = el('p', 'msg', text); m.setAttribute('role', 'alert'); box.appendChild(m); } }

  var screen = '';
  function showBusy(text) {
    screen = 'busy';
    var box = frame('');
    box.appendChild(el('div', 'spin'));
    box.appendChild(el('p', 'lede', text));
    var c = button('', t('cancel'), function () { opening++; showList(); });
    box.appendChild(c);
    c.focus();
  }
  function serverRow(s, list, saved) {
    var row = el('div', 'row');
    var b = button('server', '', function () { saved ? open(s) : open({ id: s.id, url: s.address, urls: merge(s.address, s.public_url), name: s.name }); });
    b.appendChild(el('b', '', s.name || hostOf(s.url || s.address)));
    b.appendChild(el('span', '', hostOf(s.url || s.address)));
    row.appendChild(b);
    if (saved) {
      row.appendChild(button('ghost', t('remove'), function () {
        store(load().filter(function (x) { return x.id !== s.id; }));
        showList();
      }));
    }
    list.appendChild(row);
    return b;
  }
  function showList(error) {
    screen = 'list';
    var servers = load();
    if (!servers.length && !error) return showAdd('');
    var box = frame(t('title'));
    msg(box, error);
    var list = el('div', 'list');
    var first = null;
    servers.forEach(function (s) { var b = serverRow(s, list, true); first = first || b; });
    box.appendChild(list);
    var acts = el('div', 'acts');
    acts.appendChild(button('', t('add'), function () { showAdd(''); }));
    box.appendChild(acts);
    var off = offlineServers();
    var offFirst = null;
    if (off.length) {
      box.appendChild(el('h2', '', t('offline')));
      var ol = el('div', 'list');
      off.forEach(function (o) {
        var b = button('server', '', function () { showOffline(o); });
        b.appendChild(el('b', '', t('offlineRow', o.name || '', o.profile.name || '')));
        b.appendChild(el('span', '', String(o.count)));
        var row = el('div', 'row');
        row.appendChild(b);
        ol.appendChild(row);
        offFirst = offFirst || b;
      });
      box.appendChild(ol);
    }
    if (TV) box.appendChild(el('p', 'hint', t('exitHint')));
    // the server did not answer, but there is something to watch: start there
    ((error && offFirst) || first || acts.firstChild).focus();
  }

  // ------------------------------------------------------------------ offline downloads (Android)
  // The server's web client asks the server to prepare an MP4 and hands it to the system's DownloadManager
  // (KinoraOffline.java). Here they play without the server: only the downloads of the last profile used with
  // that server (a kids profile never sees another profile's), and what was watched waits for the next connection.
  function offlineCall(fn) {
    try { var r = JSON.parse(fn()); return r && r.error ? null : r; } catch (e) { return null; }
  }
  function offlineServers() { return offline ? offlineCall(function () { return offline.servers(); }) || [] : []; }
  function fileSrc(path) {
    if (window.Capacitor && Capacitor.convertFileSrc) return Capacitor.convertFileSrc(path);
    return 'http://localhost/_capacitor_file_' + path;
  }
  function itemLabel(it) {
    var m = it.meta || {};
    return m.series ? m.series + (m.episode ? ' · ' + m.episode : '') + (it.title ? ' · ' + it.title : '') : (it.title || '');
  }
  function showOffline(o, error) {
    screen = 'offline';
    var items = offlineCall(function () { return offline.saved(o.id); }) || [];
    var box = frame(t('offline'));
    box.appendChild(el('p', 'lede', t('offlineRow', o.name || '', o.profile.name || '')));
    msg(box, error);
    if (!items.length) box.appendChild(el('p', 'hint', t('noDownloads')));
    var list = el('div', 'list');
    var first = null;
    items.forEach(function (it) {
      var row = el('div', 'row');
      var b = button('server', '', function () { playOffline(o, it); });
      b.appendChild(el('b', '', itemLabel(it)));
      b.appendChild(el('span', '', [it.quality, it.size ? Math.round(it.size / 1048576) + ' MB' : ''].filter(Boolean).join(' · ')));
      row.appendChild(b);
      row.appendChild(button('ghost', t('del'), function () {
        offlineCall(function () { return offline.forget(o.id, it.id); });
        showOffline(o);
      }));
      list.appendChild(row);
      first = first || b;
    });
    box.appendChild(list);
    box.appendChild(el('p', 'hint', t('offlineHint')));
    var acts = el('div', 'acts');
    acts.appendChild(button('', t('back'), function () { showList(); }));
    box.appendChild(acts);
    (first || acts.firstChild).focus();
  }
  var player = null;
  function playOffline(o, it) {
    screen = 'player';
    app.innerHTML = '';
    var v = document.createElement('video');
    v.className = 'offline-player';
    v.controls = true;
    v.autoplay = true;
    v.setAttribute('playsinline', '');
    v.src = fileSrc(it.file);
    (it.subs || []).forEach(function (sub, i) {
      var tr = document.createElement('track');
      tr.kind = 'subtitles';
      tr.src = fileSrc(sub.file);
      tr.srclang = (sub.lang || '').slice(0, 2);
      tr.label = sub.label || sub.lang || String(i + 1);
      v.appendChild(tr);
    });
    // resume: what was watched offline last, else where the profile stood when it was downloaded
    var q = it.queued, start = q ? (q.ended ? 0 : q.position) : ((it.meta && it.meta.position) || 0);
    var seek = function () { v.removeEventListener('loadedmetadata', seek); if (start > 0 && start < v.duration - 5) v.currentTime = start; };
    v.addEventListener('loadedmetadata', seek);
    var last = 0;
    var report = function (ended) { try { offline.played(o.id, it.id, v.currentTime || 0, !!ended); } catch (e) { /* storage full */ } };
    v.addEventListener('timeupdate', function () { if (Math.abs(v.currentTime - last) >= 10) { last = v.currentTime; report(false); } });
    v.addEventListener('pause', function () { if (!v.ended) report(false); });
    v.addEventListener('ended', function () { report(true); closePlayer(); });
    v.addEventListener('error', function () { closePlayer(t('playFailed')); });
    app.appendChild(v);
    player = { video: v, server: o, report: report };
    try { if (native) native.keepAwake(true); } catch (e) { /* old app */ }
    v.focus();
  }
  function closePlayer(error) {
    if (!player) return;
    var p = player;
    player = null;
    if (!p.video.ended && !error) p.report(false);
    p.video.pause();
    p.video.removeAttribute('src');
    try { if (native) native.keepAwake(false); } catch (e) { /* old app */ }
    showOffline(p.server, error);
  }
  function showAdd(value, error) {
    screen = 'add';
    var box = frame(t('add'));
    msg(box, error);
    var form = el('form', 'form');
    var label = el('label', 'field');
    label.appendChild(el('span', '', t('address')));
    var input = el('input');
    input.type = 'url'; input.value = value || ''; input.autocomplete = 'off'; input.spellcheck = false;
    input.setAttribute('autocapitalize', 'off'); input.setAttribute('inputmode', 'url');
    label.appendChild(input);
    form.appendChild(label);
    form.appendChild(el('p', 'hint', t('addrHint')));
    var go = button('primary', t('connect'));
    go.type = 'submit';
    form.appendChild(go);
    form.onsubmit = function (e) { e.preventDefault(); if (input.value.trim()) connect(input.value.trim()); };
    box.appendChild(form);
    var foundBox = el('div', 'found');
    box.appendChild(foundBox);
    var acts = el('div', 'acts');
    if (canSearch) acts.appendChild(button('', t('search'), function () { runSearch(foundBox, this); }));
    if (load().length) acts.appendChild(button('ghost', t('back'), function () { showList(); }));
    box.appendChild(acts);
    // on a TV the on-screen keyboard opens when OK is pressed on the field, not straight away
    if (TV) (load().length || !canSearch ? input : acts.firstChild || input).focus(); else if (!value) input.focus(); else go.focus();
    if (!load().length && canSearch && !error) runSearch(foundBox, acts.firstChild);
  }
  function runSearch(box, btn) {
    if (btn) btn.disabled = true;
    box.innerHTML = '';
    var status = el('p', 'lede', t('searching'));
    box.appendChild(status);
    var list = el('div', 'list');
    box.appendChild(list);
    var n = 0, here = screen;
    search(function (s) {
      if (screen !== here) return;
      if (!n++) status.textContent = t('found');
      var b = serverRow(s, list, false);
      if (n === 1 && TV) b.focus();
    }).then(function () {
      if (screen !== here) return;
      if (btn) btn.disabled = false;
      if (!n) status.textContent = t('none');
    });
  }

  // ------------------------------------------------------------------ remote control
  function focusables() {
    var all = app.querySelectorAll('button, input, a[href]');
    var out = [];
    for (var i = 0; i < all.length; i++) if (!all[i].disabled && all[i].offsetParent !== null) out.push(all[i]);
    return out;
  }
  function move(dir) {
    var a = document.activeElement, items = focusables();
    if (!a || items.indexOf(a) < 0) { if (items[0]) items[0].focus(); return; }
    var r = a.getBoundingClientRect(), cx = (r.left + r.right) / 2, cy = (r.top + r.bottom) / 2, best = null, bs = Infinity;
    items.forEach(function (e) {
      if (e === a) return;
      var b = e.getBoundingClientRect(), dx = (b.left + b.right) / 2 - cx, dy = (b.top + b.bottom) / 2 - cy, main, cross;
      if (dir === 'l') { if (b.right > r.left + 1) return; main = -dx; cross = Math.abs(dy); }
      else if (dir === 'r') { if (b.left < r.right - 1) return; main = dx; cross = Math.abs(dy); }
      else if (dir === 'u') { if (b.bottom > r.top + 1) return; main = -dy; cross = Math.abs(dx); }
      else { if (b.top < r.bottom - 1) return; main = dy; cross = Math.abs(dx); }
      var sc = main + cross * 2.5;
      if (sc < bs) { bs = sc; best = e; }
    });
    if (best) { best.focus(); if (best.scrollIntoView) best.scrollIntoView(false); }
  }
  function exit() {
    try {
      if (native) return native.exit();
      if (platform === 'tizen' && window.tizen) return tizen.application.getCurrentApplication().exit();
      if (platform === 'webos' && window.webOS && webOS.platformBack) return webOS.platformBack();
    } catch (e) { /* fall through */ }
    window.close();
  }
  /** Back: one screen back; false when this is the first screen (the app exits). */
  function back() {
    var a = document.activeElement;
    if (a && a.tagName === 'INPUT' && TV) { a.blur(); var f = focusables(); if (f[1]) f[1].focus(); return true; }
    if (screen === 'busy') { opening++; showList(); return true; }
    if (screen === 'player') { closePlayer(); return true; }
    if (screen === 'offline') { showList(); return true; }
    if (screen === 'add' && load().length) { showList(); return true; }
    return false;
  }
  window.kinoraBack = back; // Android: MainActivity asks the page first
  document.addEventListener('keydown', function (e) {
    var k = e.keyCode, typing = document.activeElement && document.activeElement.tagName === 'INPUT';
    var dir = { 37: 'l', 38: 'u', 39: 'r', 40: 'd' }[k];
    if (dir) {
      if (typing && (dir === 'l' || dir === 'r')) return;
      e.preventDefault(); move(dir); return;
    }
    if (k === 10009 || k === 461 || e.key === 'GoBack' || e.key === 'BrowserBack' || (TV && (k === 27 || (k === 8 && !typing)))) {
      e.preventDefault();
      if (!back()) exit();
    }
  });
  try {
    if (window.tizen && tizen.tvinputdevice) {
      ['MediaPlayPause', 'MediaPlay', 'MediaPause', 'MediaStop', 'MediaFastForward', 'MediaRewind', 'MediaTrackNext', 'MediaTrackPrevious']
        .forEach(function (k) { try { tizen.tvinputdevice.registerKey(k); } catch (e) { /* not on this model */ } });
    }
  } catch (e) { /* not a packaged Tizen app */ }

  // ------------------------------------------------------------------ start
  // Fresh start: straight to the last server. Coming back (Switch server, Back from an unreachable page): the list.
  var intent = location.hash.replace('#', '');
  if (intent === 'exit') { exit(); return; }
  var servers = load().sort(function (a, b) { return (b.lastUsed || 0) - (a.lastUsed || 0); });
  if (intent !== 'servers' && !session('kinora.launched') && servers.length) open(servers[0]);
  else showList();
  window.addEventListener('pageshow', function (e) { if (e.persisted) { opening++; showList(); } });
})();
