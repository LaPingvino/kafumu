// Chat windows on every page (85c, 88a): a contact's chat or a private
// answer opens right where you are. On a desktop they dock at the bottom,
// side by side, and come back on the next page (which ones is kept on this
// device); on a phone one fills the screen until ← closes it.
(function () {
  "use strict";
  var T = window.KAFUMU_T || {}, KEY = "kafumu.openChats", W = 356;
  function el(tag, cls, text) { var e = document.createElement(tag); if (cls) e.className = cls; if (text != null) e.textContent = text; return e; }
  function desktop() { return window.matchMedia("(min-width: 800px)").matches; }
  function saved() { try { return JSON.parse(localStorage.getItem(KEY) || "[]"); } catch (e) { return []; } }
  function persist() { try { localStorage.setItem(KEY, JSON.stringify(desktop() ? order : [])); } catch (e) {} }

  // need loads what a chat needs on pages that don't have it yet.
  var loading;
  function need() {
    if (window.kafumuDevice && window.kafumuPair && window.kafumuOLN) return Promise.resolve();
    if (loading) return loading;
    var v = encodeURIComponent(window.KAFUMU_V || "");
    var files = [!window.kafumuDevice && "device.js", !window.kafumuSHA1 && "sha1.js", !window.kafumuPair && "pair.js", !window.kafumuOLN && "oln.js"].filter(Boolean);
    loading = files.reduce(function (p, f) {
      return p.then(function () {
        return new Promise(function (ok, fail) { var s = document.createElement("script"); s.src = "/static/" + f + "?v=" + v; s.onload = ok; s.onerror = fail; document.head.appendChild(s); });
      });
    }, Promise.resolve());
    return loading;
  }
  var pairInst;
  function pair() {
    if (!pairInst) pairInst = window.kafumuPair.create({ fetch: window.fetch.bind(window), store: window.kafumuDevice.store, origin: location.origin,
      mine: function (t, kw, b) { return window.kafumuOLN.post(t, kw, b, function () {}); } });
    return pairInst;
  }
  function post(t, kw, b) { return window.kafumuOLN.post(t, kw, b, function () {}); }

  var dock, order = [], wins = {};
  function dockEl() {
    if (!dock) { dock = el("div", "chat-dock"); dock.id = "chat-dock"; document.body.appendChild(dock); }
    return dock;
  }
  // layout: on a desktop as many as fit from the right (the oldest closes),
  // on a phone only the newest.
  function layout() {
    var d = desktop(), fit = Math.max(1, Math.floor((window.innerWidth - 16) / W));
    while (order.length > (d ? fit : 1)) { var gone = order.shift(); if (wins[gone]) { wins[gone].remove(); delete wins[gone]; } }
    Object.keys(wins).forEach(function (k) {
      var i = order.indexOf(k), w = wins[k];
      w.hidden = i < 0;
      w.style.right = d && i >= 0 ? (16 + (order.length - 1 - i) * W) + "px" : "";
    });
    document.body.classList.toggle("chat-open", order.length > 0 && !d);
    persist();
  }
  function close(key) {
    order = order.filter(function (k) { return k !== key; });
    if (wins[key]) { wins[key].remove(); delete wins[key]; }
    layout();
    window.dispatchEvent(new CustomEvent("kafumu:chat-closed", { detail: key }));
  }
  var page = document.getElementById("chat-page"); // /chat?k=…: this chat is the page
  function chatURL(key) { return "/chat?k=" + key.slice(0, 2) + encodeURIComponent(key.slice(2)); } // like every other chat link
  function build(key, asPage) {
    var dev = window.kafumuDevice, kind = key.slice(0, 2), id = key.slice(2), p = pair();
    var find = kind === "c:" ? dev.store.contacts() : p.threads();
    return find.then(function (all) {
      var src = all.filter(function (x) { return x.id === id; })[0];
      if (!src) return null;
      var w = el("div", "chat-window"); w.dataset.thread = key;
      w.setAttribute("role", "dialog");
      var title = kind === "c:" ? dev.nameOf(src, T) : "🔒 " + String((src.post && src.post.text) || "").slice(0, 60);
      w.setAttribute("aria-label", title);
      var head = el("div", "win-head"), back;
      if (asPage) { back = el("a", "win-back", "← 🔔"); back.href = "/activity"; back.setAttribute("aria-label", T.back_to_mailbox || "Back"); }
      else {
        back = el("button", "win-back", "←"); back.type = "button";
        back.setAttribute("aria-label", T.close || "Close"); back.title = T.close || "Close";
        back.onclick = function () { close(key); };
      }
      head.appendChild(back); head.appendChild(el("strong", "", title));
      if (kind === "c:") { var card = el("a", "dim small", "🪪 " + (T.contact_card || "Card")); card.href = "/contacts#c-" + encodeURIComponent(id); head.appendChild(card); }
      if (!asPage) { // a docked window pops out into its own page
        var pop = el("a", "dim small pop-out", "⤢"); pop.href = chatURL(key); pop.dataset.page = "1";
        pop.title = T.pop_out || "Open as a page"; pop.setAttribute("aria-label", pop.title);
        pop.onclick = function () { close(key); };
        head.appendChild(pop);
      }
      w.appendChild(head);
      var chat = kind === "c:"
        ? dev.chatPanel(src, T, { events: true,
            send: function (c, msg) { return msg.t === "msg" ? p.sendChat(c, msg.text, post) : p.send(c, msg); },
            check: function (c) { return p.checkContact(c).then(function (got) { return p.readChat(c).then(function (n) { return n ? got.concat([{ t: "msg" }]) : got; }); }); } })
        : dev.chatPanel(src, T, { save: p.putThread,
            send: function (x, msg) { return p.sendChat(x, msg.text, post); },
            check: function (x) { // new lines land in the stored thread: copy them into this one
              return p.readAnswers().then(function () { return p.threads(); }).then(function (ts) {
                var now = ts.filter(function (a) { return a.id === x.id; })[0];
                if (!now || (now.messages || []).length === (x.messages || []).length) return [];
                x.messages = now.messages; return [{ t: "msg" }];
              });
            } });
      w.appendChild(chat.thread);
      chat.thread.querySelectorAll(".close-chat").forEach(function (x) { x.hidden = true; }); // ← closes the window
      if (asPage) {
        w.classList.add("chat-page"); page.appendChild(w); chat.open();
        // Fill what's left of the screen (banners above, the tab bar below), typing at the bottom.
        var fit = function () {
          var nav = document.querySelector(".switcher.bottom"), below = nav && getComputedStyle(nav).display !== "none" ? nav.offsetHeight : 0;
          w.style.height = Math.max(280, window.innerHeight - w.getBoundingClientRect().top - window.scrollY - below - 12) + "px";
        };
        fit(); window.addEventListener("resize", fit);
        if (window.ResizeObserver) new ResizeObserver(function () { fit(); }).observe(document.querySelector("header") ? document.body : w); // bars that show up later push it down
        document.querySelectorAll(".install-bar .no").forEach(function (b) { b.addEventListener("click", function () { setTimeout(fit, 0); }); });
        return w;
      }
      w.addEventListener("keydown", function (e) { if (e.key === "Escape") close(key); });
      dockEl().appendChild(w);
      chat.open();
      return w;
    });
  }
  // open shows the chat for key ("c:<contact id>" or "a:<answer thread id>").
  function open(key, focus) {
    if (!desktop()) { location.href = chatURL(key); return Promise.resolve(null); } // a phone: the chat's own page
    return need().then(function () {
      order = order.filter(function (k) { return k !== key; }).concat([key]);
      return wins[key] ? wins[key] : build(key).then(function (w) { if (w) wins[key] = w; else order = order.filter(function (k) { return k !== key; }); return w; });
    }).then(function (w) {
      layout();
      if (w && focus !== false) { var inp = w.querySelector(".chat-form input"); if (inp) inp.focus({ preventScroll: true }); }
      return w;
    });
  }
  // keyOf: the chat a link means (/chat?k=c:<id>; older /activity#chat-<id>).
  function keyOf(href) {
    var m = /\/chat\?k=([^&#]+)/.exec(href || "");
    if (m) return decodeURIComponent(m[1]);
    m = /\/activity#(chat|answer)-(.+)$/.exec(href || "");
    return m ? (m[1] === "chat" ? "c:" : "a:") + decodeURIComponent(m[2]) : "";
  }
  // Any link to a chat opens it in place instead of going to the mailbox.
  document.addEventListener("click", function (e) {
    var a = e.target.closest && e.target.closest("a[href]");
    if (!a || a.dataset.page || e.ctrlKey || e.metaKey || e.shiftKey || !desktop()) return; // phones follow the link
    var key = keyOf(a.getAttribute("href"));
    if (!key) return;
    e.preventDefault();
    open(key);
  });
  // A notification tapped while Kafumu is open: the service worker asks
  // this page to open the chat (88a).
  if (navigator.serviceWorker) navigator.serviceWorker.addEventListener("message", function (e) {
    var key = e.data && e.data.open && keyOf(e.data.open);
    if (key) open(key); else if (e.data && e.data.open) location.href = e.data.open;
  });
  window.addEventListener("resize", layout);
  window.kafumuChat = { open: open, close: close, keyOf: keyOf, isOpen: function (key) { return order.indexOf(key) >= 0; } };
  // On /chat?k=…, this chat is the page.
  var pageKey = page ? new URLSearchParams(location.search).get("k") || "" : "";
  if (page) need().then(function () { return pageKey ? build(pageKey, true) : null; }).then(function (w) {
    if (!w) document.getElementById("chat-missing").hidden = false;
    else { var inp = w.querySelector(".chat-form input"); if (inp) inp.focus({ preventScroll: true }); }
  });
  // The chats you had open come back on this page (desktop only).
  var was = desktop() ? saved().filter(function (k) { return k !== pageKey; }) : [];
  was.reduce(function (p, k) { return p.then(function () { return open(k, false); }); }, Promise.resolve()).catch(function () {});
})();
