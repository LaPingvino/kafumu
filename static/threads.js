// Every conversation as a chat (85a): one list of threads, built on the
// device from what it holds. A thread is two-way when you can answer in it
// (a contact, a private answer) and one-way when it only collects what
// happened to one thing of yours (a post's replies, a meetup's RSVPs, your
// public inbox). Contacts are always in it, also before any message.
//
// build(src, T) → [{ id, kind, title, twoWay, url, unread, lastAt, items }]
// with items [{ me, text, at, icon }] oldest first, threads newest first
// (contacts without anything yet last, newest connection first).
(function (root) {
  "use strict";
  function short(s, n) { s = String(s || "").replace(/\s+/g, " ").trim(); return s.length > n ? s.slice(0, n - 1) + "…" : s; }
  function time(x) { var t = new Date(x || 0).getTime(); return isNaN(t) ? 0 : t; }

  function build(src, T) {
    T = T || {};
    var nameOf = src.nameOf || function (c) { return (c.alias || "").trim() || (c.card && c.card.name) || T.unnamed || "…"; };
    var threads = [];
    (src.contacts || []).forEach(function (c) {
      var items = [];
      if (c.createdAt) items.push({ me: false, icon: "🤝", text: T.act_connected || "Connected", at: c.createdAt, note: true });
      (c.messages || []).forEach(function (m) { items.push({ me: !!m.me, icon: "", text: m.text, at: m.at }); });
      (c.signals || []).forEach(function (s) { items.push({ me: false, icon: "📶", text: src.signalText ? src.signalText(s, T) : s.s, at: s.at }); });
      var unread = (c.unreadMsgs || 0) + (c.signals || []).filter(function (s) { return s.unread; }).length;
      threads.push({ id: "c:" + c.id, kind: "contact", title: nameOf(c, T), twoWay: true, url: "/activity#chat-" + encodeURIComponent(c.id), unread: unread, items: items,
        quiet: !(c.messages || []).length && !(c.signals || []).length });
    });
    (src.answers || []).forEach(function (t) {
      threads.push({ id: "a:" + t.id, kind: "answer", title: "🔒 " + short(t.post && t.post.text, 60), twoWay: true, url: "/activity#answer-" + encodeURIComponent(t.id),
        unread: t.unreadMsgs || 0, items: (t.messages || []).map(function (m) { return { me: !!m.me, icon: "", text: m.text, at: m.at }; }) });
    });
    if ((src.inbox || []).length) {
      threads.push({ id: "i:inbox", kind: "inbox", title: "📥 " + (T.act_inbox || "Public inbox"), twoWay: false, url: "/contacts#inbox-section",
        unread: src.inbox.filter(function (m) { return m.unread; }).length,
        items: src.inbox.map(function (m) { return { me: false, icon: "", who: (m.card && m.card.name) || T.inbox_anonymous || "", text: m.text, at: m.at }; }) });
    }
    // Notices (replies to your posts, meetup news, Bluesky) grouped by the thing they're about.
    var byThing = {};
    (src.notices || []).forEach(function (n) {
      if (!n.thread) return;
      var t = byThing[n.thread];
      if (!t) t = byThing[n.thread] = { id: n.thread, kind: n.kind || "notice", title: n.title || n.about || "", twoWay: false, url: n.href, unread: 0, items: [] };
      t.items.push({ me: false, icon: n.icon, who: n.who || "", text: n.text, at: n.at });
      if (n.unread) t.unread++;
    });
    Object.keys(byThing).forEach(function (k) { threads.push(byThing[k]); });
    threads.forEach(function (t) {
      t.items.sort(function (a, b) { return time(a.at) - time(b.at); });
      var last = t.items[t.items.length - 1];
      t.lastAt = last ? last.at : "";
    });
    threads.sort(function (a, b) {
      if (!!a.quiet !== !!b.quiet) return a.quiet ? 1 : -1;
      return time(b.lastAt) - time(a.lastAt);
    });
    return threads;
  }

  root.kafumuThreads = { build: build };
})(typeof window !== "undefined" ? window : globalThis);
