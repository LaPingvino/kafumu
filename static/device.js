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
  var SUGGESTED_TAGS = ["open source", "startups", "AI", "design", "climate", "languages",
    "hiring", "looking for work", "investing", "co-founder search", "music", "coffee", "running"];
  // Plus the languages this browser says you speak, named in the UI language.
  try {
    var names = new Intl.DisplayNames([document.documentElement.lang || "en"], { type: "language" });
    (navigator.languages || []).forEach(function (l) {
      var n = names.of(l.split("-")[0]);
      if (n && SUGGESTED_TAGS.indexOf(n) < 0) SUGGESTED_TAGS.splice(6, 0, n);
    });
  } catch (e) {}

  // The country to assume for examples (phone numbers): where your area is
  // (remembered by Around), else the browser's region. Never a fixed one.
  var DIAL = { AD: 376, AE: 971, AL: 355, AM: 374, AO: 244, AR: 54, AT: 43, AU: 61, AZ: 994, BA: 387, BD: 880, BE: 32,
    BG: 359, BO: 591, BR: 55, BY: 375, CA: 1, CH: 41, CL: 56, CM: 237, CN: 86, CO: 57, CR: 506, CU: 53, CV: 238, CY: 357,
    CZ: 420, DE: 49, DK: 45, DO: 1, DZ: 213, EC: 593, EE: 372, EG: 20, ES: 34, ET: 251, FI: 358, FJ: 679, FO: 298, FR: 33,
    GB: 44, GE: 995, GH: 233, GL: 299, GR: 30, GT: 502, GW: 245, HK: 852, HR: 385, HT: 509, HU: 36, ID: 62, IE: 353,
    IL: 972, IN: 91, IQ: 964, IR: 98, IS: 354, IT: 39, JM: 1, JO: 962, JP: 81, KE: 254, KG: 996, KH: 855, KR: 82, KW: 965,
    KZ: 7, LB: 961, LK: 94, LT: 370, LU: 352, LV: 371, MA: 212, MD: 373, ME: 382, MG: 261, MK: 389, MN: 976, MO: 853,
    MT: 356, MU: 230, MX: 52, MY: 60, MZ: 258, NA: 264, NG: 234, NL: 31, NO: 47, NP: 977, NZ: 64, PA: 507, PE: 51,
    PH: 63, PK: 92, PL: 48, PT: 351, PY: 595, QA: 974, RO: 40, RS: 381, RU: 7, SA: 966, SE: 46, SG: 65, SI: 386, SK: 421,
    SN: 221, SR: 597, ST: 239, SY: 963, TH: 66, TL: 670, TN: 216, TR: 90, TW: 886, TZ: 255, UA: 380, UG: 256, US: 1,
    UY: 598, UZ: 998, VE: 58, VN: 84, ZA: 27, ZM: 260, ZW: 263, AW: 297, CW: 599, BQ: 599, SX: 1, PR: 1 };
  function country() {
    var cc = "";
    try { cc = localStorage.getItem("kafumu.country") || ""; } catch (e) {}
    if (!cc) {
      (navigator.languages || [navigator.language || ""]).some(function (l) { var m = /-([A-Z]{2})$/.exec(l); if (m) cc = m[1]; return !!m; });
    }
    return cc;
  }
  function phoneExample() { var d = DIAL[country()]; return d ? "+" + d + "…" : "+…"; }

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
    SUGGESTED_TAGS: SUGGESTED_TAGS,
    phoneExample: phoneExample
  };

  function el(tag, cls, text) { var e = document.createElement(tag); if (cls) e.className = cls; if (text != null) e.textContent = text; return e; }

  // renderContact draws one contact: name, one-liner, one-tap links and a
  // private note saved on change. opts.onDelete adds a remove button.
  function renderContact(c, T, opts) {
    T = T || {}; opts = opts || {};
    var li = el("li", "contact"), card = c.card || {};
    var head = el("div", "contact-head");
    if (card.name) head.appendChild(el("strong", null, card.name));
    else if (opts.preview || !c.card && !c.alias) head.appendChild(el("strong", null, c.alias || T.waiting_card || "…"));
    else {
      // No card from them (yet, or ever): give them a name yourself.
      var alias = el("input", "alias");
      alias.value = c.alias || "";
      alias.placeholder = T.alias_placeholder || "Your name for them";
      alias.setAttribute("aria-label", alias.placeholder);
      alias.addEventListener("change", function () { c.alias = alias.value.trim().slice(0, 80); store.putContact(c); });
      head.appendChild(alias);
    }
    if (c.createdAt) head.appendChild(el("span", "dim", new Date(c.createdAt).toLocaleDateString(window.KAFUMU_LOCALE, { day: "numeric", month: "short", year: "numeric" })));
    li.appendChild(head);
    if (card.about) li.appendChild(el("p", "dim", card.about));
    var sig = (c.signals || [])[0];
    if (sig && !opts.preview) {
      var sp = el("p", "signal" + (sig.unread ? " unread" : ""), signalText(sig, T) + " · " + ago(sig.at));
      li.appendChild(sp);
    }
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
    if (opts.send && c.cardSent === false) {
      var give = el("button", "pill-sm", T.send_my_card || "Send my card");
      give.type = "button";
      give.onclick = function () {
        give.disabled = true;
        personas.shareCard().then(function (mine) { return opts.send(c, { t: "card", card: mine }); })
          .then(function () { c.cardSent = true; store.putContact(c); give.textContent = "✓ " + (T.card_sent || "Card sent"); }, function () { give.disabled = false; });
      };
      li.appendChild(give);
    }
    if (opts.send) {
      var srow = el("div", "actions signals");
      SIGNALS.forEach(function (kind) {
        var b = el("button", "pill-sm", T["sig_btn_" + kind] || kind);
        b.type = "button";
        b.onclick = function () {
          var text = "";
          if (kind === "here") { text = prompt(T.sig_where || "Where are you?") || ""; if (!text) return; }
          b.disabled = true;
          opts.send(c, { t: "signal", s: kind, text: text.slice(0, 140), at: new Date().toISOString() })
            .then(function () { b.textContent = "✓ " + (T["sig_btn_" + kind] || kind); }, function () { b.disabled = false; });
        };
        srow.appendChild(b);
      });
      li.appendChild(srow);
    }
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

  // Canned signals: small, structured, no free-form chat (VISION.md).
  var SIGNALS = ["coffee", "here", "thanks"];
  function signalText(sig, T) {
    var t = T["sig_" + sig.s] || sig.s;
    return sig.text ? t + " " + sig.text : t;
  }
  function ago(iso) {
    var s = (Date.now() - new Date(iso).getTime()) / 1000;
    if (s < 3600) return (T0.min_ago || "{n} min ago").replace("{n}", Math.max(1, Math.round(s / 60)));
    if (s < 86400) return (T0.h_ago || "{n} h ago").replace("{n}", Math.round(s / 3600));
    return (T0.d_ago || "{n} d ago").replace("{n}", Math.round(s / 86400));
  }
  var T0 = (typeof window !== "undefined" && window.KAFUMU_T) || {};

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

  window.kafumuDevice = { signalText: signalText, store: store, FIELDS: FIELDS, links: links, renderContact: renderContact, personas: personas,
    vcards: vcards, backup: backup, restore: restore, download: download };
})();
