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
        out.push({ kind: "meetup", k: "📅 " + (today ? tr("screen_today") : tr("screen_week")), title: m.title, text: m.venue || "", meta: when(m.start), full: [m.venue, m.text].filter(Boolean).join("\n\n"), link: location.origin + "/meetups/" + m.id });
      });
    (b.notes || []).filter(function (n) { // not reactions (a kept link line is a line), not coffee codes
      return (/https?:\/\//.test(n.text) || !(n.tags || []).some(function (t) { return /^re[0-9a-f]{10}$/.test(t); })) && !/\/c#v1\./.test(n.text); })
      .forEach(function (n) { out.push({ kind: "note", k: "💬", title: firstLine(noLinks(n.text)), text: noLinks(n.text.split("\n").slice(1).join(" ")), meta: (n.author ? "@" + n.author + " · " : "") + when(n.at), full: noLinks(n.text), link: location.origin + "/n/" + n.id }); });
    (b.news || []).forEach(function (it) { out.push({ kind: "news", k: "📰 " + it.host, title: it.title, text: it.text || "", meta: when(it.at), full: it.text || "", link: it.link }); });
    (b.posts || []).filter(function (p) { return !p.bot && !(p.labels || []).length; }).slice(0, 12)
      .forEach(function (p) { out.push({ kind: "post", k: "🦋 @" + p.handle, title: "", text: p.text, meta: when(p.createdAt), full: p.text, link: p.url }); });
    return out.filter(function (c) { return c.title || c.text; });
  }
  function card(c) {
    var d = el("article", "screen-card " + c.kind);
    d.appendChild(el("div", "k", c.k));
    if (c.title) d.appendChild(el("h2", "", c.title));
    if (c.text) d.appendChild(el("p", "", c.text));
    if (c.meta) d.appendChild(el("div", "m", c.meta));
    if (touch) { // tappable; no role=button (that styles it as a pill)
      d.tabIndex = 0; d.onclick = function () { detail(c); };
      d.onkeydown = function (e) { if (e.key === "Enter" || e.key === " ") { e.preventDefault(); detail(c); } };
    }
    return d;
  }

  // Touch (84b, ?mode=touch): an interactive public screen. A tap opens a
  // card large, with a code to take it along on your phone; ‹ › page by
  // hand; after a minute without a touch it goes back to the overview.
  var touch = q.get("mode") === "touch", idle, openCard = null;
  if (touch) document.body.classList.add("touch");
  function detail(c) {
    close();
    openCard = el("div", "screen-detail");
    openCard.setAttribute("role", "dialog");
    var box = el("article", "screen-card big " + c.kind);
    var x = el("button", "pill screen-close", "✕"); x.type = "button"; x.setAttribute("aria-label", tr("close")); x.onclick = close;
    box.appendChild(x);
    box.appendChild(el("div", "k", c.k));
    if (c.title) box.appendChild(el("h2", "", c.title));
    if (c.full) box.appendChild(el("p", "full", c.full));
    if (c.meta) box.appendChild(el("div", "m", c.meta));
    if (c.link) {
      var qq = qrcode(0, "M"); qq.addData(c.link); qq.make();
      var take = el("div", "take");
      take.innerHTML = qq.createSvgTag({ cellSize: 4, margin: 1, scalable: true });
      take.appendChild(el("p", "m", tr("screen_take")));
      box.appendChild(take);
    }
    openCard.appendChild(box);
    openCard.onclick = function (e) { if (e.target === openCard) close(); };
    document.body.appendChild(openCard);
    clearTimeout(timer); // no paging under an open card
  }
  function close() { if (openCard) { openCard.remove(); openCard = null; show(); } }
  function poke() { // a touch: stay; a minute without one: back to the start
    clearTimeout(idle);
    idle = setTimeout(function () { if (openCard) { openCard.remove(); openCard = null; } at = 0; show(); }, 60000);
  }
  if (touch) {
    ["pointerdown", "keydown"].forEach(function (ev) { document.addEventListener(ev, poke); });
    var nav = el("div", "screen-nav");
    [["‹", -1], ["›", 1]].forEach(function (b) {
      var btn = el("button", "pill", b[0]); btn.type = "button";
      btn.onclick = function () { at = (at + b[1] + pages.length) % Math.max(1, pages.length); show(); };
      nav.appendChild(btn);
    });
    document.body.appendChild(nav);
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
    if (!openCard) timer = setTimeout(function () { at++; show(); }, touch ? speed * 2 : speed); // slower when people read along
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
