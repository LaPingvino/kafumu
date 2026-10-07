// Kafumu device store: everything private lives here, in IndexedDB, and
// never leaves the device unencrypted (VISION.md, tier 1).
(function () {
  "use strict";
  // Each identity has its own store: yourself ("kafumu"), and each business
  // you act as ("kafumu-biz-<id>"), so its card and contacts stay apart.
  var actingID = typeof document !== "undefined" && document.body ? document.body.dataset.actingId || "" : "";
  var DB = actingID ? "kafumu-biz-" + actingID : "kafumu", VERSION = 1, db;
  // A business's managers may sync its card and contacts (its vault):
  // "server" (the server holds the key) or "private" (devices only).
  var actingSync = actingID && document.body ? document.body.dataset.actingSync || "" : "";
  var VAULT = actingID ? "/api/business/" + actingID + "/vault" : "/api/vault";

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
  var FIELDS = ["name", "about", "email", "phone", "whatsapp", "signal", "telegram", "bluesky", "linkedin",
    "instagram", "facebook", "mastodon", "tiktok", "youtube", "website"];

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
    function handle(v) { return (v || "").trim().replace(/^@/, ""); }
    function site(v, base) { return /^https?:|\./.test(handle(v)) && /\//.test(v || "") ? safeURL(/^https?:/.test(v) ? v : "https://" + v.trim()) : base + encodeURIComponent(handle(v)); }
    add("instagram", card.instagram, site(card.instagram, "https://instagram.com/"));
    add("facebook", card.facebook, site(card.facebook, "https://facebook.com/"));
    // Mastodon: @you@server → https://server/@you
    var m = /^@?([^@\s]+)@([^@\s]+\.[^@\s]+)$/.exec((card.mastodon || "").trim());
    add("mastodon", card.mastodon, m ? "https://" + m[2] + "/@" + m[1] : safeURL(card.mastodon));
    add("tiktok", card.tiktok, site(card.tiktok, "https://www.tiktok.com/@"));
    add("youtube", card.youtube, site(card.youtube, "https://www.youtube.com/@"));
    add("website", card.website, safeURL(card.website));
    // Your own fields: a link or an email becomes tappable; the rest is text.
    (card.custom || []).forEach(function (f) {
      var v = (f.value || "").trim(), href = /^[^@\s]+@[^@\s]+\.[^@\s]+$/.test(v) ? "mailto:" + v : /^(https?:\/\/|www\.)/i.test(v) ? safeURL(/^https?:/i.test(v) ? v : "https://" + v) : "";
      if (v && href) out.push({ field: "custom", label: f.label || v, href: href });
    });
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
      // Each persona carries its own updatedAt (for sync); a removed one
      // leaves a tombstone so another device doesn't bring it back.
      var now = new Date().toISOString();
      return Promise.all([store.get("personas"), store.get("tombstones")]).then(function (r) {
        var before = {}, ts = r[1] || {};
        (r[0] || []).forEach(function (p) { before[p.id] = JSON.stringify(Object.assign({}, p, { updatedAt: null })); });
        var keep = {};
        ps.forEach(function (p) {
          keep[p.id] = true;
          if (before[p.id] !== JSON.stringify(Object.assign({}, p, { updatedAt: null }))) p.updatedAt = now;
        });
        Object.keys(before).forEach(function (id) { if (!keep[id]) ts["p:" + id] = now; });
        // Keep "card" mirroring the first persona for older pages and backups.
        return store.set("personas", ps).then(function () { return store.set("tombstones", ts); })
          .then(function () { return store.set("card", (ps[0] && ps[0].card) || {}); });
      }).then(syncSoon);
    },
    // share returns what to hand over: persona p, only the ticked fields.
    share: function (p, fields) {
      var c = p.card || {}, out = { name: c.name };
      (fields || FIELDS).forEach(function (f) { if (f !== "name" && c[f]) out[f] = c[f]; });
      if (c.tags && c.tags.length && (!fields || fields.indexOf("tags") >= 0)) out.tags = c.tags.slice();
      if (c.custom && c.custom.length && (!fields || fields.indexOf("custom") >= 0)) out.custom = c.custom.slice(0, 10);
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
    setChoice: function (personaID, fields) {
      return store.set("shareChoice", { persona: personaID, fields: fields })
        .then(function () { return store.set("personasAt", new Date().toISOString()); }).then(syncSoon);
    },
    shareCard: function () { return personas.choice().then(function (ch) { return personas.share(ch.persona, ch.fields); }); },
    // linkCard: the card people get through your kafumu.com/@name link (the
    // persona you chose for it on My card; else your current choice).
    linkCard: function () {
      return Promise.all([store.get("handle"), personas.list()]).then(function (r) {
        var p = r[0] && !r[0].off && r[1].filter(function (x) { return x.id === r[0].persona; })[0];
        return p ? personas.share(p, null) : personas.shareCard();
      });
    },
    newID: newID,
    SUGGESTED_TAGS: SUGGESTED_TAGS,
    phoneExample: phoneExample
  };

  function el(tag, cls, text) { var e = document.createElement(tag); if (cls) e.className = cls; if (text != null) e.textContent = text; return e; }

  // quietDays: days since anything arrived from this contact (or since you
  // connected, if nothing ever did).
  function quietDays(c) { return Math.floor((Date.now() - new Date(c.lastHeard || c.createdAt || Date.now())) / 864e5); }

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
    if (c.pending && !opts.preview) head.appendChild(el("span", "dim small", "· ⏳ " + (T.pending_send || "waiting to send")));
    var quiet = quietDays(c);
    if (quiet >= 14 && !opts.preview) head.appendChild(el("span", "dim small", "· 💤 " + (T.quiet_for || "quiet for {n} days").replace("{n}", quiet)));
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
      var a = el("a", "pill-sm", l.field === "custom" ? l.label : (T["field_" + l.field] || l.field));
      a.href = l.href; a.target = "_blank"; a.rel = "noopener"; a.setAttribute("role", "button");
      row.appendChild(a);
    });
    li.appendChild(row);
    (card.custom || []).forEach(function (f) {
      var v = (f.value || "").trim();
      if (v && !links({ custom: [f] }).length) li.appendChild(el("p", "dim small", (f.label ? f.label + ": " : "") + v));
    });
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
    // Chat: end-to-end encrypted over the pair's mailbox, stored in the
    // contact (synced to your own devices).
    if (opts.send && !opts.preview) {
      var unread = c.unreadMsgs || 0;
      var chatBtn = el("button", "pill-sm" + (unread ? " suggested" : ""), "💬 " + (T.chat || "Chat") + (unread ? " (" + unread + ")" : ""));
      chatBtn.type = "button";
      var thread = el("div", "chat");
      thread.hidden = true;
      function drawThread() {
        var log = thread.querySelector(".chat-log") || thread.appendChild(el("div", "chat-log"));
        log.textContent = "";
        (c.messages || []).forEach(function (m) {
          var b = el("div", "bubble" + (m.me ? " me" : ""), m.text);
          b.title = new Date(m.at).toLocaleString(window.KAFUMU_LOCALE);
          log.appendChild(b);
        });
        if (!(c.messages || []).length) log.appendChild(el("p", "dim small", T.chat_empty || "No messages yet."));
        log.scrollTop = log.scrollHeight;
      }
      var form = el("form", "chat-form");
      var input = el("input");
      input.placeholder = T.chat_placeholder || "Message";
      input.maxLength = 500;
      var sendBtn = el("button", "pill-sm suggested", T.chat_send || "Send");
      sendBtn.type = "submit";
      form.appendChild(input); form.appendChild(sendBtn);
      form.onsubmit = function (e) {
        e.preventDefault();
        var text = input.value.trim();
        if (!text) return;
        sendBtn.disabled = true;
        var msg = { t: "msg", text: text, at: new Date().toISOString() };
        opts.send(c, msg).then(function () {
          c.messages = (c.messages || []).concat([{ me: true, text: text, at: msg.at }]).slice(-200);
          input.value = "";
          return store.putContact(c);
        }).then(drawThread, function () { input.placeholder = T.network_retry || "Try again"; })
          .then(function () { sendBtn.disabled = false; input.focus(); });
      };
      chatBtn.onclick = function () {
        thread.hidden = !thread.hidden;
        if (thread.hidden) return;
        if (!thread.contains(form)) thread.appendChild(form);
        drawThread();
        if (c.unreadMsgs) { c.unreadMsgs = 0; store.putContact(c); chatBtn.textContent = "💬 " + (T.chat || "Chat"); chatBtn.classList.remove("suggested"); }
        input.focus();
        // While open, look for replies every few seconds.
        (function poll() {
          if (thread.hidden || !document.body.contains(thread) || !opts.check) return;
          opts.check(c).then(function (got) { if (got && got.length) { c.unreadMsgs = 0; store.putContact(c); drawThread(); } })
            .catch(function () {}).then(function () { setTimeout(poll, 4000); });
        })();
      };
      li.appendChild(chatBtn);
      li.appendChild(thread);
    }
    if (opts.duplicateOf) {
      var dup = el("p", "dim small", (T.dup_hint || "Same person as another contact?") + " ");
      var rm = el("button", "pill-sm", T.dup_remove || "Remove this one");
      rm.type = "button";
      rm.onclick = function () { store.deleteContact(c.id).then(function () { li.remove(); if (opts.onDelete) opts.onDelete(c); }); };
      dup.appendChild(rm);
      li.appendChild(dup);
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
    return Promise.all([store.get("card"), store.contacts(), store.get("personas"), store.get("syncKey"), store.get("tombstones")]).then(function (r) {
      return { kafumu: 1, exportedAt: new Date().toISOString(), card: r[0] || {}, contacts: r[1] || [], personas: r[2] || [], syncKey: r[3] || undefined, tombstones: r[4] || {} };
    });
  }
  function restore(data) {
    if (!data || data.kafumu !== 1 || !Array.isArray(data.contacts)) return Promise.reject(new Error("not a Kafumu backup"));
    var theirs = { contacts: data.contacts, personas: data.personas && data.personas.length ? data.personas : (data.card && data.card.name ? [{ id: "imported", label: "", card: data.card }] : []),
      shareChoice: null, personasAt: "", tombstones: data.tombstones || {} };
    return snapshot().then(function (mine) {
      var m = merge(mine, theirs);
      var steps = [writeLocal(mine, m)];
      // A move (or backup) carries the account's sync key: from now on this
      // device syncs with the others by itself.
      if (data.syncKey) steps.push(store.set("syncKey", data.syncKey));
      return Promise.all(steps).then(function () { syncSoon(); return data.contacts.length; });
    });
  }

  function download(name, type, text) {
    var a = document.createElement("a");
    a.href = URL.createObjectURL(new Blob([text], { type: type }));
    a.download = name;
    document.body.appendChild(a); a.click(); a.remove();
    setTimeout(function () { URL.revokeObjectURL(a.href); }, 1000);
  }

  // ---- Sync between your own devices (signed in to the same account) ----
  // One encrypted vault per account on the server (GET/PUT /api/vault); the
  // key is made on the first device and travels to others only inside the
  // account-bound move (or a backup file). Contacts merge per contact, the
  // newer write winning; a deletion is final (tombstone, kept 30 days): a
  // late message for that contact must not bring it back, and connecting
  // again makes a new contact anyway. Personas and the share choice go as
  // one, newer wins.
  var rawPut = store.putContact, rawDel = store.deleteContact, syncTimer = null, syncing = null;
  store.putContact = function (c) { c.updatedAt = new Date().toISOString(); return rawPut(c).then(function (r) { syncSoon(); return r; }); };
  store.deleteContact = function (id) {
    return rawDel(id).then(function () { return store.get("tombstones"); }).then(function (ts) {
      ts = ts || {}; ts[id] = new Date().toISOString();
      return store.set("tombstones", ts);
    }).then(syncSoon);
  };
  // Sync is your own vault: off while acting as a business (its contacts
  // must not land in your personal account; business sync is LOOP-STATE 75c).
  function signedIn() { return typeof document !== "undefined" && !!(document.body && document.body.dataset.signedIn) && (!actingID || !!actingSync); }
  function syncSoon() { if (!signedIn()) return; clearTimeout(syncTimer); syncTimer = setTimeout(function () { sync(); }, 1500); }
  function setSync(state) { window.kafumuSync = state; window.dispatchEvent(new CustomEvent("kafumu:sync", { detail: state })); }
  function b64e(u8) { var s = ""; for (var i = 0; i < u8.length; i++) s += String.fromCharCode(u8[i]); return btoa(s); }
  function b64d(s) { return Uint8Array.from(atob(s), function (c) { return c.charCodeAt(0); }); }
  var AAD = new TextEncoder().encode("kafumu-vault-v1");
  function aes(keyB64) { return crypto.subtle.importKey("raw", b64d(keyB64), "AES-GCM", false, ["encrypt", "decrypt"]); }
  function seal(keyB64, obj) {
    var iv = crypto.getRandomValues(new Uint8Array(12));
    return aes(keyB64).then(function (k) {
      return crypto.subtle.encrypt({ name: "AES-GCM", iv: iv, additionalData: AAD }, k, new TextEncoder().encode(JSON.stringify(obj)));
    }).then(function (ct) { return "v1." + b64e(iv) + "." + b64e(new Uint8Array(ct)); });
  }
  function unseal(keyB64, text) {
    var p = String(text).split(".");
    if (p[0] !== "v1" || p.length !== 3) return Promise.reject(new Error("vault format"));
    return aes(keyB64).then(function (k) {
      return crypto.subtle.decrypt({ name: "AES-GCM", iv: b64d(p[1]), additionalData: AAD }, k, b64d(p[2]));
    }).then(function (pt) { return JSON.parse(new TextDecoder().decode(pt)); });
  }
  function snapshot() {
    return Promise.all([store.contacts(), store.get("personas"), store.get("shareChoice"), store.get("personasAt"), store.get("tombstones"), store.get("chips"), store.get("invite:named"), store.get("handle"), store.get("me"), store.get("publicInbox"), store.get("inboxMsgs")])
      .then(function (r) { return { contacts: r[0] || [], personas: r[1] || [], shareChoice: r[2] || null, personasAt: r[3] || "", tombstones: r[4] || {}, chips: r[5] || null, named: r[6] && r[6].privJwk ? r[6] : null, handle: r[7] || null, me: r[8] || null,
        inbox: r[9] && r[9].privJwk ? r[9] : null, inboxMsgs: r[10] || [] }; });
  }
  function stamp(c) { return (c && (c.updatedAt || c.createdAt)) || ""; }
  function canon(s) {
    var cs = s.contacts.slice().sort(function (a, b) { return a.id < b.id ? -1 : 1; });
    return JSON.stringify([cs, s.personas, s.shareChoice, s.personasAt, Object.keys(s.tombstones).sort().map(function (k) { return [k, s.tombstones[k]]; }), s.chips || null, s.named || null, s.handle || null, s.me || null, s.inbox || null, (s.inboxMsgs || []).map(function (m) { return m.id; }).sort()]);
  }
  function merge(a, b) {
    var cutoff = new Date(Date.now() - 30 * 864e5).toISOString(), ts = {}, byID = {};
    [a.tombstones, b.tombstones].forEach(function (t) { Object.keys(t || {}).forEach(function (id) { if (t[id] > cutoff && (!ts[id] || t[id] > ts[id])) ts[id] = t[id]; }); });
    a.contacts.concat(b.contacts).forEach(function (c) { if (!byID[c.id] || stamp(c) > stamp(byID[c.id])) byID[c.id] = c; });
    var contacts = Object.keys(byID).map(function (id) { return byID[id]; }).filter(function (c) { return !ts[c.id]; });
    // Personas per id, newer wins; ones only one side has are kept (a card
    // made before sync existed has no updatedAt: it still comes across).
    var pByID = {}, order = [];
    (a.personas || []).concat(b.personas || []).forEach(function (p) {
      if (!p || !p.id) return;
      if (!pByID[p.id]) order.push(p.id);
      if (!pByID[p.id] || (p.updatedAt || "") > (pByID[p.id].updatedAt || "")) pByID[p.id] = p;
    });
    var personas = order.filter(function (id) { return !ts["p:" + id]; }).map(function (id) { return pByID[id]; })
      // Drop only blank personas nobody ever edited (the empty one a fresh
      // device makes by itself), so a card you just started stays.
      .filter(function (p, i, all) { return all.length === 1 || p.updatedAt || (p.card && (p.card.name || p.card.about)) || p.label; });
    // Your chip row (pinned and hidden subjects): newer wins.
    var chips = ((b.chips && b.chips.at) || "") > ((a.chips && a.chips.at) || "") ? b.chips : (a.chips || b.chips || null);
    // Your named link: its key (newest code) and its setting (newest change,
    // including turning it off) are the same on all your devices.
    var named = ((b.named && b.named.createdAt) || 0) > ((a.named && a.named.createdAt) || 0) ? b.named : (a.named || b.named || null);
    var handle = ((b.handle && b.handle.at) || 0) > ((a.handle && a.handle.at) || 0) ? b.handle : (a.handle || b.handle || null);
    // "me" (the person id sent in hellos and cards): the oldest one wins, so
    // all your devices present one person.
    var me = !a.me ? b.me : !b.me ? a.me : (a.me.createdAt || "") <= (b.me.createdAt || "") ? a.me : b.me;
    // The public inbox (its key: newest wins; it lives on every device now)
    // and the messages it got (union by id, newest first, 100 kept): a
    // device that read them took them off the server.
    var inbox = ((b.inbox && b.inbox.createdAt) || 0) > ((a.inbox && a.inbox.createdAt) || 0) ? b.inbox : (a.inbox || b.inbox || null);
    var seenMsg = {}, inboxMsgs = (a.inboxMsgs || []).concat(b.inboxMsgs || []).filter(function (m) { if (!m || seenMsg[m.id]) return false; seenMsg[m.id] = true; return true; })
      .sort(function (x, y) { return String(y.at).localeCompare(String(x.at)); }).slice(0, 100);
    return { contacts: contacts, personas: personas, shareChoice: a.shareChoice || b.shareChoice, personasAt: "", tombstones: ts, chips: chips, named: named, handle: handle, me: me || null, inbox: inbox, inboxMsgs: inboxMsgs };
  }
  function writeLocal(local, m) {
    var keep = {};
    m.contacts.forEach(function (c) { keep[c.id] = true; });
    var steps = m.contacts.map(function (c) { return rawPut(c); });
    local.contacts.forEach(function (c) { if (!keep[c.id]) steps.push(rawDel(c.id)); });
    steps.push(store.set("personas", m.personas), store.set("shareChoice", m.shareChoice), store.set("personasAt", m.personasAt), store.set("tombstones", m.tombstones));
    if (m.personas && m.personas[0]) steps.push(store.set("card", m.personas[0].card || {}));
    if (m.named) steps.push(store.set("invite:named", m.named));
    if (m.inbox) steps.push(store.set("publicInbox", m.inbox));
    if (m.inboxMsgs && m.inboxMsgs.length) steps.push(store.set("inboxMsgs", m.inboxMsgs));
    if (m.me) steps.push(store.set("me", m.me));
    if (m.handle) steps.push(store.set("handle", m.handle));
    if (m.chips) { steps.push(store.set("chips", m.chips)); try { localStorage.setItem("kafumu.chips", JSON.stringify(m.chips)); } catch (e) {} }
    return Promise.all(steps);
  }
  function sync(retry) {
    if (!signedIn()) return Promise.resolve("off");
    if (syncing && !retry) return syncing;
    var run = fetch(VAULT, { credentials: "same-origin" }).then(function (r) { if (!r.ok) throw new Error("off"); return r.json(); }).then(function (v) {
      return Promise.all([serverKey(), snapshot()]).then(function (r) {
        var key = r[0], local = r[1];
        if (!v.data && !key) {
          // First device of this account: make the key.
          key = b64e(crypto.getRandomValues(new Uint8Array(32)));
          return store.set("syncKey", key).then(function () { return push(key, local, v.version); });
        }
        if (!key) return actingSync === "private" ? bizKey.request().then(function (k) { return k ? "conflict" : "needs-key"; }) : "needs-key";
        if (!v.data) return push(key, local, v.version);
        return unseal(key, v.data).then(function (remote) {
          var m = merge(local, remote);
          var toLocal = canon(m) !== canon(local), toRemote = canon(m) !== canon(merge(remote, remote));
          return (toLocal ? writeLocal(local, m).then(function () { window.dispatchEvent(new CustomEvent("kafumu:synced")); }) : Promise.resolve())
            .then(function () { return toRemote ? push(key, m, v.version) : "on"; });
        }, function () { return "needs-key"; });
      });
    }).then(function (state) {
      if (state === "conflict" && !retry) return sync(true);
      setSync(state === "conflict" ? "on" : state);
      return state;
    }).catch(function () { setSync("off"); return "off"; });
    if (!retry) { syncing = run; run.then(function () { syncing = null; }); }
    return run;
  }
  // serverKey: the vault key on this device; for a business in server mode,
  // fetched from the server (kept here too).
  function serverKey() {
    return store.get("syncKey").then(function (have) {
      if (actingSync !== "server") return have;
      return fetch("/api/business/" + actingID + "/key", { credentials: "same-origin" })
        .then(function (r) { return r.ok ? r.json() : {}; })
        .then(function (j) { return j.key && j.key !== have ? store.set("syncKey", j.key).then(function () { return j.key; }) : have; }, function () { return have; });
    });
  }
  function push(key, snap, version) {
    return seal(key, snap).then(function (text) {
      return fetch(VAULT, { method: "PUT", credentials: "same-origin", headers: { "If-Match": String(version || 0) }, body: text });
    }).then(function (r) { return r.status === 409 ? "conflict" : r.ok ? "on" : "off"; });
  }
  // On every page, signed in: say when this device still needs the key
  // (get it once), or when another device of yours is asking for it (send).
  function bar(text, label, href, dismissKey) {
    if (document.getElementById("sync-bar") || (!dismissKey && /^\/contacts/.test(location.pathname))) return;
    var d = document.createElement("div");
    d.id = "sync-bar"; d.className = "install-bar";
    var t = document.createElement("span"); t.className = "text"; t.textContent = text;
    var a = document.createElement("a"); a.className = "go pill-sm"; a.href = href; a.textContent = label; a.setAttribute("role", "button");
    d.appendChild(t); d.appendChild(a);
    if (dismissKey) {
      var x = document.createElement("button");
      x.type = "button"; x.className = "no pill-sm"; x.textContent = "×";
      x.setAttribute("aria-label", (window.KAFUMU_T || {}).oln_hide || "Hide"); // a screen reader says "Hide", not "×"
      x.onclick = function () { try { localStorage.setItem(dismissKey, String(Date.now())); } catch (e) {} d.remove(); };
      d.appendChild(x);
    }
    var head = document.querySelector(".headerbar");
    if (head) head.after(d); else document.body.prepend(d);
  }
  if (window.addEventListener) window.addEventListener("kafumu:sync", function (e) {
    var T = window.KAFUMU_T || {};
    if (e.detail === "needs-key" && actingSync === "private") { if (!/^\/business/.test(location.pathname)) bar(T.bizkey_bar_need || "This device needs the business key.", T.bizkey_open || "Open", "/business"); }
    else if (e.detail === "needs-key") bar(T.sync_needs_key || "Your cards and contacts are on your other device.", T.sync_get || "Get them", "/contacts#get");
    if (e.detail === "on" && actingSync === "private" && !/^\/business/.test(location.pathname)) bizKey.pending().then(function (rs) {
      if (rs.length) bar(T.bizkey_bar_asks || "A manager asks for the business key.", T.bizkey_open || "Open", "/business");
    });
  });

  // ---- The business key in private mode (LOOP-STATE 75c-2) ----
  // A device without it asks with a one-off ECDH key; a manager's device
  // that has it seals it to that key, once both screens show the same code
  // (derived from the asking device's public key, so a key swapped in by
  // the server would show a different one). The server sees public keys
  // and ciphertext only.
  var bizKey = (function () {
    var ECDH = { name: "ECDH", namedCurve: "P-256" }, subtle = crypto.subtle, enc = new TextEncoder();
    var api = "/api/business/" + actingID;
    function code(pubB64) {
      return subtle.digest("SHA-256", b64d(pubB64)).then(function (h) {
        var n = new DataView(h).getUint32(0) % 1000000, s = String(n).padStart(6, "0");
        return s.slice(0, 3) + " " + s.slice(3);
      });
    }
    function aes(priv, theirRaw, salt) {
      return subtle.importKey("raw", theirRaw, ECDH, false, []).then(function (pub) { return subtle.deriveBits({ name: "ECDH", public: pub }, priv, 256); })
        .then(function (bits) { return subtle.importKey("raw", bits, "HKDF", false, ["deriveKey"]); })
        .then(function (ikm) { return subtle.deriveKey({ name: "HKDF", hash: "SHA-256", salt: salt, info: enc.encode("kafumu bizkey v1") }, ikm, { name: "AES-GCM", length: 256 }, false, ["encrypt", "decrypt"]); });
    }
    function cat(a, b) { var o = new Uint8Array(a.length + b.length); o.set(a); o.set(b, a.length); return o; }
    function list(pub) { return fetch(api + "/keyreq?pub=" + encodeURIComponent(pub || ""), { credentials: "same-origin", cache: "no-store" }).then(function (r) { return r.ok ? r.json() : []; }); }
    function post(path, form) { return fetch(api + path, { method: "POST", credentials: "same-origin", body: new URLSearchParams(form) }); }
    // mine: this device's request (made once, kept in the business store).
    function mine() {
      return store.get("bizKeyReq").then(function (q) {
        if (q) return q;
        return subtle.generateKey(ECDH, true, ["deriveBits"]).then(function (k) {
          return Promise.all([subtle.exportKey("raw", k.publicKey), subtle.exportKey("jwk", k.privateKey)]).then(function (r) {
            q = { pub: b64e(new Uint8Array(r[0])), privJwk: r[1] };
            return store.set("bizKeyReq", q).then(function () { return q; });
          });
        });
      });
    }
    // request: ask (or check the answer); resolves to the key once it came.
    function request() {
      return mine().then(function (q) {
        return list(q.pub).then(function (rs) {
          var me = rs.filter(function (x) { return x.mine; })[0];
          if (!me) return post("/keyreq", { pub: q.pub }).then(function () { return null; });
          if (!me.wrapped) return null;
          var parts = me.wrapped.split("."), eph = b64d(parts[0]), box = b64d(parts[1]);
          return subtle.importKey("jwk", q.privJwk, ECDH, false, ["deriveBits"]).then(function (priv) { return aes(priv, eph, cat(eph, b64d(q.pub))); })
            .then(function (k) { return subtle.decrypt({ name: "AES-GCM", iv: box.slice(0, 12) }, k, box.slice(12)); })
            .then(function (pt) {
              var key = new TextDecoder().decode(pt);
              return store.set("syncKey", key).then(function () { return store.set("bizKeyReq", null); }).then(function () { return key; });
            }, function () { return null; });
        });
      });
    }
    // cleanup: a device that has the key withdraws a request it left behind
    // (it asked before the key existed, or got it by another way).
    function cleanup() {
      return Promise.all([store.get("syncKey"), store.get("bizKeyReq")]).then(function (r) {
        if (!r[0] || !r[1]) return;
        return post("/keyreq", { pub: r[1].pub, cancel: "1" }).then(function () { return store.set("bizKeyReq", null); }, function () {});
      });
    }
    // pending: other devices' open requests, with their codes.
    function pending() {
      return store.get("syncKey").then(function (have) {
        if (!have || actingSync !== "private") return [];
        return cleanup().then(function () { return list(); }).then(function (rs) {
          rs = rs.filter(function (x) { return !x.mine && !x.wrapped; });
          return Promise.all(rs.map(function (x) { return code(x.pub).then(function (c) { x.code = c; return x; }); }));
        });
      });
    }
    // grant: seal the key to that request's public key and send it.
    function grant(req) {
      return store.get("syncKey").then(function (key) {
        if (!key) throw new Error("no key on this device");
        var theirs = b64d(req.pub);
        return subtle.generateKey(ECDH, true, ["deriveBits"]).then(function (k) {
          return subtle.exportKey("raw", k.publicKey).then(function (ephRaw) {
            ephRaw = new Uint8Array(ephRaw);
            var iv = crypto.getRandomValues(new Uint8Array(12));
            return aes(k.privateKey, theirs, cat(ephRaw, theirs)).then(function (ak) { return subtle.encrypt({ name: "AES-GCM", iv: iv }, ak, enc.encode(key)); })
              .then(function (ct) { return post("/keygrant", { pub: req.pub, wrapped: b64e(ephRaw) + "." + b64e(cat(iv, new Uint8Array(ct))) }); });
          });
        });
      }).then(function (r) { if (!r.ok) throw new Error("grant " + r.status); });
    }
    return { code: code, mine: mine, request: request, pending: pending, grant: grant, cleanup: cleanup };
  })();
  function askedForKey() {
    Promise.all([fetch("/account/move", { credentials: "same-origin" }).then(function (r) { return r.ok ? r.json() : {}; }), store.get("invite:move"), store.get("syncKey")])
      .then(function (r) {
        var req = r[0], mine = r[1], T = window.KAFUMU_T || {};
        if (req.box && !(mine && mine.box === req.box) && r[2]) bar(T.sync_asked || "Another of your devices asks for your cards and contacts.", T.sync_send || "Send", "/contacts");
      }).catch(function () {});
  }
  if (signedIn()) { setTimeout(function () { sync(); }, 300); if (!actingID) setTimeout(askedForKey, 800); }
  // Something worth keeping here (contacts, a card) but no way back in if
  // this device is lost: suggest an account, a username and a passkey.
  // Dismissed, it stays away for a week.
  function suggestAccount() {
    if (typeof document === "undefined" || !document.body || /^\/(account|findable|business)/.test(location.pathname)) return;
    var b = document.body.dataset, key = "kafumu.accountNudge";
    if (b.acting) { // using Kafumu as a business: thanks instead (Joop)
      var T0 = window.KAFUMU_T || {}, k2 = "kafumu.bizNudge";
      try { if (Date.now() - parseInt(localStorage.getItem(k2) || "0", 10) < 7 * 864e5) return; } catch (e) { return; }
      bar((T0.nudge_biz || "Thanks for creating a business account! You're using Kafumu as {name}.").replace("{name}", b.acting), T0.nudge_biz_go || "Manage", "/business", k2);
      return;
    }
    try { if (Date.now() - parseInt(localStorage.getItem(key) || "0", 10) < 7 * 864e5) return; } catch (e) { return; }
    if (b.signedIn && b.named && b.passkey) return;
    Promise.all([store.contacts(), store.get("card")]).then(function (r) {
      var n = (r[0] || []).length, hasCard = !!(r[1] && r[1].name);
      if (!n && !hasCard) return;
      var T = window.KAFUMU_T || {};
      var text = !b.signedIn ? (T.nudge_account || "Save your contacts and cards: make an account with a passkey, so you can get them back on another device.")
        : (T.nudge_secure || "Add a username and a passkey, so you can always get back into your account.");
      bar(text, T.nudge_go || "Set it up", "/account", key);
    }).catch(function () {});
  }
  if (typeof document !== "undefined") setTimeout(suggestAccount, 1500);
  else if (typeof document !== "undefined") document.addEventListener("DOMContentLoaded", function () { if (signedIn()) sync(); });

  window.kafumuDevice = { bizKey: bizKey, actingSync: actingSync,  signalText: signalText, store: store, FIELDS: FIELDS, links: links, renderContact: renderContact, personas: personas,
    vcards: vcards, backup: backup, restore: restore, download: download, sync: sync, quietDays: quietDays,
    // chips: your filter-chip row ({pinned: [...], hidden: [...], at}); a
    // localStorage mirror lets Around draw it without waiting.
    chips: {
      get: function () { try { return JSON.parse(localStorage.getItem("kafumu.chips") || "null") || { pinned: [], hidden: [] }; } catch (e) { return { pinned: [], hidden: [] }; } },
      set: function (c) {
        c.at = new Date().toISOString();
        try { localStorage.setItem("kafumu.chips", JSON.stringify(c)); } catch (e) {}
        return store.set("chips", c).then(syncSoon);
      }
    } };
})();
