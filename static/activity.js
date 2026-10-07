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
            who: n.author ? "@" + n.author : (n.biz || ""), text: n.text, about: tr("act_reply_to", { post: short(p.text, 40) }),
            href: "/?cell=" + (n.cell || p.cell) });
        });
      });
      return out;
    });
  }

  // gather returns every item, newest first, each with .unread.
  function gather() {
    var s = dev.store;
    return Promise.all([s.get("myPosts"), s.get("answers"), s.contacts(), s.get("inboxMsgs"), s.get("activity:seen")]).then(function (r) {
      var now = Date.now(), posts = (r[0] || []).filter(function (p) { return p.until > now; }), items = [];
      (r[1] || []).forEach(function (t) { // private answers, both ways
        var last = (t.messages || []).filter(function (m) { return !m.me; }).slice(-1)[0];
        if (last) items.push({ id: "a:" + t.id + ":" + (t.messages || []).length, at: last.at, icon: "🔒", who: "",
          text: last.text, about: tr("act_answer_to", { post: short(t.post && t.post.text, 40) }), href: "/#answers", hot: !!t.unreadMsgs });
      });
      r[2].forEach(function (c) { // contacts: their last message, their last signal
        var name = (c.card && c.card.name) || tr("inbox_anonymous");
        var last = (c.messages || []).filter(function (m) { return !m.me; }).slice(-1)[0];
        if (last) items.push({ id: "m:" + c.id + ":" + last.at, at: last.at, icon: "💬", who: name, text: last.text,
          about: tr("act_message"), href: "/contacts", hot: !!c.unreadMsgs });
        var sig = (c.signals || [])[0];
        if (sig) items.push({ id: "s:" + c.id + ":" + sig.at, at: sig.at, icon: "📶", who: name, text: dev.signalText(sig, T),
          about: "", href: "/contacts", hot: !!sig.unread });
      });
      (r[3] || []).forEach(function (m) {
        items.push({ id: "i:" + m.id, at: m.at, icon: "📥", who: (m.card && m.card.name) || tr("inbox_anonymous"), text: m.text,
          about: tr("act_inbox"), href: "/contacts#inbox-section" });
      });
      var seen = r[4] || {};
      return replies(posts).then(function (rs) {
        items = items.concat(rs);
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

  function render(ul, items) {
    ul.textContent = "";
    items.forEach(function (it) {
      var li = document.createElement("li");
      if (it.unread) li.className = "unread";
      var a = document.createElement("a");
      a.href = it.href;
      var icon = document.createElement("span"); icon.className = "act-icon"; icon.textContent = it.icon; icon.setAttribute("aria-hidden", "true");
      var body = document.createElement("span"); body.className = "act-body";
      var head = document.createElement("span"); head.className = "dim small";
      head.textContent = [it.who, it.about, new Date(it.at).toLocaleString(window.KAFUMU_LOCALE, { day: "numeric", month: "short", hour: "2-digit", minute: "2-digit" })].filter(Boolean).join(" · ");
      var text = document.createElement("span"); text.className = "act-text"; text.textContent = short(it.text, 200);
      body.appendChild(head); body.appendChild(text);
      a.appendChild(icon); a.appendChild(body);
      li.appendChild(a);
      ul.appendChild(li);
    });
  }

  window.kafumuActivity = { gather: gather, count: count, badge: badge };

  var ul = document.getElementById("activity");
  if (ul) { // the Activity page: show, then count it all as seen
    count().then(function (items) {
      render(ul, items);
      document.getElementById("activity-empty").hidden = items.length > 0;
      return markSeen(items).then(function () { badge(items.filter(function (i) { return i.hot; }).length); });
    }).catch(function () { document.getElementById("activity-empty").hidden = false; });
  } else { // elsewhere: keep the bell's count fresh
    var tick = function () { if (!document.hidden) count().catch(function () {}); };
    setTimeout(tick, 4000);
    setInterval(tick, 5 * 60000);
  }
})();
