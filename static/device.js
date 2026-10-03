// Kafumu device store: everything private lives here, in IndexedDB, and
// never leaves the device unencrypted (VISION.md, tier 1).
(function () {
  "use strict";
  var DB = "kafumu", VERSION = 1, db;

  function open() {
    if (db) return Promise.resolve(db);
    return new Promise(function (resolve, reject) {
      var req = indexedDB.open(DB, VERSION);
      req.onupgradeneeded = function () {
        var d = req.result;
        if (!d.objectStoreNames.contains("kv")) d.createObjectStore("kv");
        if (!d.objectStoreNames.contains("contacts")) d.createObjectStore("contacts", { keyPath: "id" });
      };
      req.onsuccess = function () { db = req.result; resolve(db); };
      req.onerror = function () { reject(req.error); };
    });
  }

  function tx(store, mode, fn) {
    return open().then(function (d) {
      return new Promise(function (resolve, reject) {
        var t = d.transaction(store, mode), s = t.objectStore(store), out;
        var req = fn(s);
        if (req) req.onsuccess = function () { out = req.result; };
        t.oncomplete = function () { resolve(out); };
        t.onerror = t.onabort = function () { reject(t.error); };
      });
    });
  }

  var store = {
    get: function (k) { return tx("kv", "readonly", function (s) { return s.get(k); }); },
    set: function (k, v) { return tx("kv", "readwrite", function (s) { return s.put(v, k); }); },
    contacts: function () { return tx("contacts", "readonly", function (s) { return s.getAll(); }); },
    putContact: function (c) { return tx("contacts", "readwrite", function (s) { return s.put(c); }); },
    deleteContact: function (id) { return tx("contacts", "readwrite", function (s) { return s.delete(id); }); }
  };

  // Ask the browser not to evict our data (Safari drops storage of sites
  // not installed to the home screen after 7 idle days).
  if (navigator.storage && navigator.storage.persist) navigator.storage.persist().catch(function () {});

  // Card fields, in display order. The card holds what *you* choose to hand
  // to someone you meet; nothing is required.
  var FIELDS = ["name", "about", "email", "phone", "whatsapp", "signal", "telegram", "bluesky", "linkedin", "website"];

  function digits(s) { return (s || "").replace(/[^\d+]/g, "").replace(/^00/, "+"); }
  function safeURL(s) {
    s = (s || "").trim();
    if (!s) return "";
    if (!/^https?:\/\//i.test(s)) s = "https://" + s;
    try { var u = new URL(s); return u.protocol === "https:" || u.protocol === "http:" ? u.href : ""; } catch (e) { return ""; }
  }

  // links turns a card into [{field, label, href}] for one-tap contact.
  function links(card) {
    var out = [];
    function add(f, label, href) { if (card[f] && href) out.push({ field: f, label: label, href: href }); }
    add("email", card.email, "mailto:" + encodeURIComponent((card.email || "").trim()));
    add("phone", card.phone, "tel:" + digits(card.phone));
    add("whatsapp", card.whatsapp, "https://wa.me/" + digits(card.whatsapp).replace("+", ""));
    add("signal", card.signal, /^\+?\d[\d\s-]+$/.test(card.signal || "")
      ? "https://signal.me/#p/" + digits(card.signal) : safeURL(card.signal));
    add("telegram", card.telegram, "https://t.me/" + (card.telegram || "").trim().replace(/^@/, "").replace(/^https?:\/\/t\.me\//, ""));
    add("bluesky", card.bluesky, "https://bsky.app/profile/" + (card.bluesky || "").trim().replace(/^@/, "").replace(/^https?:\/\/bsky\.app\/profile\//, ""));
    add("linkedin", card.linkedin, /^https?:|linkedin\.com/i.test(card.linkedin || "")
      ? safeURL(card.linkedin) : "https://www.linkedin.com/in/" + encodeURIComponent((card.linkedin || "").trim()));
    add("website", card.website, safeURL(card.website));
    return out;
  }

  window.kafumuDevice = { store: store, FIELDS: FIELDS, links: links };
})();
