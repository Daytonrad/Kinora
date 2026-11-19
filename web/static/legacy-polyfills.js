/* Kinora web client – the few newer built-ins app.js uses, for 2019–2020 TV engines (Chromium 63–79).
   The server prepends this to app.legacy.js (app.js lowered to ES2017 syntax); modern browsers never load it. */
(function () {
  'use strict';
  function def(o, name, fn) { if (!o[name]) Object.defineProperty(o, name, { value: fn, configurable: true, writable: true }); }
  def(Array.prototype, 'flat', function flat(depth) {
    var d = depth === undefined ? 1 : Number(depth);
    var out = [];
    (function walk(a, level) {
      for (var i = 0; i < a.length; i++) {
        if (!(i in a)) continue;
        if (Array.isArray(a[i]) && level > 0) walk(a[i], level - 1); else out.push(a[i]);
      }
    })(this, d);
    return out;
  });
  def(Object, 'fromEntries', function fromEntries(it) {
    var o = {};
    Array.from(it, function (p) { o[p[0]] = p[1]; });
    return o;
  });
  def(String.prototype, 'matchAll', function matchAll(re) {
    var flags = re.flags.indexOf('g') < 0 ? re.flags + 'g' : re.flags;
    var r = new RegExp(re.source, flags), s = String(this), out = [], m;
    while ((m = r.exec(s)) !== null) { out.push(m); if (m[0] === '') r.lastIndex++; }
    return out[Symbol.iterator]();
  });
  function replaceChildren() {
    while (this.lastChild) this.removeChild(this.lastChild);
    this.append.apply(this, arguments);
  }
  def(Element.prototype, 'replaceChildren', replaceChildren);
  def(DocumentFragment.prototype, 'replaceChildren', replaceChildren);
})();
