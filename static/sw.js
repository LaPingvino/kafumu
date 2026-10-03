// Kafumu service worker: an offline shell. Pages are network-first with the
// last copy as fallback; versioned static assets are cache-first. The API,
// bundles and anything cross-origin are never cached here.
var CACHE = "kafumu-v2";

self.addEventListener("install", function () { self.skipWaiting(); });
self.addEventListener("activate", function (e) {
  e.waitUntil(caches.keys().then(function (ks) {
    return Promise.all(ks.filter(function (k) { return k !== CACHE; }).map(function (k) { return caches.delete(k); }));
  }).then(function () { return self.clients.claim(); }));
});

self.addEventListener("fetch", function (e) {
  var req = e.request, url = new URL(req.url);
  if (req.method !== "GET" || url.origin !== location.origin) return;
  if (/^\/(api|bundle|cron|auth|cal)\b/.test(url.pathname) || url.pathname.endsWith("/ics")) return;
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
self.addEventListener("push", function (e) {
  var d = {};
  try { d = e.data ? e.data.json() : {}; } catch (err) {}
  e.waitUntil(self.registration.showNotification("Kafumu", {
    body: d.text || "☕", tag: "kafumu-signal", renotify: true, icon: "/static/icon.svg", data: { url: "/contacts" }
  }));
});
self.addEventListener("notificationclick", function (e) {
  e.notification.close();
  var url = (e.notification.data && e.notification.data.url) || "/";
  e.waitUntil(self.clients.matchAll({ type: "window" }).then(function (ws) {
    for (var i = 0; i < ws.length; i++) if (ws[i].url.indexOf(url) >= 0 && "focus" in ws[i]) return ws[i].focus();
    return self.clients.openWindow(url);
  }));
});
