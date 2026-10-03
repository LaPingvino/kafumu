// /contacts: everyone you've connected with, kept on this device.
(function () {
  "use strict";
  var T = window.KAFUMU_T || {}, dev = window.kafumuDevice;
  var pair = window.kafumuPair.create({ fetch: window.fetch.bind(window), store: dev.store, origin: location.origin });
  var $ = function (id) { return document.getElementById(id); };
  var list = $("contacts"), empty = $("contacts-empty"), filter = $("contacts-filter");

  function show() {
    dev.store.contacts().then(function (cs) {
      cs.sort(function (a, b) { return (b.createdAt || "").localeCompare(a.createdAt || ""); });
      list.textContent = "";
      empty.hidden = cs.length > 0;
      $("contacts-tools").hidden = cs.length === 0;
      function send(c, msg) { return pair.send(c, msg); }
      cs.forEach(function (c) {
        var opts = { onDelete: function () { if (!list.children.length) show(); }, send: send };
        var li = dev.renderContact(c, T, opts);
        li.dataset.search = JSON.stringify([c.card, c.note]).toLowerCase();
        list.appendChild(li);
        // Cards on their way and new signals: one mailbox read per contact.
        pair.checkContact(c).then(function (got) {
          if (!got.length) return;
          var fresh = dev.renderContact(c, T, opts);
          fresh.dataset.search = li.dataset.search;
          list.replaceChild(fresh, li);
          li = fresh;
        }).catch(function () {});
      });
    });
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
  show();

  // Push: watch our own inboxes (one per contact, plus invite codes) so a
  // signal wakes the phone. The server only learns endpoint ↔ random ids.
  function inboxes() {
    return Promise.all([dev.store.contacts(), dev.store.get("invite"), dev.store.get("invite:badge")]).then(function (r) {
      var ids = r[0].map(function (c) { return pair._boxOf(pair._unb64(c.key), c.role); });
      [r[1], r[2]].forEach(function (inv) { if (inv && inv.box) ids.push(Promise.resolve(inv.box)); });
      return Promise.all(ids);
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
