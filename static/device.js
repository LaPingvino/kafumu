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

  // Personas: several cards, each with its own fields and tags. The old
  // single "card" becomes the first persona. A share picks one persona and
  // a subset of its fields; the name always travels.
  var SUGGESTED_TAGS = ["open source", "startups", "AI", "design", "climate", "languages", "Esperanto",
    "hiring", "looking for work", "investing", "co-founder search", "music", "coffee", "running"];

  function newID() { return Array.from(crypto.getRandomValues(new Uint8Array(6)), function (b) { return b.toString(16).padStart(2, "0"); }).join(""); }

  var personas = {
    list: function () {
      return store.get("personas").then(function (ps) {
        if (ps && ps.length) return ps;
        return store.get("card").then(function (card) {
          var first = [{ id: newID(), label: "", card: card || {} }];
          return store.set("personas", first).then(function () { return first; });
        });
      });
    },
    save: function (ps) {
      // Keep "card" mirroring the first persona for older pages and backups.
      return store.set("personas", ps).then(function () { return store.set("card", (ps[0] && ps[0].card) || {}); });
    },
    // share returns what to hand over: persona p, only the ticked fields.
    share: function (p, fields) {
      var c = p.card || {}, out = { name: c.name };
      (fields || FIELDS).forEach(function (f) { if (f !== "name" && c[f]) out[f] = c[f]; });
      if (c.tags && c.tags.length && (!fields || fields.indexOf("tags") >= 0)) out.tags = c.tags.slice();
      return out;
    },
    // choice is the last persona + fields picked on /connect.
    choice: function () {
      return Promise.all([personas.list(), store.get("shareChoice")]).then(function (r) {
        var ps = r[0], ch = r[1] || {};
        var p = ps.filter(function (x) { return x.id === ch.persona; })[0] || ps[0];
        return { persona: p, fields: ch.persona === p.id && ch.fields ? ch.fields : null, all: ps };
      });
    },
    setChoice: function (personaID, fields) { return store.set("shareChoice", { persona: personaID, fields: fields }); },
    shareCard: function () { return personas.choice().then(function (ch) { return personas.share(ch.persona, ch.fields); }); },
    newID: newID,
    SUGGESTED_TAGS: SUGGESTED_TAGS
  };

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
    if (card.tags && card.tags.length) {
      var theirs = el("div", "chips");
      card.tags.forEach(function (t) { theirs.appendChild(el("span", "chip", t)); });
      li.appendChild(theirs);
    }
    // Your own tags about them ("met at WS"), private like the note.
    var mine = el("div", "chips mine");
    function drawMine() {
      mine.textContent = "";
      (c.tags || []).forEach(function (t, i) {
        var chip = el("button", "chip", t + " ×");
        chip.type = "button";
        chip.title = T.remove || "Remove";
        chip.onclick = function () { c.tags.splice(i, 1); store.putContact(c); drawMine(); };
        mine.appendChild(chip);
      });
      var add = el("input", "chip-input");
      add.placeholder = T.add_tag || "+ tag";
      add.setAttribute("aria-label", T.add_tag || "tag");
      add.addEventListener("keydown", function (e) {
        if (e.key !== "Enter" && e.key !== ",") return;
        e.preventDefault();
        var v = add.value.trim().slice(0, 40);
        if (!v) return;
        c.tags = (c.tags || []).concat([v]);
        store.putContact(c); drawMine(); mine.querySelector("input").focus();
      });
      mine.appendChild(add);
    }
    drawMine();
    var row = el("div", "actions");
    links(card).forEach(function (l) {
      var a = el("a", "pill-sm", T["field_" + l.field] || l.field);
      a.href = l.href; a.target = "_blank"; a.rel = "noopener"; a.setAttribute("role", "button");
      row.appendChild(a);
    });
    li.appendChild(row);
    if (opts.preview) return li;
    li.appendChild(mine);
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
      var cats = (k.tags || []).concat(c.tags || []);
      if (cats.length) lines.push("CATEGORIES:" + cats.map(esc).join(","));
      var note = [k.about, c.note, "Kafumu " + (c.createdAt || "").slice(0, 10)].filter(Boolean).join(" — ");
      lines.push("NOTE:" + esc(note), "END:VCARD");
      return lines.join("\r\n");
    }).join("\r\n") + "\r\n";
  }

  // backup is everything on this device, including pair keys: treat the
  // file like a password. restore merges it back in.
  function backup() {
    return Promise.all([store.get("card"), store.contacts(), store.get("personas")]).then(function (r) {
      return { kafumu: 1, exportedAt: new Date().toISOString(), card: r[0] || {}, contacts: r[1] || [], personas: r[2] || [] };
    });
  }
  function restore(data) {
    if (!data || data.kafumu !== 1 || !Array.isArray(data.contacts)) return Promise.reject(new Error("not a Kafumu backup"));
    return store.get("card").then(function (mine) {
      var steps = data.contacts.map(function (c) { return store.putContact(c); });
      if ((!mine || !mine.name) && data.card) steps.push(store.set("card", data.card));
      if ((!mine || !mine.name) && data.personas && data.personas.length) steps.push(store.set("personas", data.personas));
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

  window.kafumuDevice = { store: store, FIELDS: FIELDS, links: links, renderContact: renderContact, personas: personas,
    vcards: vcards, backup: backup, restore: restore, download: download };
})();
