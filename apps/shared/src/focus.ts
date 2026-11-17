// Spatial navigation for remotes: arrows move focus to the nearest focusable
// element in that direction, rows remember their last focused card, and the
// focused element is always scrolled into view (with an overscan margin).
const SEL = 'button:not([disabled]), a[href], input:not([disabled]), select:not([disabled]), textarea, [tabindex="0"]';

export type Dir = 'up' | 'down' | 'left' | 'right';

function visible(el: HTMLElement): boolean {
  if (el.offsetParent === null && getComputedStyle(el).position !== 'fixed') return false;
  const r = el.getBoundingClientRect();
  return r.width > 0 && r.height > 0;
}

/** The active focus scope: the top-most open layer (sheet/dialog), else the page. */
export function scope(): HTMLElement {
  const layers = document.querySelectorAll('.layer');
  return (layers.length ? layers[layers.length - 1] : document.getElementById('app')) as HTMLElement;
}

export function focusables(root: HTMLElement = scope()): HTMLElement[] {
  return (Array.prototype.slice.call(root.querySelectorAll(SEL)) as HTMLElement[]).filter(visible);
}

const rowMemory = new WeakMap<Element, HTMLElement>();

function rowOf(el: Element): Element | null {
  return el.closest ? el.closest('[data-row]') : null;
}

export function focusEl(el: HTMLElement | null | undefined): void {
  if (!el) return;
  el.focus();
  const row = rowOf(el);
  if (row) rowMemory.set(row, el);
  reveal(el);
}

/** Scrolls every scrollable ancestor so el is fully visible, keeping a margin. */
export function reveal(el: HTMLElement): void {
  const margin = 48;
  let p = el.parentElement;
  while (p && p !== document.body) {
    const cs = getComputedStyle(p);
    const r = el.getBoundingClientRect(), pr = p.getBoundingClientRect();
    if (/(auto|scroll)/.test(cs.overflowX) && p.scrollWidth > p.clientWidth) {
      if (r.left < pr.left + margin) p.scrollLeft -= pr.left + margin - r.left;
      else if (r.right > pr.right - margin) p.scrollLeft += r.right - (pr.right - margin);
    }
    if (/(auto|scroll)/.test(cs.overflowY) && p.scrollHeight > p.clientHeight) {
      if (r.top < pr.top + margin) p.scrollTop -= pr.top + margin - r.top;
      else if (r.bottom > pr.bottom - margin) p.scrollTop += r.bottom - (pr.bottom - margin);
    }
    p = p.parentElement;
  }
  const r = el.getBoundingClientRect();
  const vh = window.innerHeight;
  if (r.top < margin * 2) window.scrollBy(0, r.top - margin * 2);
  else if (r.bottom > vh - margin) window.scrollBy(0, r.bottom - vh + margin);
}

/** Moves focus in a direction; returns false when nothing lies that way. */
export function move(dir: Dir): boolean {
  const all = focusables();
  if (!all.length) return false;
  const cur = document.activeElement as HTMLElement | null;
  if (!cur || all.indexOf(cur) < 0) { focusEl(preferred(all)); return true; }
  const c = cur.getBoundingClientRect();
  const cx = c.left + c.width / 2, cy = c.top + c.height / 2;
  let best: HTMLElement | null = null, bestScore = Infinity;
  for (const el of all) {
    if (el === cur) continue;
    const r = el.getBoundingClientRect();
    const x = r.left + r.width / 2, y = r.top + r.height / 2;
    let main: number, cross: number;
    switch (dir) {
      case 'left': if (r.right > c.left + 2) continue; main = c.left - r.right; cross = Math.abs(y - cy); break;
      case 'right': if (r.left < c.right - 2) continue; main = r.left - c.right; cross = Math.abs(y - cy); break;
      case 'up': if (r.bottom > c.top + 2) continue; main = c.top - r.bottom; cross = Math.abs(x - cx); break;
      default: if (r.top < c.bottom - 2) continue; main = r.top - c.bottom; cross = Math.abs(x - cx);
    }
    // overlap on the cross axis counts as aligned
    const overlap = dir === 'left' || dir === 'right'
      ? Math.min(r.bottom, c.bottom) - Math.max(r.top, c.top)
      : Math.min(r.right, c.right) - Math.max(r.left, c.left);
    const score = Math.max(0, main) + (overlap > 0 ? 0 : cross * 2) + cross * 0.3;
    if (score < bestScore) { bestScore = score; best = el; }
  }
  if (!best) return false;
  // entering another row vertically: return to that row's remembered card
  if (dir === 'up' || dir === 'down') {
    const row = rowOf(best);
    if (row && row !== rowOf(cur)) {
      const mem = rowMemory.get(row);
      if (mem && document.body.contains(mem) && visible(mem)) best = mem;
    }
  }
  focusEl(best);
  return true;
}

function preferred(all: HTMLElement[]): HTMLElement {
  const auto = scope().querySelector('[data-autofocus]') as HTMLElement | null;
  return auto && all.indexOf(auto) >= 0 ? auto : all[0];
}

/** Focuses the view's preferred element (data-autofocus) or the first focusable. */
export function focusFirst(): void {
  const all = focusables();
  if (all.length) focusEl(preferred(all));
}
