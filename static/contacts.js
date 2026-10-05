// /contacts: everyone you've connected with, kept on this device.
(function () {
  "use strict";
  var T = window.KAFUMU_T || {}, dev = window.kafumuDevice;
  var pair = window.kafumuPair.create({ fetch: window.fetch.bind(window), store: dev.store, origin: location.origin, mine: function (t, kw, b) { return window.kafumuOLN ? window.kafumuOLN.post(t, kw, b, function () {}) : Promise.reject(new Error("no miner")); } });
  // Anything that waited for a network goes out now, and when it's back.
  pair.flush().catch(function () {});
  window.addEventListener("online", function () { pair.flush().catch(function () {}); });
  var $ = function (id) { return document.getElementById(id); };
  var list = $("contacts"), empty = $("contacts-empty"), filter = $("contacts-filter");

  // Sync status: on, or this device still needs the key (one move away).
  function syncState(s) {
    $("sync-on").hidden = s !== "on";
    $("sync-needs-key").hidden = s !== "needs-key";
  }
  window.addEventListener("kafumu:sync", function (e) { syncState(e.detail); });
  window.addEventListener("kafumu:synced", function () { show(); });
  if (window.kafumuSync) syncState(window.kafumuSync);
  // "Sync now", and arriving with #get (from the bar on another page)
  // starts fetching from the other device right away.
  var now = $("sync-now");
  if (now) now.onclick = function () {
    now.disabled = true; now.textContent = "…";
    dev.sync().then(function (s) { now.disabled = false; now.textContent = s === "on" ? "✓" : (T.sync_now || "Sync now"); syncState(s); show(); });
  };
  if (location.hash === "#get") setTimeout(function () { var b = $("move-start"); if (b && !b.hidden) { b.scrollIntoView({ block: "center" }); b.click(); } }, 400);

  // Someone may have scanned or followed your code while the app was
  // closed: their hello waits in the mailbox (7 days). Take it in here too,
  // not only on Connect, and answer with your card.
  function takeInvites() {
    return dev.personas.shareCard().then(function (card) {
      return Promise.all([pair.checkInvite(card), pair.checkInvite(card, "badge"), dev.personas.linkCard().then(function (lc) { return pair.checkInvite(lc, "named"); })]);
    }).then(function (r) { return r[0].length + r[1].length + r[2].length; }).catch(function () { return 0; });
  }

  function show() {
    dev.store.contacts().then(function (cs) {
      cs.sort(function (a, b) { return (b.createdAt || "").localeCompare(a.createdAt || ""); });
      if (sortBy() === "near") cs = byDistance(cs);
      // Quiet for 90+ days: only those, most silent first, with "remove all".
      var quietMode = sortBy() === "quiet";
      if (quietMode) cs = cs.filter(function (c) { return dev.quietDays(c) >= 90; }).sort(function (a, b) { return dev.quietDays(b) - dev.quietDays(a); });
      $("quiet-tools").hidden = !quietMode;
      if (quietMode) {
        $("quiet-count").textContent = (T.quiet_n || "{n} contacts quiet for 90+ days.").replace("{n}", cs.length) + " ";
        $("remove-quiet").hidden = !cs.length;
        $("remove-quiet").onclick = function () {
          if (!confirm(T.remove_quiet_confirm || "Remove these contacts?")) return;
          cs.reduce(function (p, c) { return p.then(function () { return dev.store.deleteContact(c.id); }); }, Promise.resolve()).then(show);
        };
      }
      list.textContent = "";
      empty.hidden = cs.length > 0;
      $("contacts-tools").hidden = cs.length === 0;
      // Chat lines go over encrypted OLN (mined in a worker); the rest
      // (cards, signals, alive) over the pair mailbox.
      function send(c, msg) {
        if (msg.t === "msg" && window.kafumuOLN) return pair.sendChat(c, msg.text, function (text, kw, bits) { return window.kafumuOLN.post(text, kw, bits, function () {}); });
        return pair.send(c, msg);
      }
      function check(c) { return pair.checkContact(c).then(function (got) { return pair.readChat(c).then(function (n) { return n ? got.concat([{ t: "msg" }]) : got; }); }); }
      // The same person twice (e.g. they used two of your links): the newer
      // one offers to remove itself.
      var byName = {};
      cs.slice().sort(function (a, b) { return (a.createdAt || "").localeCompare(b.createdAt || ""); }).forEach(function (c) {
        var n = c.card && c.card.name && c.card.name.trim().toLowerCase();
        if (n) { if (byName[n]) c._dup = byName[n]; else byName[n] = c.id; }
      });
      cs.forEach(function (c) {
        var opts = { onDelete: function () { if (!list.children.length) show(); }, send: send, check: check, duplicateOf: c._dup };
        var li = dev.renderContact(c, T, opts);
        if (c.lastSeen) {
          var seen = document.createElement("p");
          seen.className = "dim small";
          seen.textContent = "📍 " + (T.seen_near || "seen near {where} on {day}").replace("{where}", distLabel(c.lastSeen.cell))
            .replace("{day}", new Date(c.lastSeen.day + "T12:00:00Z").toLocaleDateString(window.KAFUMU_LOCALE, { weekday: "long", day: "numeric", month: "short" }));
          li.insertBefore(seen, li.children[1] || null);
        }
        li.dataset.search = JSON.stringify([c.card, c.alias, c.note]).toLowerCase();
        list.appendChild(li);
        // Cards on their way and new signals: one mailbox read per contact.
        check(c).then(function (got) {
          if (!got.length) return;
          var fresh = dev.renderContact(c, T, opts);
          fresh.dataset.search = li.dataset.search;
          list.replaceChild(fresh, li);
          li = fresh;
        }).catch(function () {});
      });
    });
  }

  // "Nearest": by distance from your current area to where each contact was
  // last seen (friends around, on this device). Unknown ones go last.
  function here() { try { return localStorage.getItem("kafumu.lastCell"); } catch (e) { return null; } }
  function km(a, b) {
    var G = window.kafumuGeo, p = G.center(a), q = G.center(b), r = Math.PI / 180;
    var x = (q[1] - p[1]) * r * Math.cos((p[0] + q[0]) / 2 * r), y = (q[0] - p[0]) * r;
    return Math.sqrt(x * x + y * y) * 6371;
  }
  function distLabel(cell) {
    var h = here();
    if (!h || !window.kafumuGeo) return "#geo" + cell;
    var d = km(h, cell);
    return d < 8 ? (T.here_word || "here") : Math.round(d) + " km";
  }
  function byDistance(cs) {
    var h = here();
    if (!h || !window.kafumuGeo) return cs;
    return cs.slice().sort(function (a, b) {
      var da = a.lastSeen ? km(h, a.lastSeen.cell) : Infinity, db = b.lastSeen ? km(h, b.lastSeen.cell) : Infinity;
      return da - db;
    });
  }
  function sortBy() { try { return localStorage.getItem("kafumu.contactSort") || "new"; } catch (e) { return "new"; } }
  var sortSel = $("contacts-sort");
  if (sortSel) {
    sortSel.value = sortBy();
    sortSel.onchange = function () { try { localStorage.setItem("kafumu.contactSort", sortSel.value); } catch (e) {} show(); };
  }

  filter.addEventListener("input", function () {
    var q = filter.value.trim().toLowerCase();
    Array.prototype.forEach.call(list.children, function (li) { li.hidden = q && li.dataset.search.indexOf(q) < 0; });
  });
  $("export-vcf").onclick = function () {
    dev.store.contacts().then(function (cs) { dev.download("kafumu-contacts.vcf", "text/vcard", dev.vcards(cs)); });
  };
  $("export-json").onclick = function () {
    dev.backup().then(function (b) { dev.download("kafumu-backup-" + b.exportedAt.slice(0, 10) + ".json", "application/json", JSON.stringify(b, null, 1)); });
  };
  $("import-json").onchange = function () {
    var f = this.files[0];
    if (!f) return;
    f.text().then(function (t) { return dev.restore(JSON.parse(t)); })
      .then(function (n) { $("contacts-status").textContent = (T.restored || "Restored {n}").replace("{n}", n); show(); })
      .catch(function () { $("contacts-status").textContent = T.restore_failed || "Not a Kafumu backup."; });
  };
  takeInvites().then(function (n) { if (n) show(); });
  // Your kafumu.com/@name link, if on: renewed weekly from this device.
  dev.store.get("handle").then(function (h) {
    var bd = document.body.dataset, named = bd.actingId ? bd.actingNamed : bd.named; // you, or the named business you act as
    if (h && !h.off && named && Date.now() - h.at > 7 * 864e5) pair.namedLink().catch(function () {});
  });
  // A weekly "alive" to each contact (one small encrypted message), so both
  // sides can tell a connection still works.
  function pingQuietly() {
    dev.store.contacts().then(function (cs) {
      var week = 7 * 864e5;
      cs.filter(function (c) { return !c.lastPing || Date.now() - new Date(c.lastPing) > week; }).slice(0, 30).reduce(function (p, c) {
        return p.then(function () {
          return pair.send(c, { t: "alive", at: new Date().toISOString() }).then(function () { c.lastPing = new Date().toISOString(); return dev.store.putContact(c); }).catch(function () {});
        });
      }, Promise.resolve());
    }).catch(function () {});
  }
  setTimeout(pingQuietly, 3000);
  show();

  // Messages to your public inbox: decrypted here; Connect makes a contact.
  function showInbox() {
    pair.readInbox().then(function (msgs) {
      var sec = $("inbox-section"), ul = $("inbox-msgs");
      sec.hidden = !msgs.length;
      ul.textContent = "";
      msgs.forEach(function (m) {
        var li = document.createElement("li");
        var who = document.createElement("strong");
        who.textContent = (m.card && m.card.name) || (T.inbox_anonymous || "Someone");
        var meta = document.createElement("span");
        meta.className = "dim"; meta.textContent = " · " + new Date(m.at).toLocaleString(window.KAFUMU_LOCALE, { day: "numeric", month: "short", hour: "2-digit", minute: "2-digit" });
        var p = document.createElement("p");
        p.className = "text"; p.textContent = m.text;
        li.appendChild(who); li.appendChild(meta); li.appendChild(p);
        var row = document.createElement("div");
        row.className = "actions";
        if (m.card) {
          var c = document.createElement("button");
          c.type = "button"; c.className = "pill-sm suggested"; c.textContent = T.inbox_connect || "Connect";
          c.onclick = function () { dev.personas.shareCard().then(function (card) { return pair.connectBack(m, card); }).then(function () { forget(m); show(); }); };
          row.appendChild(c);
        }
        var x = document.createElement("button");
        x.type = "button"; x.className = "pill-sm"; x.textContent = T.remove || "Remove";
        x.onclick = function () { forget(m); };
        row.appendChild(x);
        li.appendChild(row);
        ul.appendChild(li);
      });
    }).catch(function () {});
  }
  function forget(m) {
    dev.store.get("inboxMsgs").then(function (ms) {
      return dev.store.set("inboxMsgs", (ms || []).filter(function (x) { return x.id !== m.id; }));
    }).then(showInbox);
  }
  showInbox();

  // Push: watch our own inboxes (one per contact, plus invite codes) so a
  // signal wakes the phone. The server only learns endpoint ↔ random ids.
  function inboxes() {
    return Promise.all([dev.store.contacts(), dev.store.get("invite"), dev.store.get("invite:badge")]).then(function (r) {
      var ids = r[0].map(function (c) { return pair._boxOf(pair._unb64(c.key), c.role); });
      [r[1], r[2]].forEach(function (inv) { if (inv && inv.box) ids.push(Promise.resolve(inv.box)); });
      ids.push(dev.store.get("publicInbox").then(function (ib) { return ib && ib.box; }));
      return Promise.all(ids).then(function (all) { return all.filter(Boolean); });
    });
  }
  function b64ToBytes(s) { s = s.replace(/-/g, "+").replace(/_/g, "/"); while (s.length % 4) s += "="; return Uint8Array.from(atob(s), function (c) { return c.charCodeAt(0); }); }
  function subscribe(ask) {
    if (!("serviceWorker" in navigator) || !("PushManager" in window)) return Promise.reject(new Error("unsupported"));
    return navigator.serviceWorker.ready.then(function (reg) {
      return reg.pushManager.getSubscription().then(function (sub) {
        if (sub) return sub;
        if (!ask) return null;
        return fetch("/api/push/key").then(function (r) { return r.json(); }).then(function (k) {
          return reg.pushManager.subscribe({ userVisibleOnly: true, applicationServerKey: b64ToBytes(k.publicKey) });
        });
      });
    }).then(function (sub) {
      if (!sub) return false;
      return inboxes().then(function (boxes) {
        return fetch("/api/push/subscribe", { method: "POST", credentials: "omit", headers: { "Content-Type": "application/json" },
          body: JSON.stringify({ subscription: sub.toJSON(), boxes: boxes, lang: document.documentElement.lang }) });
      }).then(function () { return true; });
    });
  }
  var bell = $("push-on");
  if (bell) {
    subscribe(false).then(function (on) { bell.hidden = on; $("push-state").textContent = on ? (T.push_on || "On") : ""; }, function () { bell.hidden = true; });
    bell.onclick = function () {
      Notification.requestPermission().then(function (p) {
        if (p !== "granted") { $("push-state").textContent = T.push_denied || "Notifications are blocked."; return; }
        // Some browsers have no push service and never answer: give up after 15 s.
        var timeout = new Promise(function (_, no) { setTimeout(function () { no(new Error("timeout")); }, 15000); });
        return Promise.race([subscribe(true), timeout]).then(function () { bell.hidden = true; $("push-state").textContent = T.push_on || "On"; });
      }).catch(function () { $("push-state").textContent = T.push_failed || "Couldn't turn on notifications."; });
    };
  }

  // Opening Contacts marks signals as seen.
  setTimeout(function () {
    dev.store.contacts().then(function (cs) {
      cs.forEach(function (c) {
        if ((c.signals || []).some(function (x) { return x.unread; })) {
          c.signals.forEach(function (x) { x.unread = false; });
          dev.store.putContact(c);
        }
      });
    });
  }, 4000);
})();
