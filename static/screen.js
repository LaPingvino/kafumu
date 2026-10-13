// The area on a public screen (84a): the area's own content (the same
// bundle as its page), fitted to the physical screen: as many cards as fit
// without scrolling, the rest in turns. Settings come from the link:
// ?cell=…&speed=<seconds>. Nothing is stored on the server for a screen.
(function () {
  "use strict";
  var T = window.KAFUMU_T || {}, G = window.kafumuGeo, q = new URLSearchParams(location.search);
  function tr(k, v) { var s = T[k] || k; for (var n in v || {}) s = s.split("{" + n + "}").join(v[n]); return s; }
  function el(tag, cls, text) { var e = document.createElement(tag); if (cls) e.className = cls; if (text != null) e.textContent = text; return e; }
  var cell = (q.get("cell") || "").toLowerCase(), speed = Math.max(5, Math.min(120, parseInt(q.get("speed"), 10) || 12)) * 1000;
  var grid = document.getElementById("screen-grid");
  if (!G || !G.validCell(cell)) { grid.appendChild(el("p", "", "?cell=…")); return; }

  // The corner: a QR code and the link, so people here add to their area.
  var link = location.origin + "/?cell=" + cell;
  var qr = qrcode(0, "M"); qr.addData(link); qr.make();
  document.getElementById("screen-qr-code").innerHTML = qr.createSvgTag({ cellSize: 4, margin: 1, scalable: true });
  document.getElementById("screen-link").textContent = location.host + "/?cell=" + cell;

  function clock() {
    document.getElementById("screen-clock").textContent = new Date().toLocaleString(window.KAFUMU_LOCALE, { weekday: "long", day: "numeric", month: "long", hour: "2-digit", minute: "2-digit" });
  }
  clock(); setInterval(clock, 20000);

  function when(iso) { return new Date(iso).toLocaleString(window.KAFUMU_LOCALE, { weekday: "short", day: "numeric", month: "short", hour: "2-digit", minute: "2-digit" }); }
  function firstLine(t) { return String(t || "").split("\n")[0]; }
  function noLinks(t) { return String(t || "").replace(/https?:\/\/\S+/g, "").trim(); }

  // cards: meetups of today and this week first, then what people here say,
  // then news from the area's sources, then a few Bluesky posts.
  function cards(b) {
    var out = [], now = Date.now(), week = now + 7 * 864e5;
    (b.meetups || []).filter(function (m) { return new Date(m.end || m.start) > now && new Date(m.start) < week; })
      .sort(function (x, y) { return new Date(x.start) - new Date(y.start); }).forEach(function (m) {
        var today = new Date(m.start).toDateString() === new Date().toDateString();
        out.push({ kind: "meetup", k: "📅 " + (today ? tr("screen_today") : tr("screen_week")), title: m.title, text: m.venue || "", meta: when(m.start) });
      });
    (b.notes || []).filter(function (n) { // not reactions (a kept link line is a line), not coffee codes
      return (/https?:\/\//.test(n.text) || !(n.tags || []).some(function (t) { return /^re[0-9a-f]{10}$/.test(t); })) && !/\/c#v1\./.test(n.text); })
      .forEach(function (n) { out.push({ kind: "note", k: "💬", title: firstLine(noLinks(n.text)), text: noLinks(n.text.split("\n").slice(1).join(" ")), meta: (n.author ? "@" + n.author + " · " : "") + when(n.at) }); });
    (b.news || []).forEach(function (it) { out.push({ kind: "news", k: "📰 " + it.host, title: it.title, text: it.text || "", meta: when(it.at) }); });
    (b.posts || []).filter(function (p) { return !p.bot && !(p.labels || []).length; }).slice(0, 12)
      .forEach(function (p) { out.push({ kind: "post", k: "🦋 @" + p.handle, title: "", text: p.text, meta: when(p.createdAt) }); });
    return out.filter(function (c) { return c.title || c.text; });
  }
  function card(c) {
    var d = el("article", "screen-card " + c.kind);
    d.appendChild(el("div", "k", c.k));
    if (c.title) d.appendChild(el("h2", "", c.title));
    if (c.text) d.appendChild(el("p", "", c.text));
    if (c.meta) d.appendChild(el("div", "m", c.meta));
    return d;
  }

  // pages: fill the grid until it would scroll, then start a new page.
  var all = [], pages = [], at = 0, timer;
  function layout() {
    var cols = Math.max(1, Math.floor(grid.clientWidth / 440));
    grid.style.setProperty("--cols", cols);
    pages = [];
    var i = 0;
    while (i < all.length) {
      grid.textContent = "";
      var start = i;
      while (i < all.length) {
        grid.appendChild(card(all[i]));
        if (grid.scrollHeight > grid.clientHeight + 2 && i > start) { grid.lastChild.remove(); break; }
        i++;
      }
      pages.push([start, i]);
      if (i === start) i++; // one card taller than the screen: it gets a page of its own
    }
    at = 0; show();
  }
  function show() {
    grid.textContent = "";
    if (!pages.length) { grid.appendChild(el("p", "dim", tr("screen_empty"))); return; }
    var p = pages[at % pages.length];
    for (var i = p[0]; i < p[1]; i++) grid.appendChild(card(all[i]));
    document.getElementById("screen-page").textContent = pages.length > 1 ? (at % pages.length + 1) + " / " + pages.length : "";
    clearTimeout(timer);
    timer = setTimeout(function () { at++; show(); }, speed);
  }
  function load() {
    var cells = G.rings(cell, 1).map(function (p) { return p[0]; });
    fetch("/bundle?cells=" + cells.join(","), { cache: "reload" }).then(function (r) { return r.json(); }).then(function (b) {
      if (b.near && b.near.name) document.getElementById("screen-place").textContent = b.near.name + (b.near.city ? ", " + b.near.city : "");
      all = cards(b);
      layout();
    }).catch(function () {});
  }
  load();
  setInterval(load, 10 * 60000); // fresh content every ten minutes
  var rs; window.addEventListener("resize", function () { clearTimeout(rs); rs = setTimeout(layout, 300); });
  // Keep the screen on, and fullscreen on the first tap or key.
  function wake() { if (navigator.wakeLock) navigator.wakeLock.request("screen").catch(function () {}); }
  wake(); document.addEventListener("visibilitychange", function () { if (!document.hidden) wake(); });
  document.addEventListener("click", function () { if (!document.fullscreenElement && document.documentElement.requestFullscreen) document.documentElement.requestFullscreen().catch(function () {}); }, { once: true });
})();
