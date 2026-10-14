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
  // titles of public threads (93a), kept on this device: "r:<rid>" → what it's about.
  // seeds: a thread's lines the page already had, handed over through
  // sessionStorage (on a phone the chat is another page).
  function seedOf(rid) { try { return (JSON.parse(sessionStorage.getItem("kafumu.threadSeeds") || "{}"))[rid] || []; } catch (e) { return []; } }
  function threadTitle(rid) { try { return (JSON.parse(localStorage.getItem("kafumu.threadTitles") || "{}"))[rid] || ""; } catch (e) { return ""; } }
  // replyThread (93a): a post's, meetup's or event's public replies (#re) as
  // a group chat: oldest first, refreshed while open; what you write is a
  // reply (as you unless you post anonymously).
  function replyThread(rid) {
    var thread = el("div", "chat"), log = el("div", "chat-log"), form = el("form", "chat-form"), input = el("input"), st = el("span", "dim small");
    input.maxLength = 500; input.placeholder = T.reply_placeholder || "Reply…"; input.setAttribute("aria-label", input.placeholder);
    var sendBtn = el("button", "pill-sm suggested", T.chat_send || "Send"); sendBtn.type = "submit";
    form.appendChild(input); form.appendChild(sendBtn); form.appendChild(st);
    thread.appendChild(log); thread.appendChild(form);
    var shown = "";
    function draw(ns) {
      var sig = ns.map(function (n) { return n.id; }).join(",");
      if (sig === shown) return;
      shown = sig;
      var atEnd = log.scrollHeight - log.scrollTop - log.clientHeight < 40;
      log.textContent = "";
      if (!ns.length) log.appendChild(el("p", "dim small", T.chat_empty || "No messages yet."));
      ns.forEach(function (n) {
        var text = String(n.text || "").split("\n")[0];
        if (/^\s*(\p{Extended_Pictographic}|\u200d|\ufe0f|\s)+\s*$/u.test(text)) { // a reaction: a small note in between
          log.appendChild(el("div", "chat-note dim small", text.trim() + (n.author ? " · @" + n.author : "")));
          return;
        }
        var b = el("div", "bubble" + (n.mine ? " me" : ""), "");
        if (n.author) b.appendChild(el("div", "dim small", "@" + n.author));
        b.appendChild(document.createTextNode(text));
        b.title = new Date(n.at).toLocaleString(window.KAFUMU_LOCALE);
        log.appendChild(b);
      });
      if (atEnd || !log.dataset.seen) log.scrollTop = log.scrollHeight;
      log.dataset.seen = "1";
    }
    var mine = seedOf(rid); // what the page already showed (the server's list may lag a minute)
    function load() {
      return fetch("/api/oln/re?ids=" + rid, { credentials: "omit", cache: "no-store" }).then(function (r) { return r.ok ? r.json() : []; }, function () { return []; }).then(function (ns) {
        var ids = {};
        ns.forEach(function (n) { ids[n.id] = true; });
        mine = mine.filter(function (n) { return !ids[n.id]; });
        draw(ns.concat(mine).sort(function (a, z) { return new Date(a.at) - new Date(z.at); }));
      });
    }
    form.onsubmit = function (e) {
      e.preventDefault();
      var text = input.value.trim();
      if (!text) return;
      sendBtn.disabled = input.disabled = true; st.textContent = "…";
      var asMe = localStorage.getItem("kafumu.postAsMe") !== "0";
      window.kafumuOLN.post(text, "#re" + rid, 4, function (n) { st.textContent = (T.oln_working || "{n}").replace("{n}", n); }, 0, asMe).then(function (n) {
        n.mine = true; mine.push(n); input.value = ""; st.textContent = ""; return load();
      }, function () { st.textContent = T.oln_failed || "!"; }).then(function () { sendBtn.disabled = input.disabled = false; input.focus(); });
    };
    load();
    var timer = setInterval(function () { if (!document.hidden && thread.isConnected) load(); else if (!thread.isConnected) clearInterval(timer); }, 20000);
    return { thread: thread, open: function () {} };
  }
  function build(key, asPage) {
    var dev = window.kafumuDevice, kind = key.slice(0, 2), id = key.slice(2), p = pair();
    if (kind === "r:") return /^[0-9a-f]{10}$/.test(id) ? Promise.resolve(win(key, asPage, "💬 " + (threadTitle(id) || "#re" + id), replyThread(id))) : Promise.resolve(null);
    var find = kind === "c:" ? dev.store.contacts() : p.threads();
    return find.then(function (all) {
      var src = all.filter(function (x) { return x.id === id; })[0];
      if (!src) return null;
      var title = kind === "c:" ? dev.nameOf(src, T) : "🔒 " + String((src.post && src.post.text) || "").slice(0, 60);
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
      return win(key, asPage, title, chat);
    });
  }
  function win(key, asPage, title, chat) {
    var kind = key.slice(0, 2), id = key.slice(2);
    {
      var w = el("div", "chat-window"); w.dataset.thread = key;
      w.setAttribute("role", "dialog");
      w.setAttribute("aria-label", title);
      var head = el("div", "win-head"), back;
      if (asPage && kind === "r:") { back = el("a", "win-back", "←"); back.href = "/"; back.onclick = function (e) { if (history.length > 1) { e.preventDefault(); history.back(); } }; back.setAttribute("aria-label", T.close || "Back"); }
      else if (asPage) { back = el("a", "win-back", "← 🔔"); back.href = "/activity"; back.setAttribute("aria-label", T.back_to_mailbox || "Back"); }
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
      w.appendChild(chat.thread);
      chat.thread.querySelectorAll(".close-chat").forEach(function (x) { x.hidden = true; }); // ← closes the window
      if (asPage) {
        w.classList.add("chat-page"); page.appendChild(w); chat.open();
        // A fixed screen, like any chat app: the header on top, the input at
        // the bottom, only the messages scroll. With the keyboard open the
        // input sits right above it (visualViewport), the tab bar out of the way.
        document.body.classList.add("chat-fullpage");
        var log = w.querySelector(".chat-log");
        var fit = function () {
          var head = document.querySelector(".headerbar"), nav = document.querySelector(".switcher.bottom");
          var vv = window.visualViewport, vh = vv ? vv.height : window.innerHeight, off = vv ? vv.offsetTop : 0;
          var keyboard = window.innerHeight - vh > 120;
          var top = head ? Math.max(0, head.getBoundingClientRect().bottom) : 0;
          var below = !keyboard && nav && getComputedStyle(nav).display !== "none" ? nav.offsetHeight : 0;
          w.style.top = (top + off) + "px";
          w.style.height = Math.max(160, vh - top - below) + "px";
          document.body.classList.toggle("keyboard-open", keyboard);
          if (log) log.scrollTop = log.scrollHeight; // the newest stays in view
        };
        fit();
        window.addEventListener("resize", fit);
        if (window.visualViewport) { window.visualViewport.addEventListener("resize", fit); window.visualViewport.addEventListener("scroll", fit); }
        return w;
      }
      w.addEventListener("keydown", function (e) { if (e.key === "Escape") close(key); });
      dockEl().appendChild(w);
      chat.open();
      return w;
    }
  }
  // open shows the chat for key ("c:<contact id>", "a:<answer thread id>" or
  // "r:<reply id>", a public thread; title: what it's about, remembered).
  function open(key, focus, title, seed) {
    if (seed && key.slice(0, 2) === "r:") try {
      var ss = {}; ss[key.slice(2)] = seed.slice(-50).map(function (n) { return { id: n.id, text: n.text, at: n.at, author: n.author, mine: n.mine }; });
      sessionStorage.setItem("kafumu.threadSeeds", JSON.stringify(ss));
    } catch (e) {}
    if (title && key.slice(0, 2) === "r:") {
      try { var ts = JSON.parse(localStorage.getItem("kafumu.threadTitles") || "{}"); ts[key.slice(2)] = String(title).slice(0, 60); var ks = Object.keys(ts); if (ks.length > 200) delete ts[ks[0]]; localStorage.setItem("kafumu.threadTitles", JSON.stringify(ts)); } catch (e) {}
    }
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
