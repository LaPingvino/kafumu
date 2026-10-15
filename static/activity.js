// Activity (77c): one list of what came back to you, from any area —
// replies and reactions to your posts, private answers, messages and
// signals from contacts, your public inbox. Assembled here, on the device,
// from what it holds plus anonymous fetches of replies by post id; the
// server never learns which posts are yours. "Seen" is kept here too.
(function () {
  var dev = window.kafumuDevice;
  if (!dev) return;
  var T = window.KAFUMU_T || {};
  function tr(k, v) { var s = T[k] || k; for (var n in v || {}) s = s.replace("{" + n + "}", v[n]); return s; }
  function short(s, n) { s = String(s || "").replace(/\s+/g, " ").trim(); return s.length > n ? s.slice(0, n - 1) + "…" : s; }

  // areaLink: the area to open for a reply, its own or else the post's; a
  // reaction without a place ("000000") has neither: Around as it is.
  function areaLink() {
    for (var i = 0; i < arguments.length; i++) if (/^[23456789cfghjmpqrvwx]{6}$/.test(arguments[i] || "")) return "/?cell=" + arguments[i];
    return "/";
  }
  // replies fetches the public replies to your live posts, 20 ids a call.
  function replies(posts) {
    var byRe = {}, mine = {}, ids = [];
    posts.forEach(function (p) { if (p.reid) { byRe[p.reid] = p; ids.push(p.reid); } if (p.id) mine[p.id] = true; });
    var calls = [];
    for (var i = 0; i < ids.length; i += 20) {
      calls.push(fetch("/api/oln/re?ids=" + ids.slice(i, i + 20).join(","), { credentials: "omit" })
        .then(function (r) { return r.ok ? r.json() : []; }, function () { return []; }));
    }
    return Promise.all(calls).then(function (lists) {
      var out = [];
      lists.forEach(function (ns) {
        ns.forEach(function (n) {
          var re = ((n.tags || []).filter(function (t) { return /^re[0-9a-f]{10}$/.test(t); })[0] || "").slice(2), p = byRe[re];
          if (!p || mine[n.id]) return; // your own reply to your own post isn't news
          out.push({ id: "r:" + n.id, at: n.at, icon: /^\s*(\p{Extended_Pictographic}️?\s*){1,3}$/u.test(n.text) ? n.text.trim() : "💬",
            who: n.author ? "@" + n.author : (n.biz || ""), text: String(n.text).split("\n")[0], about: tr("act_reply_to", { post: short(p.text, 40) }),
            href: areaLink(p.cell, n.cell) + "#re-" + p.reid, thread: "p:" + p.reid, kind: "post", title: short(p.text, 60) }); // the post's area, scrolled to the post
        });
      });
      return out;
    });
  }

  // meetups: new RSVPs to the ones you host; a changed time or place, or a
  // cancellation, for the ones you're going to (77d).
  function meetups(mine) {
    var ids = mine.map(function (m) { return m.id; }), calls = [];
    for (var i = 0; i < ids.length; i += 20) {
      calls.push(fetch("/api/meetups?ids=" + ids.slice(i, i + 20).map(encodeURIComponent).join(","), { credentials: "omit" })
        .then(function (r) { return r.ok ? r.json() : null; }, function () { return null; }));
    }
    return Promise.all(calls).then(function (rs) {
      var now = {}, gone = {}, out = [];
      rs.forEach(function (r) { if (!r) return; r.meetups.forEach(function (m) { now[m.id] = m; }); r.gone.forEach(function (id) { gone[id] = true; }); });
      mine.forEach(function (was) {
        var m = now[was.id], href = "/meetups/" + was.id, title = short(was.title, 50);
        if (was.role === "host" && m && m.going > was.going)
          out.push({ thread: "e:" + was.id, kind: "meetup", title: title, id: "g:" + was.id + ":" + m.going, icon: "🙋", who: "", text: tr("act_going", { n: m.going }), about: title, href: href });
        if (was.role === "going" && m && (new Date(m.start).getTime() !== new Date(was.start).getTime() || (m.venue || "") !== (was.venue || "")))
          out.push({ thread: "e:" + was.id, kind: "meetup", title: title, id: "u:" + was.id + ":" + m.start + "|" + (m.venue || ""), icon: "✏️", who: "", about: title, href: href,
            text: tr("act_changed", { when: new Date(m.start).toLocaleString(window.KAFUMU_LOCALE, { weekday: "short", day: "numeric", month: "short", hour: "2-digit", minute: "2-digit" }) + (m.venue ? " · " + m.venue : "") }) });
        if (was.role === "going" && gone[was.id] && new Date(was.end) > Date.now())
          out.push({ thread: "e:" + was.id, kind: "meetup", title: title, id: "x:" + was.id, icon: "❌", who: "", text: tr("act_cancelled"), about: title, href: "/" });
      });
      return out;
    });
  }

  // Bluesky (77e): likes, reposts and replies on your Bluesky posts, asked
  // of the public AppView by this device (the server isn't involved).
  var APPVIEWS = ["https://public.api.bsky.app", "https://api.bsky.app"];
  function xrpc(path) { // the AppView that answered last goes first
    var good = "";
    try { good = localStorage.getItem("kafumu.appview") || ""; } catch (e) {}
    var hosts = APPVIEWS.indexOf(good) >= 0 ? [good].concat(APPVIEWS.filter(function (h) { return h !== good; })) : APPVIEWS;
    return hosts.reduce(function (p, h) {
      return p.catch(function () {
        return fetch(h + "/xrpc/" + path, { credentials: "omit" }).then(function (r) {
          if (!r.ok) throw new Error("appview " + r.status);
          try { localStorage.setItem("kafumu.appview", h); } catch (e) {}
          return r.json();
        });
      });
    }, Promise.reject(new Error("start")));
  }
  function bskyLink(uri, handle) { return "https://bsky.app/profile/" + handle + "/post/" + uri.split("/").pop(); }
  function bluesky(posts) {
    var bs = posts.filter(function (p) { return p.kind === "bsky" && p.uri; }).slice(-25);
    if (!bs.length) return Promise.resolve([]);
    return xrpc("app.bsky.feed.getPosts?" + bs.map(function (p) { return "uris=" + encodeURIComponent(p.uri); }).join("&")).then(function (r) {
      var out = [], threads = [];
      (r.posts || []).forEach(function (v) {
        var p = bs.filter(function (x) { return x.uri === v.uri; })[0] || {}, about = tr("act_reply_to", { post: short(p.text || (v.record && v.record.text), 40) }), href = bskyLink(v.uri, v.author.handle);
        if (v.likeCount) out.push({ thread: "b:" + v.uri, kind: "bsky", title: about, id: "bl:" + v.uri + ":" + v.likeCount, icon: "❤️", who: "", text: tr("act_likes", { n: v.likeCount }), about: about, href: href });
        if (v.repostCount) out.push({ thread: "b:" + v.uri, kind: "bsky", title: about, id: "bp:" + v.uri + ":" + v.repostCount, icon: "🔁", who: "", text: tr("act_reposts", { n: v.repostCount }), about: about, href: href });
        if (v.replyCount) threads.push(xrpc("app.bsky.feed.getPostThread?depth=1&parentHeight=0&uri=" + encodeURIComponent(v.uri)).then(function (t) {
          return ((t.thread && t.thread.replies) || []).filter(function (x) { return x.post && x.post.author.did !== v.author.did; }).map(function (x) {
            return { thread: "b:" + v.uri, kind: "bsky", title: about, id: "br:" + x.post.uri, at: (x.post.record && x.post.record.createdAt) || x.post.indexedAt, icon: "🦋", who: "@" + x.post.author.handle,
              text: (x.post.record && x.post.record.text) || "", about: about, href: bskyLink(x.post.uri, x.post.author.handle) };
          }).sort(function (a, b) { return new Date(b.at) - new Date(a.at); }).slice(0, 20); // a popular post: its newest 20
        }, function () { return []; }));
      });
      return Promise.all(threads).then(function (ts) { return out.concat.apply(out, ts); });
    }, function () { return []; });
  }

  // gather returns every item, newest first, each with .unread.
  function gather() {
    var s = dev.store;
    return Promise.all([s.get("myPosts"), s.get("answers"), s.contacts(), s.get("inboxMsgs"), s.get("activity:seen"), s.get("myMeetups"), s.get("activity:first")]).then(function (r) {
      var now = Date.now(), posts = (r[0] || []).filter(function (p) { return p.until > now; }), items = [];
      (r[1] || []).forEach(function (t) { // private answers, both ways
        var last = (t.messages || []).filter(function (m) { return !m.me; }).slice(-1)[0];
        if (last) items.push({ id: "a:" + t.id + ":" + (t.messages || []).length, at: last.at, icon: "🔒", who: "",
          text: last.text, about: tr("act_answer_to", { post: short(t.post && t.post.text, 40) }), href: "/chat?k=a:" + encodeURIComponent(t.id), hot: !!t.unreadMsgs });
      });
      r[2].forEach(function (c) { // contacts: new through your code, their last message, their last signal
        var name = dev.nameOf(c, T);
        if (c.role === 0 && c.createdAt && now - new Date(c.createdAt) < 30 * 864e5) // they came to your code (coffee, a question, Connect)
          items.push({ id: "c:" + c.id, at: c.createdAt, icon: "🤝", who: name, text: tr("act_connected"), about: "", href: "/contacts#c-" + encodeURIComponent(c.id) });
        var last = (c.messages || []).filter(function (m) { return !m.me; }).slice(-1)[0];
        if (last) items.push({ id: "m:" + c.id + ":" + last.at, at: last.at, icon: "💬", who: name, text: last.text,
          about: tr("act_message"), href: "/chat?k=c:" + encodeURIComponent(c.id), hot: !!c.unreadMsgs });
        var sig = (c.signals || [])[0];
        if (sig) items.push({ id: "s:" + c.id + ":" + sig.at, at: sig.at, icon: "📶", who: name, text: dev.signalText(sig, T),
          about: "", href: "/contacts#c-" + encodeURIComponent(c.id), hot: !!sig.unread });
      });
      (r[3] || []).forEach(function (m) {
        items.push({ id: "i:" + m.id, at: m.at, icon: "📥", who: (m.card && m.card.name) || tr("inbox_anonymous"), text: m.text,
          about: tr("act_inbox"), href: "/contacts#inbox-section" });
      });
      var seen = r[4] || {};
      var mine = (r[5] || []).filter(function (m) { return new Date(m.end) > Date.now() - 864e5; }), first = r[6] || {};
      return Promise.all([replies(posts), meetups(mine), bluesky(posts)]).then(function (got) {
        // News without a time of its own (a count went up) is dated when the device first saw it.
        var dated = false;
        got[0].concat(got[1], got[2]).forEach(function (it) {
          if (!it.at) { it.at = first[it.id] || (first[it.id] = new Date().toISOString()); dated = true; }
          items.push(it);
        });
        if (dated) s.set("activity:first", first).catch(function () {});
        items.forEach(function (it) { it.unread = !seen[it.id] || !!it.hot; });
        items.sort(function (a, b) { return new Date(b.at) - new Date(a.at); });
        return items;
      });
    });
  }

  // badge shows the unread count on the 🔔 (and remembers it for pages
  // that don't gather), plus on the app icon where the browser allows.
  function badge(n) {
    try { localStorage.setItem("kafumu.activityN", String(n)); } catch (e) {}
    document.querySelectorAll(".bell-n").forEach(function (b) { b.textContent = n > 99 ? "99+" : String(n); b.hidden = !n; });
    if (navigator.setAppBadge) (n ? navigator.setAppBadge(n) : navigator.clearAppBadge()).catch(function () {});
  }
  function count() { return gather().then(function (items) { var n = items.filter(function (i) { return i.unread; }).length; badge(n); return items; }); }

  // markSeen: opening Activity has shown you everything in it.
  function markSeen(items) {
    return dev.store.get("activity:seen").then(function (seen) {
      seen = seen || {};
      items.forEach(function (i) { seen[i.id] = Date.now(); });
      var keep = {}, cut = Date.now() - 60 * 864e5; // forget marks after two months
      Object.keys(seen).forEach(function (k) { if (seen[k] > cut) keep[k] = seen[k]; });
      return dev.store.set("activity:seen", keep);
    });
  }

  // mailbox (85b): every conversation as one chat list (static/threads.js):
  // contacts (always), private answers, and one thread per post, meetup or
  // Bluesky post of yours collecting what happened to it. A row opens its
  // chat in place: two-way ones to answer, one-way ones to read.
  function mailbox(box, pair, items) {
    var ul = box.querySelector("#threads"), filter = "all", linked = false, open = {};
    function post(t, kw, b) { return window.kafumuOLN.post(t, kw, b, function () {}); }
    function sendContact(c, msg) { return msg.t === "msg" ? pair.sendChat(c, msg.text, post) : pair.send(c, msg); }
    function checkContact(c) { return pair.checkContact(c).then(function (got) { return pair.readChat(c).then(function (n) { return n ? got.concat([{ t: "msg" }]) : got; }); }); }
    function when(at) { return at ? new Date(at).toLocaleString(window.KAFUMU_LOCALE, { day: "numeric", month: "short", hour: "2-digit", minute: "2-digit" }) : ""; }
    function el(tag, cls, text) { var e = document.createElement(tag); if (cls) e.className = cls; if (text != null) e.textContent = text; return e; }
    function oneWay(t) { // what happened to one thing of yours, as bubbles, oldest first
      var log = el("div", "chat-log");
      t.items.forEach(function (it) {
        var b = el("div", "bubble", (it.icon ? it.icon + " " : "") + (it.who ? it.who + ": " : "") + String(it.text || "").split("\n").filter(function (l) { return !/^https?:\/\/\S+$/.test(l.trim()); }).join(" · "));
        b.title = new Date(it.at).toLocaleString(window.KAFUMU_LOCALE);
        log.appendChild(b);
      });
      var go = el("a", "pill-sm", (T.act_open || "Open") + " ↗"); go.href = t.url; go.setAttribute("role", "button");
      var wrap = el("div", "chat"); wrap.appendChild(log); wrap.appendChild(go);
      return wrap;
    }
    // Group boards (97e): each board a thread here, read with the group's key
    // on this device; unread = newer than your last look at that board.
    var boardItems = [];
    function readBoards() {
      if (!window.kafumuBoard) return Promise.resolve();
      return Promise.all([dev.store.get("groupKeys"), dev.store.get("groupNames"), dev.store.get("boardSeen")]).then(function (r) {
        var keys = r[0] || {}, names = r[1] || {}, seen = r[2] || {};
        return Promise.all(Object.keys(keys).map(function (g) {
          return window.kafumuBoard.read(keys[g]).then(function (posts) {
            return posts.map(function (p) {
              return { thread: "g:" + g, kind: "group", title: "📋 " + (names[g] || g), href: "/groups?g=" + encodeURIComponent(g) + "#g-" + encodeURIComponent(g),
                icon: p.kind === "offer" ? "🎁" : p.kind === "request" ? "🙋" : p.kind === "reply" ? "💬" : "📝", who: p.by ? "@" + p.by : "", text: p.text, at: p.at,
                unread: !seen[g] || String(p.at) > seen[g] };
            });
          }, function () { return []; });
        })).then(function (lists) { boardItems = [].concat.apply([], lists); });
      }, function () {});
    }
    function draw() {
      return Promise.all([dev.store.contacts(), pair.threads(), dev.store.get("inboxMsgs")]).then(function (r) {
        var ts = window.kafumuThreads.build({ contacts: r[0], answers: r[1], inbox: r[2] || [], notices: items.filter(function (i) { return i.thread; }).concat(boardItems),
          nameOf: dev.nameOf, signalText: dev.signalText }, T);
        var byId = {};
        r[0].forEach(function (c) { byId["c:" + c.id] = c; });
        r[1].forEach(function (a) { byId["a:" + a.id] = a; });
        box.querySelectorAll("#thread-filter .chip[data-f]").forEach(function (b) { b.classList.toggle("on", b.dataset.f === filter); b.setAttribute("aria-pressed", b.dataset.f === filter); });
        ul.textContent = "";
        // The Contacts filter leads to their cards too (91g: one "Contacts", not two).
        if (filter === "contacts") {
          var go = el("li", "thread-cards"), a = el("a", "pill-sm", "🪪 " + (T.act_contact_cards || "Their cards, your notes, backup") + " →");
          a.href = "/contacts"; a.setAttribute("role", "button"); go.appendChild(a); ul.appendChild(go);
        }
        ts.filter(function (t) { return filter === "all" || (filter === "contacts" ? t.kind === "contact" : t.unread > 0); }).forEach(function (t) {
          var src = byId[t.id], li = el("li", "thread" + (t.unread ? " unread" : ""));
          li.id = t.kind === "contact" ? "chat-" + src.id : t.kind === "answer" ? "answer-" + src.id : "t-" + t.id;
          var head = el("button", "thread-head"); head.type = "button";
          var last = t.items[t.items.length - 1];
          head.appendChild(el("strong", "", t.title));
          if (t.unread) head.appendChild(el("span", "badge new-count", String(t.unread)));
          head.appendChild(el("span", "dim small thread-when", when(t.lastAt)));
          var prev = last ? (last.me ? "→ " : (last.icon ? last.icon + " " : "")) + String(last.text || "").split("\n")[0] : (T.chat_empty || "");
          head.appendChild(el("span", "dim small thread-prev", short(prev, 90)));
          li.appendChild(head);
          var body;
          // Two-way threads open as chat windows (chatdock.js, 88a); one-way
          // ones unfold in the list.
          var key = t.kind === "contact" ? "c:" + src.id : t.kind === "answer" ? "a:" + src.id : "";
          function openIt() {
            if (key) { // chatdock.js loads deferred: wait for it on a fresh page
              if (window.kafumuChat) window.kafumuChat.open(key);
              else window.addEventListener("load", function () { if (window.kafumuChat) window.kafumuChat.open(key); });
              return;
            }
            if (!body) { body = el("div", "thread-body"); body.appendChild(oneWay(t)); li.appendChild(body); }
            else body.hidden = !body.hidden;
            if (body.hidden) delete open[t.id]; else open[t.id] = true;
          }
          head.onclick = openIt;
          ul.appendChild(li);
          if (open[t.id] && !key) openIt();
          var hash = decodeURIComponent(location.hash);
          if (!linked && (hash === "#" + li.id || (hash === "#answers" && t.kind === "answer" && t.unread))) {
            linked = true;
            setTimeout(function () { if (key || !body || body.hidden) openIt(); li.scrollIntoView({ block: "center", behavior: "smooth" }); li.classList.add("flash"); setTimeout(function () { li.classList.remove("flash"); }, 2500); }, 0);
          }
        });
        document.getElementById("activity-empty").hidden = ts.length > 0;
      });
    }
    box.querySelectorAll("#thread-filter .chip[data-f]").forEach(function (b) { b.onclick = function () { filter = b.dataset.f; open = {}; draw(); }; });
    window.addEventListener("hashchange", function () { linked = false; draw(); });
    // Show what's stored at once, then read new lines of the ten most recent
    // chats (one anonymous fetch each) and redraw if any came in.
    readBoards().then(function () { if (boardItems.length) draw(); }); // the boards come in after the first draw
    return draw().then(function () { return dev.store.contacts(); }).then(function (cs) {
      cs = cs.filter(function (c) { return (c.messages || []).length; }).sort(function (a, b) { return ((b.messages.slice(-1)[0] || {}).at || "").localeCompare((a.messages.slice(-1)[0] || {}).at || ""); }).slice(0, 10);
      return Promise.all(cs.map(function (c) { return pair.readChat(c).catch(function () { return 0; }); }));
    }).then(function (ns) { if (ns.some(Boolean) && !Object.keys(open).length) return draw(); });
  }

  window.kafumuActivity = { gather: gather, count: count, badge: badge };

  var mbox = document.getElementById("mailbox");
  if (mbox) { // the mailbox page: read new private answers first, then show, then count it all as seen
    var aPair = window.kafumuPair && window.kafumuPair.create({ fetch: window.fetch.bind(window), store: dev.store, origin: location.origin, mine: function (t, kw, b) { return window.kafumuOLN.post(t, kw, b, function () {}); } });
    var fresh = aPair ? aPair.readAnswers().catch(function () {}) : Promise.resolve();
    if (aPair) aPair.pushSubscribe(false).catch(function () {}); // keep what wakes you up to date
    fresh.then(count).then(function (items) {
      return mailbox(mbox, aPair, items).then(function () {
        return markSeen(items).then(function () { badge(items.filter(function (i) { return i.hot; }).length); });
      });
    }).catch(function (e) { document.getElementById("activity-empty").hidden = false; if (window.console) console.error(e); });
  } else { // elsewhere: keep the bell's count fresh
    var tick = function () { if (!document.hidden) count().catch(function () {}); };
    setTimeout(tick, 4000);
    setInterval(tick, 5 * 60000);
  }
})();
