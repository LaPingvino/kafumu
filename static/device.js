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

  function el(tag, cls, text) { var e = document.createElement(tag); if (cls) e.className = cls; if (text != null) e.textContent = text; return e; }

  // renderContact draws one contact: name, one-liner, one-tap links and a
  // private note saved on change. opts.onDelete adds a remove button.
  function renderContact(c, T, opts) {
    T = T || {}; opts = opts || {};
    var li = el("li", "contact"), card = c.card || {};
    var head = el("div", "contact-head");
    head.appendChild(el("strong", null, card.name || T.waiting_card || "…"));
    if (c.createdAt) head.appendChild(el("span", "dim", new Date(c.createdAt).toLocaleDateString()));
    li.appendChild(head);
    if (card.about) li.appendChild(el("p", "dim", card.about));
    var row = el("div", "actions");
    links(card).forEach(function (l) {
      var a = el("a", "pill-sm", T["field_" + l.field] || l.field);
      a.href = l.href; a.target = "_blank"; a.rel = "noopener"; a.setAttribute("role", "button");
      row.appendChild(a);
    });
    li.appendChild(row);
    var note = el("input");
    note.placeholder = T.note_placeholder || "";
    note.value = c.note || "";
    note.setAttribute("aria-label", T.note_placeholder || "note");
    note.addEventListener("change", function () { c.note = note.value; store.putContact(c); });
    li.appendChild(note);
    if (opts.onDelete) {
      var del = el("button", "contrast pill-sm", T.remove || "Remove");
      del.type = "button";
      del.onclick = function () { if (confirm(T.remove_confirm || "Remove?")) store.deleteContact(c.id).then(function () { li.remove(); opts.onDelete(c); }); };
      li.appendChild(del);
    }
    return li;
  }

  // vcards renders contacts as one vCard 3.0 file, for phone address books.
  function vcards(contacts) {
    function esc(s) { return String(s).replace(/\\/g, "\\\\").replace(/\n/g, "\\n").replace(/([,;])/g, "\\$1"); }
    return contacts.filter(function (c) { return c.card && c.card.name; }).map(function (c) {
      var k = c.card, lines = ["BEGIN:VCARD", "VERSION:3.0", "FN:" + esc(k.name)];
      if (k.email) lines.push("EMAIL:" + esc(k.email));
      if (k.phone) lines.push("TEL:" + esc(k.phone));
      if (k.whatsapp && k.whatsapp !== k.phone) lines.push("TEL;TYPE=CELL:" + esc(k.whatsapp));
      links(k).forEach(function (l) { if (/^https:/.test(l.href)) lines.push("URL:" + l.href); });
      var note = [k.about, c.note, "Kafumu " + (c.createdAt || "").slice(0, 10)].filter(Boolean).join(" — ");
      lines.push("NOTE:" + esc(note), "END:VCARD");
      return lines.join("\r\n");
    }).join("\r\n") + "\r\n";
  }

  // backup is everything on this device, including pair keys: treat the
  // file like a password. restore merges it back in.
  function backup() {
    return Promise.all([store.get("card"), store.contacts()]).then(function (r) {
      return { kafumu: 1, exportedAt: new Date().toISOString(), card: r[0] || {}, contacts: r[1] || [] };
    });
  }
  function restore(data) {
    if (!data || data.kafumu !== 1 || !Array.isArray(data.contacts)) return Promise.reject(new Error("not a Kafumu backup"));
    return store.get("card").then(function (mine) {
      var steps = data.contacts.map(function (c) { return store.putContact(c); });
      if ((!mine || !mine.name) && data.card) steps.push(store.set("card", data.card));
      return Promise.all(steps).then(function () { return data.contacts.length; });
    });
  }

  function download(name, type, text) {
    var a = document.createElement("a");
    a.href = URL.createObjectURL(new Blob([text], { type: type }));
    a.download = name;
    document.body.appendChild(a); a.click(); a.remove();
    setTimeout(function () { URL.revokeObjectURL(a.href); }, 1000);
  }

  window.kafumuDevice = { store: store, FIELDS: FIELDS, links: links, renderContact: renderContact,
    vcards: vcards, backup: backup, restore: restore, download: download };
})();
