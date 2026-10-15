// Kafumu service worker: an offline shell. Pages are network-first with the
// last copy as fallback; versioned static assets are cache-first. The API,
// bundles and anything cross-origin are never cached here.
var CACHE = "kafumu-v3";

// Precache the pages that must work offline (opening someone's connect
// link without a network queues your card), with the scripts and styles
// they load. Best effort: install never fails on it.
var OFFLINE_PAGES = ["/c", "/contacts"];
self.addEventListener("install", function (e) {
  self.skipWaiting();
  e.waitUntil(caches.open(CACHE).then(function (c) {
    return Promise.all(OFFLINE_PAGES.map(function (p) {
      return fetch(p, { credentials: "same-origin" }).then(function (res) {
        if (!res.ok) return;
        return res.clone().text().then(function (html) {
          var assets = [], re = /(?:src|href)="(\/static\/[^"]+)"/g, m;
          while ((m = re.exec(html))) assets.push(m[1].replace(/&amp;/g, "&"));
          return c.put(p, res).then(function () {
            return Promise.all(assets.map(function (a) { return c.add(a).catch(function () {}); }));
          });
        });
      }).catch(function () {});
    }));
  }).catch(function () {}));
});
self.addEventListener("activate", function (e) {
  e.waitUntil(caches.keys().then(function (ks) {
    return Promise.all(ks.filter(function (k) { return k !== CACHE; }).map(function (k) { return caches.delete(k); }));
  }).then(function () { return self.clients.claim(); }));
});

self.addEventListener("fetch", function (e) {
  var req = e.request, url = new URL(req.url);
  if (req.method !== "GET" || url.origin !== location.origin) return;
  if (/^\/(api|bundle|cron|auth|cal|rss|n)\b/.test(url.pathname) || url.pathname.endsWith("/ics")) return;
  if (url.pathname.startsWith("/static/")) {
    e.respondWith(caches.match(req).then(function (hit) {
      return hit || fetch(req).then(function (res) {
        if (res.ok) { var copy = res.clone(); caches.open(CACHE).then(function (c) { c.put(req, copy); }); }
        return res;
      });
    }));
    return;
  }
  if (req.mode === "navigate") {
    e.respondWith(fetch(req).then(function (res) {
      if (res.ok) { var copy = res.clone(); caches.open(CACHE).then(function (c) { c.put(url.pathname, copy); }); }
      return res;
    }).catch(function () {
      return caches.match(url.pathname).then(function (hit) { return hit || caches.match("/contacts") || Response.error(); });
    }));
  }
});

// Push: a contact sent a signal. The message carries no content; opening
// Contacts reads and decrypts it on the device.
// The push names only the box that woke up; what to say about it (who,
// which post) and where to open was written by the app on this device when
// it subscribed (pair.js saveLabels), so the server never knows (83d).
function labelFor(box) {
  return new Promise(function (resolve) {
    if (!box) return resolve(null);
    var req = indexedDB.open("kafumu");
    req.onupgradeneeded = function () { req.transaction.abort(); }; // never create it here: device.js owns its shape
    req.onerror = function () { resolve(null); };
    req.onsuccess = function () {
      var d = req.result;
      if (!d.objectStoreNames.contains("kv")) { d.close(); return resolve(null); }
      var kv = d.transaction("kv").objectStore("kv"), get = kv.get("pushLabels"), mine = kv.get("justPosted");
      mine.onsuccess = function () {
        get.onsuccess = function () {
          var l = (get.result || {})[box] || null;
          // Your own board post (97e2): one tag for the whole group, so it
          // comes back to you too; shown, but silently.
          if (l && Date.now() - ((mine.result || {})[box] || 0) < 120000) l = Object.assign({}, l, { silent: true });
          d.close(); resolve(l);
        };
        if (get.readyState === "done") get.onsuccess();
      };
      get.onerror = function () { d.close(); resolve(null); };
    };
  });
}
self.addEventListener("push", function (e) {
  var d = {};
  try { d = e.data ? e.data.json() : {}; } catch (err) {}
  e.waitUntil(labelFor(d.box).catch(function () { return null; }).then(function (l) {
    var url = (l && l.url) || d.url || "/contacts";
    return self.registration.showNotification("Kafumu", {
      body: (l && l.body) || d.text || "☕", tag: "kafumu-" + (d.box || url), renotify: !(l && l.silent), silent: !!(l && l.silent), icon: "/static/icon.svg", data: { url: url }
    });
  }));
});
self.addEventListener("notificationclick", function (e) {
  e.notification.close();
  var url = (e.notification.data && e.notification.data.url) || "/";
  e.waitUntil(self.clients.matchAll({ type: "window" }).then(function (ws) {
    // An open Kafumu window opens that conversation in place (88a): it gets
    // a message and shows the chat window, or goes there for anything else.
    for (var j = 0; j < ws.length; j++) if ("focus" in ws[j]) { ws[j].postMessage({ open: url }); return ws[j].focus(); }
    return self.clients.openWindow(url);
  }));
});
