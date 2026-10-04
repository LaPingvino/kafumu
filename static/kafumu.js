// Kafumu client. The cell is computed here; coordinates never leave the device.
(function () {
  "use strict";
  var G = window.kafumuGeo, cell = G.cell, rings = G.rings, validCell = G.validCell, parsePlace = G.parsePlace;

  var $ = function (id) { return document.getElementById(id); };

  // tr looks up a UI string handed over by the server (window.KAFUMU_T).
  function tr(key, vars) {
    var s = (window.KAFUMU_T || {})[key] || key;
    Object.keys(vars || {}).forEach(function (k) { s = s.split("{" + k + "}").join(vars[k]); });
    return s;
  }

  function setStatus(msg) { $("status").textContent = msg; }
  function note(msg) { $("list-note").textContent = msg; }

  // ---- One feed: every kind of card in one list, ranked on the device ----
  // Each renderer fills a "sink" (list-like: textContent = "" clears it,
  // appendChild adds a card); drawFeed merges the sinks by score. A card's
  // score is li.dataset.score if the renderer set one, else its kind's base
  // minus a step per position (keeping that kind's own order).
  var KINDS = ["contacts", "here", "meetups", "people", "posts"];
  // Sinks by source; friends around and signals both count as "contacts".
  var KIND_OF = { friends: "contacts", signals: "contacts", here: "here", meetups: "meetups", people: "people", posts: "posts", elsewhere: "posts" };
  var BASE = { contacts: 1000, here: 10, meetups: 7, people: 4, posts: 3 }, STEP = { contacts: 1, here: 0.5, meetups: 0.6, people: 0.8, posts: 0.15 };
  var feed = {}, drawTimer = null;
  Object.keys(KIND_OF).forEach(function (k) { feed[k] = []; });
  function feedKinds() {
    var on = null;
    try { on = JSON.parse(localStorage.getItem("kafumu.feedKinds") || "null"); } catch (e) {}
    return Array.isArray(on) && on.length ? on : KINDS.slice();
  }
  function setFeedKinds(on) { try { localStorage.setItem("kafumu.feedKinds", JSON.stringify(on)); } catch (e) {} drawFeed(); }
  function sink(src) {
    var kind = KIND_OF[src];
    var s = { hidden: false };
    Object.defineProperty(s, "textContent", { get: function () { return feed[src].map(function (li) { return li.textContent; }).join(" "); },
      set: function () { feed[src] = []; scheduleDraw(); } });
    Object.defineProperty(s, "innerHTML", { set: function () { feed[src] = []; scheduleDraw(); } });
    Object.defineProperty(s, "children", { get: function () { return feed[src]; } });
    s.appendChild = function (li) {
      var i = feed[src].length;
      li.dataset.kind = kind;
      li.dataset.label = tr("kind_" + kind);
      if (li.dataset.score === undefined) li.dataset.score = BASE[kind] - STEP[kind] * i;
      feed[src].push(li);
      scheduleDraw();
      return li;
    };
    s.prepend = function (li) { s.appendChild(li); };
    return s;
  }
  var sinks = {};
  Object.keys(KIND_OF).forEach(function (k) { sinks[k] = sink(k); });
  var noSection = { hidden: false };
  function cardsOf(kind) {
    var out = [];
    Object.keys(KIND_OF).forEach(function (src) { if (KIND_OF[src] === kind) out = out.concat(feed[src]); });
    return out;
  }
  function scheduleDraw() { clearTimeout(drawTimer); drawTimer = setTimeout(drawFeed, 0); }
  function drawFeed() {
    var on = feedKinds(), all = on.length === KINDS.length, ul = $("feed"), box = $("feed-kinds");
    // Chips: All, then each kind that has cards here, with its count.
    box.textContent = "";
    function chip(label, pressed, onclick) {
      var b = document.createElement("button");
      b.type = "button"; b.className = "chip" + (pressed ? " on" : ""); b.textContent = label;
      b.setAttribute("aria-pressed", pressed); b.onclick = onclick;
      box.appendChild(b);
    }
    chip(tr("kind_all"), all, function () { setFeedKinds(KINDS.slice()); });
    KINDS.forEach(function (k) {
      var n = cardsOf(k).filter(function (li) { return !li.classList.contains("muted"); }).length;
      if (!n) return;
      var pressed = !all && on.indexOf(k) >= 0;
      chip(tr("kind_" + k) + " " + n, pressed, function () {
        // From All, a kind chip shows just that kind; after that, chips
        // add and remove kinds; none left means All again.
        if (all) { setFeedKinds([k]); return; }
        var next = pressed ? on.filter(function (x) { return x !== k; }) : on.concat([k]);
        setFeedKinds(next.length ? next : KINDS.slice());
      });
    });
    var items = [];
    KINDS.forEach(function (k) { if (on.indexOf(k) >= 0) items = items.concat(cardsOf(k)); });
    var real = items.filter(function (li) { return !li.classList.contains("muted"); });
    if (real.length) items = real;
    items.sort(function (a, z) { return parseFloat(z.dataset.score) - parseFloat(a.dataset.score); });
    ul.classList.toggle("mixed", on.length > 1);
    ul.textContent = "";
    items.forEach(function (li) { attachReplies(li); ul.appendChild(li); });
  }

  // friendsAround: check in (only from a real location fix) and show which
  // of your contacts were in or next to this cell this week.
  function friendsAround(c, gps) {
    var dev = window.kafumuDevice;
    if (!dev || !window.kafumuPair) return;
    var pair = window.kafumuPair.create({ fetch: window.fetch.bind(window), store: dev.store, origin: location.origin });
    dev.store.contacts().then(function (cs) {
      cs = cs.filter(function (x) { return x.card; });
      if (!cs.length) return;
      return (gps ? pair.checkIn(c, cs) : Promise.resolve()).then(function () {
        return pair.around(rings(c, 1).map(function (p) { return p[0]; }), cs.slice(0, 30));
      }).then(function (hits) {
        var sec = noSection, list = sinks.friends;
        sec.hidden = !hits.length;
        travel.friends = hits.length;
        drawTravel();
        // Remember where each friend was last seen (on this device only), for
        // "Nearest" in Contacts.
        hits.forEach(function (h) {
          var c = h.contact, ls = c.lastSeen || {};
          if (!ls.day || h.day > ls.day || (h.day === ls.day && !h.near)) {
            c.lastSeen = { cell: h.cell, day: h.day };
            window.kafumuDevice.store.putContact(c);
          }
        });
        list.textContent = "";
        var today = new Date().toISOString().slice(0, 10), yesterday = new Date(Date.now() - 864e5).toISOString().slice(0, 10);
        hits.forEach(function (h) {
          var li = document.createElement("li"), a = document.createElement("a");
          a.href = "/contacts";
          a.className = "meetup-row";
          var who = document.createElement("strong");
          who.textContent = h.contact.card.name;
          var when = h.day === today ? tr("today") : h.day === yesterday ? tr("yesterday")
            : new Date(h.day + "T12:00:00Z").toLocaleDateString(window.KAFUMU_LOCALE, { weekday: "long" });
          var meta = document.createElement("div");
          meta.className = "meta";
          meta.textContent = tr(h.near ? "was_nearby" : "was_here", { when: when });
          a.appendChild(who); a.appendChild(meta); li.appendChild(a); list.appendChild(li);
        });
      });
    }).catch(function () {});
  }

  // Travel: your "home" cell is the one you've used most; somewhere more
  // than 50 km away gets a short "you're in …" summary on top. All local.
  function homeCell(c, gps) {
    var counts = {};
    try { counts = JSON.parse(localStorage.getItem("kafumu.cellCounts") || "{}"); } catch (e) {}
    if (gps) {
      counts[c] = (counts[c] || 0) + 1;
      try { localStorage.setItem("kafumu.cellCounts", JSON.stringify(counts)); } catch (e) {}
    }
    var best = null;
    Object.keys(counts).forEach(function (k) { if (!best || counts[k] > counts[best]) best = k; });
    return best;
  }
  function kmBetween(a, b) {
    var p = window.kafumuGeo.center(a), q = window.kafumuGeo.center(b), r = Math.PI / 180;
    var x = (q[1] - p[1]) * r * Math.cos((p[0] + q[0]) / 2 * r), y = (q[0] - p[0]) * r;
    return Math.sqrt(x * x + y * y) * 6371;
  }
  var travel = { place: "", friends: 0, meetups: 0, away: false };
  function drawTravel() {
    var el = $("travel");
    if (!el) return;
    el.hidden = !travel.away;
    if (!travel.away) return;
    var parts = [tr("travel", { place: travel.place || $("cell-tag").textContent })];
    if (travel.friends) parts.push(tr("travel_friends", { n: travel.friends }));
    if (travel.meetups) parts.push(tr("travel_meetups", { n: travel.meetups }));
    el.textContent = parts.join(" ");
    // Travelling: offer to be findable here for a while, and a look at your card.
    var p = document.createElement("p");
    p.textContent = tr("travel_visible") + " ";
    var go = document.createElement("a");
    go.setAttribute("role", "button"); go.className = "pill-sm suggested";
    go.href = "/findable?cell=" + currentCell; go.textContent = tr("travel_be_visible");
    var card = document.createElement("a");
    card.setAttribute("role", "button"); card.className = "pill-sm";
    card.href = "/card"; card.textContent = tr("travel_card");
    p.appendChild(go); p.appendChild(document.createTextNode(" ")); p.appendChild(card);
    el.appendChild(p);
  }

  // countLocalTags keeps a small, decaying tally of the tags seen around
  // here (on the device only), so the card editor can suggest local themes.
  function countLocalTags(b) {
    var counts = {}, seenIDs = [];
    try { counts = JSON.parse(localStorage.getItem("kafumu.localTags") || "{}"); seenIDs = JSON.parse(localStorage.getItem("kafumu.localTagIDs") || "[]"); } catch (e) {}
    var seen = {};
    seenIDs.forEach(function (id) { seen[id] = true; });
    var places = {};
    (b.places || []).forEach(function (p) { places[p.tag] = true; });
    // Each item counts once; Kafumu's own signals (local messages, meetups,
    // people) weigh more than Bluesky posts, which often carry many tags.
    function add(id, tags, weight, max) {
      if (!id || seen[id]) return;
      seen[id] = true; seenIDs.push(id);
      (tags || []).slice(0, max).forEach(function (t) {
        t = String(t || "").toLowerCase().replace(/^#/, "");
        if (!t || t.length > 30 || /^\d+$/.test(t) || places[t] ||
            /^(geo|lang[a-z]{3}$|re[0-9a-f]{10}$|ask$|coffee$)/.test(t)) return;
        counts[t] = (counts[t] || 0) + weight;
      });
    }
    (b.posts || []).forEach(function (p) { if (!p.bot) add(p.uri, p.tags, 0.3, 3); });
    (b.meetups || []).forEach(function (m) { add("m" + m.id, m.tags, 3, 8); });
    (b.notes || []).forEach(function (n) { add("n" + n.id, n.tags, 3, 8); });
    (b.people || []).forEach(function (p) { add("p" + p.name, p.tags, 3, 8); });
    var top = Object.keys(counts).sort(function (a, z) { return counts[z] - counts[a]; }).slice(0, 60), keep = {};
    top.forEach(function (t) { keep[t] = Math.round(counts[t] * 10) / 10; });
    try {
      localStorage.setItem("kafumu.localTags", JSON.stringify(keep));
      localStorage.setItem("kafumu.localTagIDs", JSON.stringify(seenIDs.slice(-800)));
    } catch (e) {}
  }

  // ---- Views: place + language + interest, filtered on the device ----
  var params = new URLSearchParams(location.search);
  var view = { lang: params.get("lang") || "", tag: (params.get("tag") || "").toLowerCase().replace(/^#/, ""),
    // 0 off · 1 boost · 2 strong boost · 3 only matching
    strength: Math.max(0, Math.min(3, parseInt(params.get("w") || "2", 10))) };
  function viewURL(c) {
    var q = new URLSearchParams();
    if (c) q.set("cell", c);
    if (view.lang) q.set("lang", view.lang);
    if (view.tag) q.set("tag", view.tag);
    if ((view.lang || view.tag) && view.strength !== 2) q.set("w", String(view.strength));
    return "/?" + q.toString();
  }
  // langTagsFor: every hashtag that means language code (lang:epo → #langepo, #esperanto…).
  function langTagsFor(code) {
    var me = window.KAFUMU_ME || {}, out = { ["lang" + code]: true };
    Object.keys(me.langTags || {}).forEach(function (t) { if ((me.langTags[t].codes || []).indexOf(code) >= 0) out[t] = true; });
    return out;
  }
  // Interests linked like places: a tag matches its whole group.
  var TAG_GROUPS = [["opensource", "foss", "floss", "oss", "freesoftware"], ["ai", "artificialintelligence", "machinelearning", "ml", "genai", "llm"],
    ["coffee", "cafe", "café", "koffie", "kafo", "kaffee"], ["startup", "startups", "founders", "entrepreneur", "entrepreneurship"],
    ["photography", "photo", "fotografia", "fotografie", "photographie"], ["music", "musica", "música", "muziek", "musik", "livemusic"],
    ["running", "run", "hardlopen", "corrida", "laufen"], ["hiking", "wandelen", "caminhadas", "trekking", "wandern"],
    ["climate", "climatechange", "climatecrisis", "sustainability"], ["languages", "languagelearning", "polyglot", "languageexchange", "tandem"],
    ["websummit", "websummit2026", "ws26"], ["esperanto", "esperantujo"], ["design", "ux", "ui", "productdesign"],
    ["crypto", "bitcoin", "web3", "blockchain"], ["food", "foodie", "comida", "eten"], ["art", "arte", "kunst"]];
  function tagGroup(t) {
    t = String(t || "").toLowerCase();
    for (var i = 0; i < TAG_GROUPS.length; i++) if (TAG_GROUPS[i].indexOf(t) >= 0) return TAG_GROUPS[i];
    return [t];
  }

  // filterBundle marks what matches the view (_match) and, at full strength,
  // keeps only that; lower strengths become a ranking bias.
  function filterBundle(b) {
    if ((!view.lang && !view.tag) || view.strength === 0) return b;
    var only = view.strength === 3;
    var me = window.KAFUMU_ME || {}, lt = view.lang ? langTagsFor(view.lang) : null, tag = view.tag;
    function tagsOK(tags, text) {
      tags = (tags || []).map(function (t) { return String(t).toLowerCase(); });
      if (lt && !tags.some(function (t) { return lt[t]; })) return false;
      if (tag && !tagGroup(tag).some(function (g) { return tags.indexOf(g) >= 0 || new RegExp("#" + g.replace(/[^\p{L}\p{N}_]/gu, "") + "\\b", "iu").test(text || ""); })) return false;
      return true;
    }
    function mark(list, ok) {
      list = (list || []).map(function (x) { x._match = ok(x); return x; });
      if (only) return list.filter(function (x) { return x._match; });
      // Bias: matching items first, otherwise keep the existing order.
      return list.map(function (x, i) { return [x, i]; }).sort(function (a, z) {
        return (z[0]._match - a[0]._match) * view.strength || a[1] - z[1];
      }).map(function (p) { return p[0]; });
    }
    b.posts = mark(b.posts, function (p) {
      var langOK = lt && (p.langs || []).some(function (l) { return (me.from1 || {})[l.slice(0, 2)] === view.lang; });
      if (langOK && !tag) return true;
      return tagsOK(p.tags, p.text) || (langOK && tagsOK((p.tags || []).concat(["lang" + view.lang]), p.text));
    });
    b.meetups = mark(b.meetups, function (m) { return tagsOK(m.tags, m.title + " " + (m.text || "")); });
    b.notes = mark(b.notes, function (n) { return tagsOK(n.tags, n.text); });
    b.people = mark(b.people, function (p) {
      if (view.lang && !(p.langs || []).some(function (l) { return l.split("/")[0] === view.lang; })) return false;
      return !tag || tagGroup(tag).some(function (g) { return (p.tags || []).map(function (x) { return String(x).toLowerCase(); }).indexOf(g) >= 0; });
    });
    return b;
  }
  function savedViews() { try { return JSON.parse(pref("kafumu.views") || "[]"); } catch (e) { return []; } }
  function drawViews() {
    var box = $("views"), vs = savedViews(), cur = viewURL(currentCell);
    box.textContent = "";
    vs.forEach(function (v, i) {
      var a = document.createElement("a");
      a.className = "chip" + (v.url === cur ? " on" : "");
      a.href = v.url; a.textContent = "☆ " + v.name;
      a.oncontextmenu = function (e) { e.preventDefault(); if (confirm(tr("view_remove", { name: v.name }))) { vs.splice(i, 1); pref("kafumu.views", JSON.stringify(vs)); drawViews(); } };
      box.appendChild(a);
    });
    if (view.lang || view.tag) {
      var clear = document.createElement("a");
      clear.className = "chip"; clear.href = viewURL(currentCell).replace(/&?(lang|tag)=[^&]*/g, "").replace("?&", "?");
      clear.textContent = "× " + [view.lang ? ((window.KAFUMU_ME || {}).names || {})[view.lang] || view.lang : "", view.tag ? "#" + view.tag : ""].filter(Boolean).join(" ");
      box.appendChild(clear);
    }
  }

  // ---- Local messages (OLN): Kafumu's own channel, first class ----
  var currentCell = "", requiredBits = 12, liveEvents = [], liveEventNames = [];
  // sayPlaceholder fits the moment: where (a running event, the place, or
  // just "around here") and what (coffee, lunch, a break, a drink).
  function sayPlaceholder() {
    var place = ($("place-name").textContent || "").split(",")[0].trim();
    var where = liveEventNames[0] ? tr("say_at", { event: liveEventNames[0] }) : place ? tr("say_in", { place: place }) : tr("say_here");
    var h = new Date().getHours();
    var slot = h < 11 ? "morning" : h < 14 ? "lunch" : h < 17 ? "afternoon" : "evening";
    return tr("say_ph_" + slot, { where: where });
  }
  function hashrate() { var r = parseFloat(pref("kafumu.hashrate") || "0"); return r > 1000 ? r : 40000; }
  function estimate(bits) {
    var s = Math.pow(2, bits) / hashrate();
    return s < 1 ? "< 1 s" : s < 90 ? Math.round(s) + " s" : Math.round(s / 60) + " min";
  }
  function olnKeywords() {
    var me = window.KAFUMU_ME || {}, f = $("oln-form");
    var code = (f && f.lang.value) || (me.from1 || {})[document.documentElement.lang];
    var ks = ["#geo" + currentCell];
    if (code) ks.push("#lang" + code);
    ((f && f.tags.value) || "").split(/[,\s]+/).forEach(function (t) {
      t = t.trim().toLowerCase().replace(/^#/, "").replace(/[^\p{L}\p{N}_]/gu, "");
      if (t && ks.length < 10) ks.push("#" + t);
    });
    liveEvents.forEach(function (t) { ks.push("#" + t); });
    return ks.join(" ");
  }
  function updateEstimate() {
    var f = $("oln-form");
    if (f) $("oln-status").textContent = tr("oln_cost", { time: estimate(requiredBits + parseInt(f.extra.value, 10)) });
  }
  // The composer has three modes: say, ask (a question with a connect code
  // for private answers), and a public answer to a question.
  var composeMode = {};
  function openComposer(mode) {
    composeMode = mode || {};
    var f = $("oln-form");
    f.hidden = false;
    $("oln-mode").textContent = composeMode.ask ? "❓ " + tr("ask_label") : composeMode.re ? "💬 " + tr("ask_answering", { q: (composeMode.about || "").replace(/https?:\/\/\S+/, "").slice(0, 80) }) : "";
    f.text.placeholder = composeMode.ask ? tr("ask_placeholder") : sayPlaceholder();
    if (!f.lang.value) f.lang.value = view.lang || ((window.KAFUMU_ME || {}).from1 || {})[document.documentElement.lang] || "";
    if (!f.tags.value && view.tag) f.tags.value = view.tag;
    updateEstimate(); f.text.focus();
    f.scrollIntoView({ block: "nearest" });
  }
  $("ask").onclick = function () { if ($("oln-form").hidden || !composeMode.ask) openComposer({ ask: true }); else $("oln-form").hidden = true; };
  $("say").onclick = function () {
    var f = $("oln-form");
    if (!f.hidden && !composeMode.ask && !composeMode.re) { f.hidden = true; return; }
    openComposer({}); // starts from the current view's language and interest
  };
  $("oln-form").extra.onchange = updateEstimate;
  $("oln-form").addEventListener("submit", function (e) {
    e.preventDefault();
    var f = this, text = f.text.value.trim(), bits = requiredBits + parseInt(f.extra.value, 10);
    if (!text || !currentCell) return;
    f.querySelector("button[type=submit]").disabled = true;
    var t0 = Date.now(), keywords = olnKeywords(), ready = Promise.resolve();
    if (composeMode.re) keywords += " #re" + composeMode.re.slice(0, 10);
    if (composeMode.ask) {
      keywords += " #ask";
      // A question carries a connect code, so answers can also come privately.
      var dev = window.kafumuDevice, pair = window.kafumuPair.create({ fetch: window.fetch.bind(window), store: dev.store, origin: location.origin });
      ready = pair.invite(false).then(function (inv) { text += "\n" + inv.url; });
    }
    ready.then(function () { return window.kafumuOLN.post(text, keywords, bits, function (tries, ms) {
      if (ms > 0) pref("kafumu.hashrate", String(Math.round(tries / ms * 1000)));
      $("oln-status").textContent = tr("oln_working", { n: Math.round(tries / 1000) + "k" });
    }); }).then(function (n) {
      if (n && n.id) ownNotes.push(n);
      composeMode = {};
      f.text.value = "";
      f.hidden = true;
      setStatus(tr("oln_sent", { s: Math.round((Date.now() - t0) / 1000) }));
      load(currentCell, true);
    }).catch(function (err) {
      $("oln-status").textContent = tr("oln_failed") + " " + err.message;
    }).then(function () { f.querySelector("button[type=submit]").disabled = false; });
  });

  // "Who's up for coffee?": a local message carrying your connect code, so
  // whoever taps Join swaps cards with you (only what your persona shares).
  $("coffee").onclick = function () { var c = $("coffee-card"); c.hidden = !c.hidden; };
  $("coffee-go").onclick = function () {
    postCoffee(this, $("coffee-status"), function (card) { return tr("coffee_text", { name: card.name || "" }); }, function () { return olnKeywords() + " #coffee"; });
  };
  // postCoffee: a local message with your connect code; then listen a while
  // for people who join.
  function postCoffee(btn, statusEl, textFor, keywords) {
    var dev = window.kafumuDevice;
    if (!dev || !window.kafumuPair || !currentCell) return;
    btn.disabled = true;
    var pair = window.kafumuPair.create({ fetch: window.fetch.bind(window), store: dev.store, origin: location.origin });
    dev.personas.shareCard().then(function (card) {
      return pair.invite(false).then(function (inv) {
        var text = textFor(card).trim() + "\n" + inv.url;
        return window.kafumuOLN.post(text, keywords(), requiredBits, function (tries) {
          statusEl.textContent = tr("oln_working", { n: Math.round(tries / 1000) + "k" });
        });
      });
    }).then(function (n) {
      if (n && n.id) ownNotes.push(n);
      statusEl.textContent = tr("coffee_sent");
      load(currentCell, true);
      // Keep listening for people who join, like the Connect page does.
      var pairer = window.kafumuPair.create({ fetch: window.fetch.bind(window), store: dev.store, origin: location.origin });
      var until = Date.now() + 30 * 60 * 1000;
      (function listen() {
        dev.personas.shareCard().then(function (card) { return pairer.checkInvite(card); }).then(function (added) {
          if (added.length) statusEl.textContent = tr("coffee_joined", { names: added.map(function (c) { return c.card.name; }).join(", ") });
        }).catch(function () {}).then(function () { if (Date.now() < until) setTimeout(listen, 10000); });
      })();
    }).catch(function (err) { statusEl.textContent = tr("oln_failed") + " " + err.message; btn.disabled = false; });
  }

  // ---- "I'm confused": a short tour of the page, one thing at a time ----
  var TOUR = [[".cell-tag", "tour_area"], ["#coffee", "tour_coffee"], ["#say", "tour_say"], ["#ask", "tour_ask"],
    ["#filter-chips", "tour_chips"], ["#feed-kinds", "tour_feed"], ["#change-area", "tour_change"], [".switcher.bottom", "tour_tabs"]];
  var tourAt = -1;
  function tourShow(i) {
    var old = document.querySelector(".tour-focus");
    if (old) old.classList.remove("tour-focus");
    var steps = TOUR.filter(function (s) { var e = document.querySelector(s[0]); return e && !e.hidden && e.offsetParent !== null; });
    if (i < 0 || i >= steps.length) { $("tour").hidden = true; tourAt = -1; pref("kafumu.tourSeen", "1"); return; }
    tourAt = i;
    var el = document.querySelector(steps[i][0]);
    el.classList.add("tour-focus");
    el.scrollIntoView({ block: "center", behavior: "smooth" });
    $("tour-text").textContent = tr(steps[i][1]);
    $("tour-step").textContent = (i + 1) + " / " + steps.length;
    $("tour-back").disabled = i === 0;
    $("tour-next").hidden = i === steps.length - 1;
    $("tour").hidden = false;
  }
  $("tour-start").onclick = function () { tourShow(0); };
  $("tour-next").onclick = function () { tourShow(tourAt + 1); };
  $("tour-back").onclick = function () { tourShow(tourAt - 1); };
  $("tour-done").onclick = function () { tourShow(-1); };

  // ---- "Learn the local language" ----
  // The area's main language (by country); offered when you don't speak it.
  var COUNTRY_LANG = { PT: "por", BR: "por", AO: "por", MZ: "por", NL: "nld", SR: "nld", DE: "deu", AT: "deu", FR: "fra",
    ES: "spa", MX: "spa", AR: "spa", CO: "spa", CL: "spa", PE: "spa", UY: "spa", IT: "ita", PL: "pol", RU: "rus", UA: "ukr",
    TR: "tur", GR: "ell", SE: "swe", NO: "nor", DK: "dan", FI: "fin", IS: "isl", CZ: "ces", SK: "slk", HU: "hun", RO: "ron",
    BG: "bul", HR: "hrv", RS: "srp", SI: "slv", EE: "est", LV: "lav", LT: "lit", JP: "jpn", KR: "kor", CN: "cmn", TW: "cmn",
    VN: "vie", TH: "tha", ID: "ind", MY: "msa", IL: "heb", EG: "ara", MA: "ara", SA: "ara", AE: "ara", IR: "fas", IN: "hin",
    PK: "urd", BD: "ben", KE: "swa", TZ: "swa", GE: "kat", AM: "hye", AL: "sqi", CV: "por", CU: "spa", DO: "spa", VE: "spa" };
  var learnLang = "", learnNeeded = false;
  // ---- Filter chips: events, the local language, interests ----
  // Tapping one narrows Around to it (another tap clears it). Events and
  // interests filter strictly; a language boosts (it's broader).
  var editChips = false, lastBundle = null;
  function chipURL(kind, value, on) {
    var next = { lang: view.lang, tag: view.tag, strength: view.strength };
    if (kind === "tag") { next.tag = on ? "" : value; next.strength = on ? 2 : 3; }
    else { next.lang = on ? "" : value; next.strength = on ? 2 : 3; }
    var q = new URLSearchParams();
    q.set("cell", currentCell);
    if (next.lang) q.set("lang", next.lang);
    if (next.tag) q.set("tag", next.tag);
    if ((next.lang || next.tag) && next.strength !== 2) q.set("w", String(next.strength));
    return "/?" + q.toString();
  }
  function chipCfg() { var d = window.kafumuDevice; return d && d.chips ? d.chips.get() : { pinned: [], hidden: [] }; }
  function saveChips(c) { var d = window.kafumuDevice; if (d && d.chips) return d.chips.set(c); return Promise.resolve(); }
  function pin(key) { var c = chipCfg(); c.pinned = (c.pinned || []).filter(function (k) { return k !== key; }).concat([key]).slice(-20); c.hidden = (c.hidden || []).filter(function (k) { return k !== key; }); return saveChips(c); }
  function normTag(t) { return String(t || "").toLowerCase().replace(/^#/, "").replace(/[^\p{L}\p{N}_]/gu, ""); }
  function drawFilterChips(b) {
    if (b) lastBundle = b; else b = lastBundle || {};
    var box = $("filter-chips"), me = window.KAFUMU_ME || {}, cfg = chipCfg();
    box.textContent = "";
    var chips = [], seen = {};
    function short(code) { return String((me.names || {})[code] || code).split(" (")[0]; }
    function add(kind, value, label, pinned) {
      var k = kind + ":" + value;
      if (!value || seen[k] || (!pinned && (cfg.hidden || []).indexOf(k) >= 0)) return;
      seen[k] = true; chips.push({ kind: kind, value: value, label: label, key: k, pinned: !!pinned });
    }
    // Yours first (pinned), then suggestions: events, the local language,
    // what's popular here, your profile interests.
    (cfg.pinned || []).forEach(function (k) {
      var p = k.split(":"), kind = p[0], v = p.slice(1).join(":");
      add(kind, v, kind === "lang" ? "🗣 " + short(v) : "#" + v, true);
    });
    (b.events || []).forEach(function (e) { add("tag", e.tag, (e.live ? "🔴 " : "📅 ") + "#" + e.tag); });
    if (learnLang && (me.names || {})[learnLang]) add("lang", learnLang, "🗣 " + short(learnLang));
    if (view.lang) add("lang", view.lang, "🗣 " + short(view.lang));
    var counts = {};
    try { counts = JSON.parse(localStorage.getItem("kafumu.localTags") || "{}"); } catch (e) {}
    Object.keys(counts).sort(function (a, z) { return counts[z] - counts[a]; }).slice(0, 6).forEach(function (t) { add("tag", t, "#" + t); });
    (me.tags || []).slice(0, 6).forEach(function (t) { add("tag", normTag(t), "#" + normTag(t)); });
    if (view.tag) add("tag", view.tag, "#" + view.tag);
    chips.forEach(function (c) {
      var on = c.kind === "tag" ? view.tag === c.value : view.lang === c.value;
      var a = document.createElement("a");
      a.className = "chip" + (on ? " on" : "") + (c.pinned ? " pinned" : "");
      a.setAttribute("aria-pressed", on);
      a.textContent = (editChips ? "× " : on ? "✓ " : "") + c.label;
      a.href = chipURL(c.kind, c.value, on);
      if (editChips) a.onclick = function (e) {
        // Edit: a pinned chip is unpinned; a suggestion is hidden from now on.
        e.preventDefault();
        var cf = chipCfg();
        if (c.pinned) cf.pinned = (cf.pinned || []).filter(function (k) { return k !== c.key; });
        else cf.hidden = (cf.hidden || []).concat([c.key]);
        saveChips(cf).then(function () { drawFilterChips(); });
      };
      box.appendChild(a);
    });
    // 🗣 +: any language. # +: any subject (pinned, then filtered).
    var langs = document.createElement("select");
    langs.className = "chip chip-select";
    langs.setAttribute("aria-label", tr("chip_lang"));
    var first = document.createElement("option"); first.value = ""; first.textContent = "🗣 +"; langs.appendChild(first);
    Object.keys(me.names || {}).sort(function (x, y) { return short(x).localeCompare(short(y)); }).forEach(function (code) {
      var o = document.createElement("option"); o.value = code; o.textContent = short(code); langs.appendChild(o);
    });
    langs.onchange = function () { var v = langs.value; if (!v) return; pin("lang:" + v).then(function () { location.href = chipURL("lang", v, false); }); };
    // Small until tapped: "🗣 +" and "# +" open their picker in place.
    function opener(label, title, open) {
      var b = document.createElement("button");
      b.type = "button"; b.className = "chip"; b.textContent = label; b.title = title;
      b.onclick = function () { b.replaceWith(open()); };
      return b;
    }
    box.appendChild(opener("🗣 +", tr("chip_lang"), function () {
      langs.hidden = false;
      setTimeout(function () { langs.focus(); try { langs.showPicker(); } catch (e) {} }, 0);
      return langs;
    }));
    var subj = document.createElement("form");
    subj.className = "chip-subject";
    var inp = document.createElement("input");
    inp.placeholder = "# +"; inp.setAttribute("aria-label", tr("chip_subject")); inp.setAttribute("list", "chip-suggest"); inp.size = 6;
    var dl = document.createElement("datalist"); dl.id = "chip-suggest";
    TAG_GROUPS.forEach(function (g) { var o = document.createElement("option"); o.value = "#" + g[0]; dl.appendChild(o); });
    subj.appendChild(inp); subj.appendChild(dl);
    subj.onsubmit = function (e) { e.preventDefault(); var t = normTag(inp.value); if (!t) return; pin("tag:" + t).then(function () { location.href = chipURL("tag", t, false); }); };
    box.appendChild(opener("# +", tr("chip_subject"), function () { setTimeout(function () { inp.focus(); }, 0); return subj; }));
    var edit = document.createElement("button");
    edit.type = "button"; edit.className = "chip" + (editChips ? " on" : "");
    edit.textContent = editChips ? "✓" : "✎"; edit.title = tr("chip_edit");
    edit.onclick = function () { editChips = !editChips; drawFilterChips(); };
    box.appendChild(edit);
    box.hidden = false;
    // Looking at a language you don't speak: the learn card comes along.
    if (view.lang && view.lang === learnLang && learnNeeded) $("learn-card").hidden = false;
  }
  function offerLearn(country) {
    var me = window.KAFUMU_ME || {}, code = COUNTRY_LANG[country] || "", b = $("learn");
    var speaks = (me.langs || []).some(function (l) { var p = l.split("/"); return p[0] === code && p[1] !== "learning"; }) ||
      (me.from1 || {})[document.documentElement.lang] === code;
    learnLang = code;
    learnNeeded = !!code && !speaks && !!(me.names || {})[code];
    b.hidden = true; // offered as the language chip in the filter row instead
    if (!learnNeeded) return;
    var name = me.names[code];
    b.textContent = "🗣 " + tr("learn_btn", { lang: name });
    $("learn-title").textContent = "🗣 " + tr("learn_btn", { lang: name });
    $("learn-explain").textContent = tr("learn_explain", { lang: name });
    $("learn-view").href = "/?cell=" + currentCell + "&lang=" + code + "&w=2";
  }
  $("learn").onclick = function () { var c = $("learn-card"); c.hidden = !c.hidden; };
  $("learn-go").onclick = function () {
    var name = ((window.KAFUMU_ME || {}).names || {})[learnLang] || learnLang;
    postCoffee(this, $("learn-status"), function (card) { return tr("learn_text", { lang: name, name: card.name || "" }); },
      function () { return ["#geo" + currentCell, "#lang" + learnLang, "#learn", "#coffee"].concat(liveEvents.map(function (t) { return "#" + t; })).join(" "); });
  };

  function hiddenNotes() { try { return JSON.parse(pref("kafumu.hiddenNotes") || "[]"); } catch (e) { return []; } }
  var ownNotes = []; // shown at once, even if another instance's cache lags
  // Notes, questions and answers. A question is a note tagged #ask (with a
  // connect code for private answers); a public answer is tagged #re<id>
  // and shown under it. Questions matching your own tags come first.
  function noteItem(n, opts) {
    var li = document.createElement("li");
    var meta = document.createElement("div");
    meta.className = "meta";
    var left = Math.max(0, (new Date(n.expires) - Date.now()) / 36e5);
    meta.textContent = (opts.forYou ? "★ " + tr("ask_for_you") + " · " : "") + ago(n.at) + " · ⚡" + n.bits + " · " +
      tr("oln_left", { h: left < 1 ? "<1" : Math.round(left) }) +
      (n.tags || []).filter(function (t) { return !/^(geo|re[0-9a-f]{10}$|ask$)/.test(t); }).map(function (t) { return " #" + t; }).join("");
    var text = document.createElement("p");
    text.className = "text";
    var m = n.text.match(/https?:\/\/[^\s]+\/c#v1\.[A-Za-z0-9_-]+/);
    text.textContent = (opts.question ? "❓ " : "") + (m ? n.text.replace(m[0], "").trim() : n.text);
    li.appendChild(meta); li.appendChild(text);
    var row = document.createElement("div");
    row.className = "actions";
    if (m && m[0].indexOf(location.origin + "/c#") === 0) {
      var join = document.createElement("a");
      join.href = m[0]; join.setAttribute("role", "button"); join.className = "pill-sm suggested";
      join.textContent = opts.question ? "🔒 " + tr("ask_private") : "☕ " + tr("coffee_join");
      row.appendChild(join);
    }
    if (opts.question) {
      var ans = document.createElement("button");
      ans.type = "button"; ans.className = "pill-sm"; ans.textContent = "💬 " + tr("ask_public");
      ans.onclick = function () { openComposer({ re: n.id, about: n.text }); };
      row.appendChild(ans);
    }
    function hideIt() { var h = hiddenNotes(); h.push(n.id); pref("kafumu.hiddenNotes", JSON.stringify(h.slice(-500))); li.remove(); }
    var hide = document.createElement("button");
    hide.type = "button"; hide.className = "pill-sm"; hide.textContent = tr("oln_hide");
    hide.onclick = hideIt;
    if (!opts.question && !opts.reply) li.dataset.reid = reactRow("note", n.id, n.text, row);
    else if (opts.question) li.dataset.reid = reID("note", n.id);
    row.appendChild(hide);
    row.appendChild(reportButton("note", n.id, n.text, li, hideIt));
    li.appendChild(row);
    swipeAway(li, hideIt);
    return li;
  }

  // ---- Reactions: react to any card with a local message ----
  // A reaction is an OLN message tagged #re<10 hex>: a local message's own id,
  // or for anything else (Bluesky post, meetup, person) a hash of what it is.
  // Emoji-only reactions show as counts; text ones as a thread under the card.
  var olnReplies = {};
  function reID(kind, id) {
    if (kind === "note") return String(id).slice(0, 10);
    var h = window.kafumuSHA1.sha1(new TextEncoder().encode(kind + ":" + id));
    return h.slice(0, 2).map(function (w) { return (w >>> 0).toString(16).padStart(8, "0"); }).join("").slice(0, 10);
  }
  var QUICK = ["👍", "❤️", "😂", "☕"];
  function isEmojiOnly(t) { return /^\s*(\p{Extended_Pictographic}\uFE0F?\s*){1,3}$/u.test(t || ""); }
  function reactRow(kind, id, about, row) {
    var rid = reID(kind, id);
    QUICK.forEach(function (e) {
      var b = document.createElement("button");
      b.type = "button"; b.className = "pill-sm react"; b.textContent = e; b.title = tr("react");
      b.onclick = function (ev) {
        ev.preventDefault(); ev.stopPropagation();
        if (!currentCell || !window.kafumuOLN) return;
        b.disabled = true;
        window.kafumuOLN.post(e, "#geo" + currentCell + " #re" + rid, requiredBits, function () {})
          .then(function (n) { if (n && n.id) ownNotes.push(n); b.textContent = e + " ✓"; load(currentCell, true); }, function () { b.disabled = false; });
      };
      row.appendChild(b);
    });
    var t = document.createElement("button");
    t.type = "button"; t.className = "pill-sm"; t.textContent = "💬 " + tr("react");
    t.onclick = function (ev) { ev.preventDefault(); ev.stopPropagation(); openComposer({ re: rid, about: about }); };
    row.appendChild(t);
    return rid;
  }
  // attachReplies puts a card's reactions under it: emoji counts, then texts.
  function attachReplies(li) {
    var rid = li.dataset.reid;
    if (!rid) return;
    Array.prototype.forEach.call(li.querySelectorAll(":scope > .reactions, :scope > .replies"), function (x) { x.remove(); });
    var rs = olnReplies[rid] || [];
    if (!rs.length) return;
    var counts = {}, texts = [];
    rs.forEach(function (r) { if (isEmojiOnly(r.text)) { var k = r.text.trim(); counts[k] = (counts[k] || 0) + 1; } else texts.push(r); });
    if (Object.keys(counts).length) {
      var c = document.createElement("div");
      c.className = "reactions";
      c.textContent = Object.keys(counts).map(function (k) { return k + " " + counts[k]; }).join("  ");
      li.appendChild(c);
    }
    if (texts.length) {
      var ul = document.createElement("ul");
      ul.className = "replies";
      texts.slice().reverse().forEach(function (r) { ul.appendChild(noteItem(r, { reply: true })); });
      li.appendChild(ul);
    }
  }

  // reportButton: ⚑ → reason chips → a stamped report; the card then goes
  // away on this device. Moderators see it in /admin.
  var REASONS = ["spam", "scam", "harassment", "wrong-place", "other"];
  function reportButton(kind, item, snippet, li, onHidden) {
    var b = document.createElement("button");
    b.type = "button"; b.className = "pill-sm report"; b.textContent = "⚑"; b.title = tr("report");
    b.setAttribute("aria-label", tr("report"));
    b.onclick = function (e) {
      e.preventDefault(); e.stopPropagation();
      var row = document.createElement("div");
      row.className = "chips report-reasons";
      row.appendChild(Object.assign(document.createElement("span"), { className: "dim small", textContent: tr("report_why") }));
      REASONS.forEach(function (r) {
        var c = document.createElement("button");
        c.type = "button"; c.className = "chip"; c.textContent = tr("reason_" + r.replace("-", "_"));
        c.onclick = function (e) {
          e.preventDefault(); e.stopPropagation();
          row.textContent = tr("report_sending");
          var pair = window.kafumuPair && window.kafumuPair.create({ fetch: window.fetch.bind(window), store: window.kafumuDevice.store, origin: location.origin });
          pair.report({ kind: kind, item: item, reason: r, snippet: String(snippet || "").slice(0, 200), cell: currentCell })
            .then(function () { row.textContent = "✓ " + tr("reported"); setTimeout(function () { if (onHidden) onHidden(); else li.remove(); }, 1200); },
              function () { row.textContent = tr("oln_failed"); });
        };
        row.appendChild(c);
      });
      b.replaceWith(row);
    };
    return b;
  }

  // swipeAway: drag an item sideways past a third of its width to hide it.
  function swipeAway(el, done) {
    var x0 = null, y0 = 0, dx = 0;
    el.addEventListener("touchstart", function (e) { x0 = e.touches[0].clientX; y0 = e.touches[0].clientY; dx = 0; el.style.transition = "none"; }, { passive: true });
    el.addEventListener("touchmove", function (e) {
      if (x0 === null) return;
      dx = e.touches[0].clientX - x0;
      if (Math.abs(e.touches[0].clientY - y0) > Math.abs(dx)) { x0 = null; el.style.transform = ""; return; } // scrolling
      el.style.transform = "translateX(" + dx + "px)";
      el.style.opacity = String(Math.max(0.2, 1 - Math.abs(dx) / el.offsetWidth));
    }, { passive: true });
    el.addEventListener("touchend", function () {
      if (x0 === null) return;
      x0 = null;
      el.style.transition = "transform .2s, opacity .2s";
      if (Math.abs(dx) > el.offsetWidth / 3) {
        el.style.transform = "translateX(" + (dx > 0 ? "" : "-") + "110%)"; el.style.opacity = "0";
        setTimeout(done, 200);
      } else { el.style.transform = ""; el.style.opacity = ""; }
    });
  }

  function showNotes(notes) {
    var have = {};
    notes.forEach(function (n) { have[n.id] = true; });
    ownNotes = ownNotes.filter(function (n) { return new Date(n.expires) > Date.now() && n.cell && currentCell && rings(currentCell, 2).some(function (p) { return p[0] === n.cell; }); });
    notes = ownNotes.filter(function (n) { return !have[n.id]; }).concat(notes);
    var hidden = hiddenNotes(), list = sinks.here, mySeq = loadSeq;
    notes = notes.filter(function (n) { return hidden.indexOf(n.id) < 0; });
    if (!notes.length) list.textContent = "";
    loadMyTags().then(function (mine) {
      if (mySeq !== loadSeq) return; // a newer load will draw the list
      list.textContent = ""; // cleared only now, right before drawing
      var me = window.KAFUMU_ME || {};
      (me.tags || []).forEach(function (t) { mine[t.toLowerCase().replace(/\s+/g, "")] = true; });
      (me.langs || []).forEach(function (l) { mine["lang" + l.split("/")[0]] = true; });
      var replies = {};
      notes.forEach(function (n) {
        (n.tags || []).forEach(function (t) { var r = /^re([0-9a-f]{10})$/.exec(t); if (r) (replies[r[1]] = replies[r[1]] || []).push(n); });
      });
      olnReplies = replies;
      var isReply = function (n) { return (n.tags || []).some(function (t) { return /^re[0-9a-f]{10}$/.test(t); }); };
      var isAsk = function (n) { return (n.tags || []).indexOf("ask") >= 0; };
      var forYou = function (n) { return isAsk(n) && (n.tags || []).some(function (t) { return t !== "ask" && mine[t]; }); };
      var top = notes.filter(function (n) { return !isReply(n); });
      top.sort(function (a, z) { return forYou(z) - forYou(a); }); // stable: keeps the ranking otherwise
      top.forEach(function (n) {
        var li = noteItem(n, { question: isAsk(n), forYou: forYou(n) });
        list.appendChild(li);
      });
    });
  }

  function show(c, how, gps) {
    var tag = "#geo" + c;
    $("cell-tag").textContent = tag;
    $("place-name").textContent = "";
    $("place-also").hidden = true;
    document.querySelector(".cell-tag").classList.remove("named");
    document.querySelector(".cell-tag").hidden = false;
    $("cell-actions").hidden = false;
    $("compose").href = "https://bsky.app/intent/compose?text=" + encodeURIComponent("\n\n" + tag);
    $("be-findable").href = "/findable?cell=" + c;
    // Connected to ATproto: Bluesky posts from here, into your own account.
    var comp = $("composer");
    if (comp) {
      comp.cell.value = c;
      $("compose").onclick = function (e) { e.preventDefault(); comp.hidden = !comp.hidden; if (!comp.hidden) comp.text.focus(); };
    }
    currentCell = c;
    $("share").onclick = function (e) {
      e.preventDefault();
      var url = location.origin + "/?cell=" + c;
      if (navigator.share) navigator.share({ title: tag, url: url }).catch(function () {});
      else if (navigator.clipboard) navigator.clipboard.writeText(url).then(function () { setStatus(tr("link_copied")); });
    };
    setStatus(how);
    try { localStorage.setItem("kafumu.lastCell", c); } catch (e) {}
    currentCell = c;
    drawViews();
    var home = homeCell(c, gps);
    travel = { place: "", friends: 0, meetups: 0, away: !!home && home !== c && kmBetween(home, c) > 50 };
    drawTravel();
    load(c);
    friendsAround(c, !!gps);
  }

  // fresh skips the browser's 30-second cache (after your own post). Only
  // the newest load renders: an older one finishing late is dropped, or two
  // quick loads (last area, then your location) would show things twice.
  var loadSeq = 0;
  function load(c, fresh) {
    var seq = ++loadSeq;
    var near = rings(c, 2);
    var ringOf = {};
    near.forEach(function (p) { ringOf["geo" + p[0]] = p[1]; });
    ["here", "meetups", "people", "posts"].forEach(function (k) { feed[k] = []; });
    scheduleDraw();
    note(tr("looking"));
    fetch("/bundle?cells=" + near.map(function (p) { return p[0]; }).join(","), fresh ? { cache: "reload" } : {})
      .then(function (r) { if (!r.ok) throw new Error(r.status); return r.json(); })
      .then(function (b) {
        if (seq !== loadSeq) return;
        countLocalTags(b);
        b = filterBundle(b);
        var places = {};
        (b.places || []).forEach(function (pt) { places[pt.tag] = pt; });
        // Event tags (#websummit) count as local as a #geo tag while they run.
        (b.events || []).forEach(function (e) { ringOf[e.tag] = e.live ? 0 : 1; });
        showEvents(b.events || [], c);
        requiredBits = b.requiredBits || 12;
        liveEvents = (b.events || []).filter(function (e) { return e.live; }).map(function (e) { return e.tag; });
        liveEventNames = (b.events || []).filter(function (e) { return e.live; }).map(function (e) { return e.name; });
        showNotes(b.notes || []);
        showMeetups(b.meetups || [], b.events || []);
        travel.meetups = (b.meetups || []).length;
        travel.place = ((b.places || [])[0] || {}).place || "";
        // A human heading: "Barreiro" or "Areeiro, Lisbon", the cell tag below.
        if (b.near && b.near.country) { try { localStorage.setItem("kafumu.country", b.near.country); } catch (e) {} }
        offerLearn(b.near ? b.near.country : "");
        drawFilterChips(b);
        if (b.near && b.near.name) {
          var placeText = b.near.name + (b.near.city ? ", " + b.near.city : "");
          // With a filter on, the heading says what you're looking at here.
          var langName = view.lang ? (((window.KAFUMU_ME || {}).names || {})[view.lang] || view.lang) : "";
          $("place-name").textContent = view.tag ? "#" + view.tag + " · " + placeText : langName ? "🗣 " + langName + " · " + placeText : placeText;
          document.querySelector(".cell-tag").classList.add("named");
          // The cloud: neighbourhoods and villages around, small.
          var also = $("place-also");
          also.textContent = (b.near.also || []).join(" · ");
          also.hidden = !(b.near.also || []).length;
          travel.place = travel.place || b.near.name;
        }
        drawTravel();
        showPeople(b.people || []);
        render(b.posts || [], ringOf, places, c);
        elsewhere(b, c);
        asksForMe(b, c);
        var named = (b.places || []).filter(function (pt) { return pt.weight >= 0.5; })
          .slice(0, 3).map(function (pt) { return "#" + pt.tag; });
        $("list-note").textContent = named.length
          ? tr("also_tags", { tags: named.join(", ") })
          : "";
      })
      .catch(function () { feed.posts = []; scheduleDraw(); note(tr("load_failed")); });
  }

  // myTags: the tags on your personas, used only here on the device to
  // rank meetups — the server never learns them.
  var myTags = null;
  function loadMyTags() {
    if (myTags || !window.kafumuDevice) return Promise.resolve(myTags || {});
    return window.kafumuDevice.personas.list().then(function (ps) {
      myTags = {};
      ps.forEach(function (p) { ((p.card && p.card.tags) || []).forEach(function (t) { myTags[t.toLowerCase().replace(/\s+/g, "")] = true; }); });
      return myTags;
    }).catch(function () { return (myTags = {}); });
  }

  function showMeetups(ms, events) {
    var sec = noSection, list = sinks.meetups;
    sec.hidden = !ms.length;
    if (!ms.length) return;
    loadMyTags().then(function (mine) {
      var live = {};
      events.forEach(function (e) { live[e.tag] = true; });
      function rank(m) {
        var hits = (m.tags || []).filter(function (t) { return mine[t.replace(/\s+/g, "")] || live[t]; }).length;
        var hours = (new Date(m.start) - Date.now()) / 36e5;
        return hits * 24 - Math.max(hours, 0);
      }
      ms.sort(function (a, b) { return rank(b) - rank(a); });
      list.textContent = "";
      ms.slice(0, 12).forEach(function (m) {
        var li = document.createElement("li"), a = document.createElement("a");
        var hoursTo = (new Date(m.start) - Date.now()) / 36e5, live = hoursTo <= 0 && new Date(m.end) > Date.now();
        li.dataset.score = live ? 12 : 8 + rank(m) / 24; // rank: 24 per tag match, minus hours to go
        a.href = "/meetups/" + m.id;
        a.className = "meetup-row";
        var s = new Date(m.start), e = new Date(m.end), now = Date.now();
        var when = document.createElement("div");
        when.className = "when";
        when.textContent = (s <= now && e > now ? tr("now") + " · " : s.toLocaleDateString(window.KAFUMU_LOCALE, { weekday: "short", day: "numeric", month: "short" }) + " · ") +
          s.toLocaleTimeString(window.KAFUMU_LOCALE, { hour: "2-digit", minute: "2-digit" });
        var title = document.createElement("strong");
        title.textContent = m.title;
        var meta = document.createElement("div");
        meta.className = "meta";
        meta.textContent = [m.venue, m.via ? tr("via", { site: m.via }) : tr("going_n", { n: m.going })].concat((m.tags || []).map(function (t) { return "#" + t; })).filter(Boolean).join(" · ");
        a.appendChild(when); a.appendChild(title); a.appendChild(meta);
        li.appendChild(a);
        var mrow = document.createElement("div");
        mrow.className = "actions";
        li.dataset.reid = reactRow("meetup", m.id, m.title, mrow);
        mrow.appendChild(reportButton("meetup", m.id, m.title, li));
        li.appendChild(mrow);
        list.appendChild(li);
      });
    });
  }

  // People ranking, on the device (amikumu's insight): someone who speaks
  // what you learn and learns what you speak first; then a shared language,
  // weighted by how rare it is here; then shared interests.
  function showPeople(people) {
    var me = window.KAFUMU_ME || { langs: [], tags: [], names: {} };
    var sec = noSection, list = sinks.people;
    sec.hidden = !people.length;
    if (!people.length) return;
    function split(ls) {
      var speak = {}, learn = {};
      (ls || []).forEach(function (l) { var p = l.split("/"); if (p[1] === "learning") learn[p[0]] = true; else speak[p[0]] = true; });
      return { speak: speak, learn: learn };
    }
    var mine = split(me.langs), myTags = {};
    (me.tags || []).forEach(function (t) { myTags[t] = true; });
    var count = {};
    people.forEach(function (p) { (p.langs || []).forEach(function (l) { var c = l.split("/")[0]; count[c] = (count[c] || 0) + 1; }); });
    var name = function (c) { return (me.names || {})[c] || c; };
    people.forEach(function (p) {
      var th = split(p.langs), score = 0, why = [];
      var teach = Object.keys(th.speak).filter(function (c) { return mine.learn[c]; });
      var learnFromMe = Object.keys(th.learn).filter(function (c) { return mine.speak[c]; });
      if (teach.length && learnFromMe.length) { score += 10; why.push(tr("exchange", { a: name(teach[0]), b: name(learnFromMe[0]) })); }
      else if (teach.length) { score += 5; why.push(tr("speaks_learning", { lang: name(teach[0]) })); }
      else if (learnFromMe.length) { score += 4; why.push(tr("learns_speak", { lang: name(learnFromMe[0]) })); }
      Object.keys(th.speak).forEach(function (c) {
        if (mine.speak[c]) { score += 4 / count[c]; if (count[c] <= 3) why.push(tr("rare_shared", { lang: name(c) })); }
      });
      (p.tags || []).forEach(function (t) { if (myTags[t]) { score += 1; why.push("#" + t); } });
      p._score = score; p._why = why;
    });
    people.sort(function (a, b) { return b._score - a._score; });
    list.textContent = "";
    people.slice(0, 20).forEach(function (p) {
      var li = document.createElement("li");
      var head = document.createElement("strong");
      head.textContent = "@" + p.name + (p.inbox ? " ✉️" : "");
      li.appendChild(head);
      if (p._why.length) { var w = document.createElement("div"); w.className = "why"; w.textContent = p._why.slice(0, 3).join(" · "); li.appendChild(w); }
      var meta = document.createElement("div");
      meta.className = "meta";
      meta.textContent = [p.bio, p.where ? "📍 " + p.where : "", (p.langs || []).map(function (l) {
        var x = l.split("/"); return name(x[0]) + (x[1] === "learning" ? " (" + tr("learning") + ")" : "");
      }).join(", ")].filter(Boolean).join(" · ");
      li.appendChild(meta);
      if (p.inbox && window.kafumuPair && window.kafumuOLN) li.appendChild(writeBox(p));
      var prow = document.createElement("div");
      prow.className = "actions";
      li.dataset.reid = reactRow("person", p.name, "@" + p.name + (p.bio ? ": " + p.bio : ""), prow);
      prow.appendChild(reportButton("person", p.name, "@" + p.name + (p.bio ? ": " + p.bio : ""), li));
      li.appendChild(prow);
      list.appendChild(li);
    });
  }

  // writeBox: write to someone's public inbox, paying their price in work.
  function writeBox(p) {
    var wrap = document.createElement("div"), btn = document.createElement("button");
    btn.type = "button"; btn.className = "pill-sm";
    var s = Math.pow(2, p.inbox.bits) / hashrate();
    btn.textContent = "✉️ " + tr("inbox_write", { cost: s < 90 ? Math.max(1, Math.round(s)) + " s" : Math.round(s / 60) + " min" });
    wrap.appendChild(btn);
    btn.onclick = function () {
      btn.hidden = true;
      var f = document.createElement("form");
      f.innerHTML = '<textarea rows="2" maxlength="1000"></textarea><label class="small"><input type="checkbox" checked> ' +
        tr("inbox_share_card") + '</label><div class="actions"><button type="submit" class="pill-sm suggested"></button></div><p class="dim small"></p>';
      f.querySelector("button").textContent = tr("oln_send") || "Send";
      wrap.appendChild(f);
      f.querySelector("textarea").focus();
      f.onsubmit = function (e) {
        e.preventDefault();
        var text = f.querySelector("textarea").value.trim(), st = f.querySelector("p");
        if (!text) return;
        f.querySelector("button").disabled = true;
        var dev = window.kafumuDevice, pair = window.kafumuPair.create({ fetch: window.fetch.bind(window), store: dev.store, origin: location.origin });
        var t0 = Date.now();
        (f.querySelector("input").checked ? dev.personas.shareCard() : Promise.resolve(null)).then(function (card) {
          st.textContent = tr("oln_working", { n: "…" });
          return pair.writeTo(p.inbox, text, card, function (tail, bits) {
            return window.kafumuOLN.mineTail(tail, bits, function (tries, ms) {
              if (ms > 0) pref("kafumu.hashrate", String(Math.round(tries / ms * 1000)));
              st.textContent = tr("oln_working", { n: Math.round(tries / 1000) + "k" });
            });
          });
        }).then(function () {
          st.textContent = tr("inbox_sent", { s: Math.round((Date.now() - t0) / 1000) });
          f.querySelector("textarea").value = "";
        }).catch(function (err) { st.textContent = tr("oln_failed") + " " + err.message; f.querySelector("button").disabled = false; });
      };
    };
    return wrap;
  }

  function showEvents(events, c) {
    var box = $("events");
    box.textContent = "";
    box.hidden = !events.length;
    events.forEach(function (e) {
      var p = document.createElement("p");
      var strong = document.createElement("strong");
      strong.textContent = e.name;
      p.appendChild(strong);
      p.appendChild(document.createTextNode(e.live
        ? " " + tr("event_live", { name: "", tag: "#" + e.tag }).trim()
        : " " + tr("event_upcoming", { name: "", from: e.from, to: e.to, tag: "#" + e.tag }).trim()));
      var a = document.createElement("a");
      a.href = "https://bsky.app/intent/compose?text=" + encodeURIComponent("\n\n#geo" + c + " #" + e.tag);
      a.target = "_blank"; a.rel = "noopener";
      a.textContent = " " + tr("post_with", { tag: "#" + e.tag });
      p.appendChild(a);
      box.appendChild(p);
    });
  }

  // myLangs: the languages to favour, as 639-3 codes — from your profile, or
  // from the browser's languages when you have none. Never sent anywhere.
  var myLangs = (function () {
    var me = window.KAFUMU_ME || {}, out = {};
    (me.langs || []).forEach(function (l) { out[l.split("/")[0]] = true; });
    if (!Object.keys(out).length) (navigator.languages || []).forEach(function (l) {
      var c = (me.from1 || {})[l.slice(0, 2).toLowerCase()];
      if (c) out[c] = true;
    });
    return out;
  })();

  // langMatch: a language hashtag or the post's own language in your list.
  function langMatch(p) {
    var me = window.KAFUMU_ME || {}, tags = me.langTags || {}, from1 = me.from1 || {}, hit = "";
    (p.tags || []).forEach(function (t) {
      var lt = tags[t];
      if (lt && (lt.codes.length === 0 ? false : lt.codes.some(function (c) { return myLangs[c]; }))) hit = "#" + t;
    });
    var postLang = ((p.langs || [])[0] || "").slice(0, 2);
    return { tag: hit, lang: !!myLangs[from1[postLang]] };
  }

  // score ranks on the device: #geo posts before place-tag posts, nearer
  // rings and fresher posts first, your languages up, noisy posts last.
  function score(p, ringOf, places) {
    var ageH = (Date.now() - new Date(p.createdAt).getTime()) / 36e5;
    var lm = langMatch(p);
    var s = -ageH / 24 - (p.bot ? 5 : 0) + (lm.tag ? 2 : 0) + (lm.lang ? 0.5 : 0) + (p._match ? view.strength * 3 : 0);
    // Reliability: posts aimed at many cities at once, brand-new accounts and
    // moderation labels weigh less; real engagement a little more.
    if ((p.placeTags || 0) >= 5) s -= 8;
    if (p.since && Date.now() - new Date(p.since) < 30 * 864e5) s -= 1;
    if ((p.labels || []).length) s -= 3;
    s += Math.min(1, Math.log10(1 + (p.likes || 0) + 2 * (p.reposts || 0) + (p.replies || 0)) / 2);
    if (p.via in ringOf) return s - ringOf[p.via] * 0.5;
    var pt = places[p.via];
    // A place-tag post that also carries a #geo tag of this area is strong.
    var geoToo = (p.tags || []).some(function (t) { return t in ringOf; });
    return s - (geoToo ? 0.5 : 2) - (1 - (pt ? pt.weight : 0)) * 3;
  }

  function render(posts, ringOf, places, c) {
    var list = sinks.posts;
    list.textContent = "";
    if (!posts.length) {
      var li = document.createElement("li");
      li.className = "muted";
      li.textContent = tr("empty", { tag: "#geo" + c });
      list.appendChild(li);
      return;
    }
    posts.sort(function (a, b) { return score(b, ringOf, places) - score(a, ringOf, places); });
    // At most two posts per author, so one busy account (news feeds, flight
    // trackers on #ams) can't fill the list.
    var perAuthor = {};
    posts = posts.filter(function (p) {
      perAuthor[p.handle] = (perAuthor[p.handle] || 0) + 1;
      return perAuthor[p.handle] <= 2;
    });
    var hidden = hiddenNotes();
    posts.filter(function (p) { return hidden.indexOf(p.uri) < 0; }).slice(0, 50).forEach(function (p) {
      var li = postItem(p, ringOf, places);
      li.dataset.score = 3 + score(p, ringOf, places);
      list.appendChild(li);
    });
  }

  // postItem draws a Bluesky post as a card like a local message: who
  // (with what helps judge them: own-domain handle, account age), when,
  // engagement, where it was found, labels; then the actions. No avatar
  // images: loading them would tell Bluesky's CDN who looks at what.
  var ADULT = ["porn", "sexual", "nudity", "graphic-media", "gore"];
  function postItem(p, ringOf, places) {
    var li = document.createElement("li");
    li.className = "post" + (p.bot ? " bot" : "");
    function span(cls, text) { var e = document.createElement("span"); if (cls) e.className = cls; e.textContent = text; return e; }
    var head = document.createElement("div");
    head.className = "post-head";
    var initial = span("avatar", ((p.name || p.handle || "?").trim()[0] || "?").toUpperCase());
    head.appendChild(initial);
    var who = document.createElement("div");
    who.className = "who";
    who.appendChild(span("name", p.name || p.handle));
    var own = !/\.bsky\.social$/.test(p.handle || "");
    var handle = span("dim small", "@" + p.handle + (own ? " ✓" : ""));
    if (own) handle.title = tr("own_domain");
    who.appendChild(handle);
    head.appendChild(who);
    li.appendChild(head);

    var meta = document.createElement("div");
    meta.className = "meta";
    var bits = [ago(p.createdAt)];
    if (p.likes) bits.push("♥ " + p.likes);
    if (p.reposts) bits.push("🔁 " + p.reposts);
    if (p.replies) bits.push("💬 " + p.replies);
    if (p.since) {
      var days = (Date.now() - new Date(p.since)) / 864e5;
      bits.push(days < 30 ? "🆕 " + tr("new_account") : tr("on_bsky_since", { when: new Date(p.since).toLocaleDateString(window.KAFUMU_LOCALE, { month: "short", year: "numeric" }) }));
    }
    meta.appendChild(document.createTextNode(bits.join(" · ") + " "));
    var via = span("badge", "");
    if (p.via in ringOf) via.textContent = "#" + p.via + (ringOf[p.via] ? "" : " · " + tr("here"));
    else via.textContent = tr("from", { tag: "#" + p.via }) + (places[p.via] && places[p.via].ambiguous ? " " + tr("maybe_elsewhere") : "");
    meta.appendChild(via);
    if (p.bot) meta.appendChild(span("badge", tr("bot")));
    if ((p.placeTags || 0) >= 5) meta.appendChild(span("badge warn", tr("tags_n_cities", { n: p.placeTags })));
    var lm = langMatch(p);
    if (lm.tag) meta.appendChild(span("badge lang", lm.tag));
    (p.labels || []).forEach(function (l) { meta.appendChild(span("badge warn", "⚠ " + l)); });
    li.appendChild(meta);

    var text = document.createElement("p");
    text.className = "text";
    text.textContent = p.text;
    var row = document.createElement("div");
    row.className = "actions";
    if ((p.labels || []).some(function (l) { return ADULT.indexOf(l) >= 0; })) {
      text.hidden = true;
      var reveal = document.createElement("button");
      reveal.type = "button"; reveal.className = "pill-sm"; reveal.textContent = tr("show_anyway");
      reveal.onclick = function () { text.hidden = false; reveal.remove(); };
      row.appendChild(reveal);
    }
    li.appendChild(text);
    var open = document.createElement("a");
    open.href = p.url; open.target = "_blank"; open.rel = "noopener";
    open.setAttribute("role", "button"); open.className = "pill-sm";
    open.textContent = "💬 " + tr("reply_on_bsky") + " ↗";
    row.appendChild(open);
    function hideIt() { var h = hiddenNotes(); h.push(p.uri); pref("kafumu.hiddenNotes", JSON.stringify(h.slice(-500))); li.remove(); }
    var hide = document.createElement("button");
    hide.type = "button"; hide.className = "pill-sm"; hide.textContent = tr("oln_hide");
    hide.onclick = hideIt;
    row.appendChild(hide);
    li.dataset.reid = reactRow("post", p.uri, (p.handle ? "@" + p.handle + ": " : "") + p.text, row);
    row.appendChild(reportButton("post", p.uri, (p.handle ? "@" + p.handle + ": " : "") + p.text, li, hideIt));
    li.appendChild(row);
    swipeAway(li, hideIt);
    return li;
  }

  // elsewhere: a tag filter that finds little here also shows that tag's
  // posts from anywhere, at the bottom.
  function elsewhere(b, c) {
    feed.elsewhere = [];
    // The tag to look for elsewhere: the filter's tag, or the language's own
    // hashtag (#tokipona for toki pona).
    var etag = view.tag;
    if (!etag && view.lang) etag = Object.keys(langTagsFor(view.lang)).filter(function (t) { return t !== "lang" + view.lang; })[0] || "lang" + view.lang;
    var matched = ["posts", "notes", "meetups", "people"].reduce(function (n, k) { return n + (b[k] || []).filter(function (x) { return x._match !== false; }).length; }, 0);
    var fn = $("filter-note");
    fn.hidden = !(view.tag || view.lang);
    if (view.tag || view.lang) {
      var what = view.tag ? "#" + view.tag : "🗣 " + String(((window.KAFUMU_ME || {}).names || {})[view.lang] || view.lang).split(" (")[0];
      fn.textContent = tr(matched >= 5 ? "filter_here" : "filter_few", { what: what, n: matched, tag: "#" + etag });
    }
    if (!etag) return;
    var here = (b.posts || []).filter(function (p) { return p._match !== false; }).length;
    if (here >= 5) return;
    var seq = loadSeq, uris = {};
    (b.posts || []).forEach(function (p) { uris[p.uri] = true; });
    fetch("/tagposts?tag=" + encodeURIComponent(etag)).then(function (r) { return r.ok ? r.json() : []; }).then(function (ps) {
      if (seq !== loadSeq) return;
      var hidden = hiddenNotes();
      ps.filter(function (p) { return !uris[p.uri] && hidden.indexOf(p.uri) < 0; }).slice(0, 20).forEach(function (p, i) {
        var li = postItem(p, {}, {});
        li.dataset.score = -100 - i;
        sinks.elsewhere.appendChild(li);
        li.dataset.label = tr("elsewhere", { tag: "#" + etag });
      });
    }).catch(function () {});
  }

  // asksForMe: questions about your interests from further away (up to
  // ASK_KM), found by subject; nearest first, marked with how far.
  var ASK_KM = 200;
  function asksForMe(b, c) {
    var me = window.KAFUMU_ME || {}, tags = {};
    (me.tags || []).forEach(function (t) { var n = normTag(t); if (n) tags[n] = true; });
    (chipCfg().pinned || []).forEach(function (k) { if (k.indexOf("tag:") === 0) tags[k.slice(4)] = true; });
    var list = Object.keys(tags).slice(0, 8);
    if (!list.length) return;
    var seq = loadSeq, have = {};
    (b.notes || []).forEach(function (n) { have[n.id] = true; });
    fetch("/api/asks?tags=" + encodeURIComponent(list.join(","))).then(function (r) { return r.ok ? r.json() : []; }).then(function (ns) {
      if (seq !== loadSeq) return;
      var hidden = hiddenNotes();
      ns.map(function (n) { n._km = kmBetween(c, n.cell); return n; })
        .filter(function (n) { return !have[n.id] && hidden.indexOf(n.id) < 0 && n._km <= ASK_KM; })
        .sort(function (a, z) { return a._km - z._km; }).slice(0, 10)
        .forEach(function (n, i) {
          var li = noteItem(n, { question: true, forYou: true });
          var subj = (n.tags || []).filter(function (t) { return tags[t]; }).map(function (t) { return "#" + t; }).join(" ");
          var far = document.createElement("div");
          far.className = "why";
          far.textContent = "❓ " + tr("ask_far", { tags: subj, km: Math.max(1, Math.round(n._km)) });
          li.insertBefore(far, li.firstChild);
          li.dataset.score = 9.5 - i * 0.3;
          sinks.here.appendChild(li);
        });
    }).catch(function () {});
  }

  function ago(iso) {
    var s = (Date.now() - new Date(iso).getTime()) / 1000;
    if (s < 3600) return tr("min_ago", { n: Math.max(1, Math.round(s / 60)) });
    if (s < 86400) return tr("h_ago", { n: Math.round(s / 3600) });
    return tr("d_ago", { n: Math.round(s / 86400) });
  }

  // Location is only asked for after a tap. Once you've used it, later
  // visits locate silently (the browser remembers the permission).
  function pref(k, v) {
    try { if (v === undefined) return localStorage.getItem(k); localStorage.setItem(k, v); } catch (e) {}
    return null;
  }
  function locate(silent) {
    if (!navigator.geolocation) { if (!silent) setStatus(tr("no_location")); return; }
    if (!silent) setStatus(tr("locating"));
    navigator.geolocation.getCurrentPosition(function (pos) {
      pref("kafumu.autoLocate", "1");
      // Round immediately: only the 5 km cell is kept.
      var c = cell(pos.coords.latitude, pos.coords.longitude);
      $("picker").hidden = true;
      show(c, tr("your_cell"), true);
    }, function () {
      if (!silent) { setStatus(tr("unavailable")); openPicker(); }
    }, { enableHighAccuracy: false, timeout: 10000, maximumAge: 600000 });
  }

  function choose(c, how) {
    history.replaceState(null, "", viewURL(c));
    show(c, how || tr("chosen_cell"));
  }

  function drawMap(around, selected) {
    $("map-hint").hidden = false;
    window.kafumuArea.render($("area-map-box"), around, selected, function (c) {
      drawMap(around, c);
      choose(c);
    }, function (next) { drawMap(next, selected); });
  }

  function openPicker() {
    $("picker").hidden = false;
    var cur = $("cell-tag").textContent.replace("#geo", "");
    if (validCell(cur)) drawMap(cur, cur);
  }

  var searchTimer;
  $("place-q").addEventListener("input", function () {
    var q = this.value.trim();
    clearTimeout(searchTimer);
    if (q.length < 2) { $("place-results").textContent = ""; return; }
    searchTimer = setTimeout(function () {
      fetch("/places?q=" + encodeURIComponent(q)).then(function (r) { return r.json(); }).then(function (ms) {
        var ul = $("place-results");
        ul.textContent = "";
        ms.forEach(function (m) {
          var li = document.createElement("li"), b = document.createElement("button");
          b.type = "button";
          b.textContent = m.name + (m.country ? " · " + m.country : "") + (m.tag ? "  #" + m.tag : "");
          b.onclick = function () {
            ul.textContent = "";
            $("place-q").value = m.name;
            drawMap(m.cell, m.cell);
            choose(m.cell, tr("chosen_place", { place: m.name }));
          };
          li.appendChild(b); ul.appendChild(li);
        });
      }).catch(function () {});
    }, 250);
  });
  $("locate").onclick = function () { locate(false); };
  var strengthNames = [tr("w0"), tr("w1"), tr("w2"), tr("w3")];
  $("view-strength").value = String(view.strength);
  $("view-strength-label").textContent = strengthNames[view.strength];
  $("view-strength").oninput = function () { $("view-strength-label").textContent = strengthNames[this.value]; };
  $("view-lang").value = view.lang;
  $("view-tag").value = view.tag;
  $("view-apply").onclick = function () {
    view.lang = $("view-lang").value;
    view.tag = $("view-tag").value.trim().toLowerCase().replace(/^#/, "").replace(/\s+/g, "");
    view.strength = parseInt($("view-strength").value, 10);
    if (currentCell) choose(currentCell, $("status").textContent);
    drawViews();
  };
  $("view-save").onclick = function () {
    $("view-apply").onclick();
    var place = travel.place || ((window.kafumuGeo && currentCell) ? "#geo" + currentCell : "");
    var name = prompt(tr("view_name"), [place, view.lang ? ((window.KAFUMU_ME || {}).names || {})[view.lang] : "", view.tag ? "#" + view.tag : ""].filter(Boolean).join(" · "));
    if (!name) return;
    var vs = savedViews().filter(function (v) { return v.name !== name; });
    vs.push({ name: name.slice(0, 40), url: viewURL(currentCell) });
    pref("kafumu.views", JSON.stringify(vs.slice(-12)));
    drawViews();
  };
  $("change-area").onclick = function () { if ($("picker").hidden) openPicker(); else $("picker").hidden = true; };

  $("manual-form").addEventListener("submit", function (e) {
    e.preventDefault();
    var c = parsePlace(this.where.value);
    if (!c) { setStatus(tr("bad_place")); return; }
    drawMap(c, c);
    choose(c);
  });

  // Signals from people you've connected with. Only recent contacts are
  // checked on open (one mailbox read each), to keep this cheap.
  function checkSignals() {
    var dev = window.kafumuDevice;
    if (!dev || !window.kafumuPair) return;
    var pair = window.kafumuPair.create({ fetch: window.fetch.bind(window), store: dev.store, origin: location.origin });
    dev.store.contacts().then(function (cs) {
      var recent = cs.filter(function (c) { return c.card; }).sort(function (a, b) {
        var la = ((a.signals || [])[0] || {}).at || a.createdAt || "", lb = ((b.signals || [])[0] || {}).at || b.createdAt || "";
        return lb.localeCompare(la);
      }).slice(0, 10);
      return Promise.all(recent.map(function (c) { return pair.checkContact(c).catch(function () { return []; }); })).then(function () {
        var unread = recent.filter(function (c) { return (c.signals || []).some(function (x) { return x.unread; }); });
        var list = sinks.signals;
        list.textContent = "";
        unread.forEach(function (c) {
          var li = document.createElement("li"), a = document.createElement("a");
          a.href = "/contacts";
          a.className = "meetup-row";
          var who = document.createElement("strong");
          who.textContent = c.card.name || "?";
          var what = document.createElement("div");
          what.className = "meta";
          what.textContent = dev.signalText(c.signals[0], window.KAFUMU_T || {});
          a.appendChild(who); a.appendChild(what); li.appendChild(a); list.appendChild(li);
        });
      });
    }).catch(function () {});
  }
  checkSignals();
  // Hellos from people who used your code while the app was closed.
  (function takeInvites() {
    var dev = window.kafumuDevice;
    if (!dev || !window.kafumuPair) return;
    var pair = window.kafumuPair.create({ fetch: window.fetch.bind(window), store: dev.store, origin: location.origin });
    dev.personas.shareCard().then(function (card) { return Promise.all([pair.checkInvite(card), pair.checkInvite(card, "badge"), pair.checkInvite(card, "named")]); })
      .then(function (r) { if (r[0].length + r[1].length + r[2].length) checkSignals(); }).catch(function () {});
  })();

  var given = $("here").dataset.cell, last = pref("kafumu.lastCell");
  if (given && validCell(given)) show(given, tr("shared_cell"));
  else if (pref("kafumu.autoLocate") === "1") { if (last && validCell(last)) show(last, tr("your_area")); locate(true); }
  else if (last && validCell(last)) show(last, tr("your_area"));
  else if (validCell($("here").dataset.guess || "") && $("here").dataset.guessName) {
    // First visit, no location yet: "Are you in Ede?" from the server's
    // city guess. Yes shows it; nothing is remembered until you choose.
    var g = $("here").dataset.guess, box = $("guess");
    $("guess-q").textContent = tr("guess_q", { place: $("here").dataset.guessName });
    box.hidden = false;
    $("guess-yes").onclick = function () { box.hidden = true; show(g, tr("your_area")); };
    $("guess-locate").onclick = function () { box.hidden = true; pref("kafumu.autoLocate", "1"); locate(false); };
    $("guess-pick").onclick = function () { box.hidden = true; setStatus(tr("pick_first")); openPicker(); };
  }
  else { setStatus(tr("pick_first")); openPicker(); }

})();
