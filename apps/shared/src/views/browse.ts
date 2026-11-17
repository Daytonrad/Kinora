// Home (facade + shelves), library, search, detail.
import { h, put, icon, toast, debounce } from '../dom';
import { T } from '../i18n';
import { state, api, route, shell, shelf, card, spinner, choose, go, Item, itemSub, onCleanup } from '../app';
import { focusFirst } from '../focus';

// ------------------------------------------------------------------ home

interface HomeRow { id: string; title: string; style: 'poster' | 'landscape'; items: Item[]; reasons?: Record<string, string> }

function metaLine(it: Item): HTMLElement {
  const bits: (string | HTMLElement)[] = [];
  if (it.year) bits.push(String(it.year));
  if (it.content_rating) bits.push(h('span.badge', null, it.content_rating));
  if (it.runtime_sec) { const m = Math.round(it.runtime_sec / 60); bits.push(T.hm(Math.floor(m / 60), m % 60)); }
  if (it.community_rating) bits.push('★ ' + it.community_rating.toFixed(1));
  if (it.genres && it.genres.length) bits.push(it.genres.slice(0, 3).join(', '));
  const out = h('p.meta');
  bits.forEach((b, i) => { if (i) out.appendChild(h('span.sep', null, '·')); out.appendChild(typeof b === 'string' ? document.createTextNode(b) : b); });
  return out;
}

function resumeLabel(it: Item): string {
  const ud = it.user_data;
  return ud && ud.position > 30 && !ud.played ? T.resume : T.heroPlay;
}

function playHref(it: Item): string {
  const ud = it.user_data;
  return `#/play/${it.id}` + (ud && ud.position > 30 && !ud.played ? `?start=${Math.floor(ud.position)}` : '');
}

function facade(): { el: HTMLElement; show: (it: Item, reason?: string, withActions?: boolean) => void } {
  const bg = h('div.facade-bg');
  const reasonEl = h('p.reason');
  const title = h('h1.display');
  const meta = h('div');
  const syn = h('p.synopsis');
  const actions = h('div.actions');
  const el = h('section.facade', null, bg, h('div.facade-scrim'), h('div.facade-copy', null, reasonEl, title, meta, syn, actions));
  let shownId = 0;
  const show = (it: Item, reason?: string, withActions = true) => {
    if (shownId === it.id) return;
    shownId = it.id;
    // the server falls back backdrop → thumb → series → first episode frame
    bg.style.backgroundImage = `url("${api().image(it.id, 'backdrop')}")`;
    reasonEl.textContent = reason || '';
    title.textContent = it.kind === 'episode' ? it.series_title || it.title : it.title;
    put(meta, metaLine(it));
    syn.textContent = it.overview || '';
    if (withActions) {
      put(actions,
        (it.kind === 'movie' || it.kind === 'episode') && h('a.btn.primary.big', { href: playHref(it) }, icon('play'), resumeLabel(it)),
        h('a.btn.big', { href: `#/item/${it.id}` }, icon('info'), T.details));
    }
  };
  return { el, show };
}

route('home', async () => {
  const main = shell('home', spinner());
  main.classList.add('home');
  const data = await api().get<{ hero: Item | null; rows: HomeRow[] }>('/api/home');
  put(main);
  if (!data.hero && !data.rows.length) {
    main.appendChild(h('div.empty', null, h('p.lede', null, T.emptyHome)));
    return;
  }
  const f = facade();
  if (data.hero) f.show(data.hero, (data.rows[0] && data.rows[0].reasons && data.rows[0].reasons[data.hero.id]) || '');
  main.appendChild(f.el);
  const tv = state.mode === 'tv';
  // TV: the facade follows the beam (focused card); phones keep a static hero
  const follow = tv ? debounce((it: Item, reason?: string) => f.show(it, reason, false), 120) : undefined;
  const rows = h('div.rows');
  for (const r of data.rows) {
    rows.appendChild(shelf(r.title, r.items, r.style, { reasons: r.reasons, onFocus: follow && ((it: Item) => follow(it, r.reasons && r.reasons[it.id])) }));
  }
  main.appendChild(rows);
  if (tv) {
    const first = rows.querySelector('.card') as HTMLElement | null;
    if (first) first.setAttribute('data-autofocus', '');
  }
});

// ------------------------------------------------------------------ library

interface Library { id: number; name: string; kind: string }

route('library', async (parts, q) => {
  const main = shell('library', spinner());
  const libs = await api().get<Library[]>('/api/libraries');
  if (!libs.length) { put(main, h('div.empty', null, h('p.lede', null, T.emptyHome))); return; }
  const libId = +(parts[1] || 0) || libs[0].id;
  const sort = q.get('sort') || 'title';
  const played = q.get('played') || '';
  const fav = q.get('favorite') === '1';
  const lib = libs.filter(l => l.id === libId)[0] || libs[0];
  const link = (o: Record<string, string>) => {
    const p = new URLSearchParams({ sort, played, favorite: fav ? '1' : '' });
    Object.keys(o).forEach(k => p.set(k, o[k]));
    const qs = p.toString().replace(/[^&=]+=(&|$)/g, '').replace(/&$/, '');
    return `#/library/${o.lib || lib.id}` + (qs ? '?' + qs : '');
  };
  const order = sort === 'title' ? 'asc' : 'desc';
  const grid = h('div.grid', { 'data-row': '' });
  const more = h('button.btn.wide', { onclick: () => load() }, T.loadMore);
  let offset = 0, total = 0;
  const load = async () => {
    more.setAttribute('disabled', '');
    const params = new URLSearchParams({ library: String(lib.id), sort, order, offset: String(offset), limit: '60' });
    if (played) params.set('played', played);
    if (fav) params.set('favorite', '1');
    const r = await api().get<{ items: Item[]; total: number }>('/api/items?' + params.toString());
    total = r.total;
    const firstNew = grid.children.length;
    r.items.forEach(it => grid.appendChild(card(it, 'poster')));
    offset += r.items.length;
    more.removeAttribute('disabled');
    more.style.display = offset < total ? '' : 'none';
    if (!total) grid.appendChild(h('p.muted', null, T.libEmpty));
    if (firstNew && state.mode === 'tv') (grid.children[firstNew] as HTMLElement).focus();
  };
  const chip = (label: string, on: boolean, href: string) => h('a.chip' + (on ? '.on' : ''), { href }, label);
  put(main, h('div.page', null,
    h('div.page-head', null, h('h1', null, lib.name)),
    libs.length > 1 && h('div.chips', { 'data-row': '' }, libs.map(l => chip(l.name, l.id === lib.id, link({ lib: String(l.id) })))),
    h('div.chips', { 'data-row': '' },
      chip(T.filterAll, !played && !fav, link({ played: '', favorite: '' })),
      chip(T.filterUnwatched, played === 'false', link({ played: 'false', favorite: '' })),
      chip(T.filterFav, fav, link({ favorite: '1', played: '' })),
      h('button.chip', { onclick: async () => {
        const v = await choose(T.sortBy, Object.keys(T.sorts).map(k => [k, T.sorts[k]] as [string, string]), sort);
        if (v) go(link({ sort: v }));
      } }, T.sortBy + ': ' + T.sorts[sort])),
    grid, more));
  await load();
  if (state.mode === 'tv') { const c = grid.querySelector('.card') as HTMLElement; if (c) c.setAttribute('data-autofocus', ''); }
});

// ------------------------------------------------------------------ search

route('search', async (_p, q) => {
  const input = h('input.search', { type: 'search', placeholder: T.searchPh, 'aria-label': T.search, value: q.get('q') || '', 'data-autofocus': '' }) as HTMLInputElement;
  const results = h('div.grid', { 'data-row': '' });
  const main = shell('search', h('div.page', null, h('div.page-head', null, h('h1', null, T.search)),
    h('form', { onsubmit: (e: Event) => { e.preventDefault(); run(); input.blur(); } }, input), results));
  void main;
  let seq = 0;
  const run = async () => {
    const v = input.value.trim();
    history.replaceState(null, '', '#/search' + (v ? '?q=' + encodeURIComponent(v) : ''));
    if (v.length < 2) { put(results, h('p.muted', null, T.searchHint)); return; }
    const my = ++seq;
    const r = await api().get<Item[]>('/api/search?group=1&q=' + encodeURIComponent(v)); // one card per series
    if (my !== seq) return;
    put(results, r.length ? r.map(it => card(it, 'poster')) : h('p.muted', null, T.searchEmpty));
  };
  input.addEventListener('input', debounce(run, 350));
  if (input.value) await run(); else put(results, h('p.muted', null, T.searchHint));
  if (state.mode !== 'tv') input.focus();
});

// ------------------------------------------------------------------ detail

route('item', async parts => {
  const id = +parts[1];
  const main = shell('', spinner());
  const d = await api().get<any>('/api/items/' + id);
  const it: Item = d.item;
  const ud = it.user_data;
  const playable = it.kind === 'movie' || it.kind === 'episode';
  const target: Item | null = playable ? it : d.play_next || null;

  const actions = h('div.actions');
  const renderActions = () => {
    put(actions,
      target && h('a.btn.primary.big', { href: playHref(target), 'data-autofocus': '' }, icon('play'),
        target.kind === 'episode' && !playable
          ? (target.user_data && target.user_data.position > 30 ? T.resumeEp : T.playEp)(target.season_num || 0, target.episode_num || 0)
          : resumeLabel(target)),
      playable && ud.position > 30 && !ud.played && h('a.btn.big', { href: `#/play/${it.id}` }, T.restart),
      h('button.btn.icon.big' + (ud.played ? '.on' : ''), { 'aria-label': ud.played ? T.markUnplayed : T.markPlayed, title: ud.played ? T.markUnplayed : T.markPlayed,
        onclick: async () => { const r = await api().post(`/api/items/${id}/played`, { played: !ud.played }); Object.assign(ud, r); renderActions(); } }, icon('check')),
      h('button.btn.icon.big' + (ud.favorite ? '.on' : ''), { 'aria-label': T.favorite, title: T.favorite,
        onclick: async () => { const r = await api().post(`/api/items/${id}/favorite`, { favorite: !ud.favorite }); Object.assign(ud, r); renderActions(); } }, icon('heart', ud.favorite)),
      h('button.btn.icon.big' + (ud.stars ? '.on' : ''), { 'aria-label': T.stars, title: T.stars,
        onclick: async () => {
          const v = await choose<number>(T.stars, [1, 2, 3, 4, 5].map(n => [n, '★'.repeat(n)] as [number, string]), ud.stars || undefined);
          if (v) { const r = await api().post(`/api/items/${id}/stars`, { stars: v }); Object.assign(ud, r); renderActions(); }
        } }, icon('star', !!ud.stars), ud.stars ? String(ud.stars) : null),
      (it.kind === 'movie' || it.kind === 'series') && h('button.btn.icon.big', { 'aria-label': T.hide, title: T.hide,
        onclick: async () => { await api().post(`/api/items/${id}/hide`, { hidden: true }); toast(T.hidden); history.back(); } }, icon('hide')));
  };
  renderActions();

  const bgId = it.has_backdrop ? it.id : it.series_id || it.id;
  const head = h('section.detail-head', null,
    h('div.facade-bg', { style: { backgroundImage: `url("${api().image(bgId, 'backdrop')}")` } }), h('div.facade-scrim'),
    h('div.detail-copy', null,
      it.kind === 'episode' && h('p.reason', null, `${it.series_title} · ${T.ep(it.season_num, it.episode_num)}`),
      it.kind === 'season' && h('p.reason', null, it.series_title || ''),
      h('h1.display', null, it.title), metaLine(it),
      playable && ud.position > 30 && !ud.played && it.runtime_sec ? h('div.progress.inline', null, h('i', { style: { width: (ud.position / it.runtime_sec * 100).toFixed(1) + '%' } })) : null,
      it.overview && h('p.synopsis.long', null, it.overview),
      actions));
  put(main, head);

  // seasons → episodes
  if (it.kind === 'series' && d.seasons && d.seasons.length) {
    const epBox = h('div.episodes', { 'data-row': '' });
    const chips = h('div.chips', { 'data-row': '' });
    const showSeason = async (s: Item) => {
      Array.prototype.forEach.call(chips.children, (c: HTMLElement) => c.classList.toggle('on', c.getAttribute('data-id') === String(s.id)));
      put(epBox, spinner());
      const sd = await api().get<any>('/api/items/' + s.id);
      put(epBox, (sd.episodes || []).map(episodeRow));
    };
    d.seasons.forEach((s: Item) => chips.appendChild(h('button.chip', { 'data-id': s.id, onclick: () => showSeason(s) }, T.season(s.season_num || 0))));
    main.appendChild(h('section.below', null, h('h2.shelf-title', null, T.episodes), chips, epBox));
    const pick = (target && d.seasons.filter((s: Item) => s.season_num === target.season_num)[0]) || d.seasons[0];
    showSeason(pick);
  }
  if (it.kind === 'season' && d.episodes) main.appendChild(h('section.below', null, h('h2.shelf-title', null, T.episodes), h('div.episodes', { 'data-row': '' }, d.episodes.map(episodeRow))));
  if (it.kind === 'episode' && d.next) main.appendChild(h('div.below', null, shelf(T.nextEp, [d.next], 'landscape')));
  if (d.similar && d.similar.length) main.appendChild(h('div.below', null, shelf(T.similar, d.similar, 'poster')));
  if (state.mode === 'tv') focusFirst();
  onCleanup(() => {});
});

function episodeRow(e: Item): HTMLElement {
  const ud = e.user_data;
  const pct = e.runtime_sec && ud.position > 0 && !ud.played ? ud.position / e.runtime_sec * 100 : 0;
  return h('a.ep-row', { href: playHref(e), 'aria-label': `${T.ep(e.season_num, e.episode_num)} ${e.title}` },
    h('div.ep-thumb', null, h('img', { src: api().image(e.id, 'thumb'), alt: '', loading: 'lazy' }),
      pct > 0 && h('div.progress', null, h('i', { style: { width: pct.toFixed(1) + '%' } })),
      ud.played && h('span.tick', null, icon('check'))),
    h('div.ep-text', null, h('span.ep-num', null, T.ep(e.season_num, e.episode_num)), h('span.ep-title', null, e.title),
      e.runtime_sec ? h('span.cs', null, T.minutes(Math.round(e.runtime_sec / 60))) : null,
      e.overview && h('span.ep-over', null, e.overview)));
}

export { itemSub };
