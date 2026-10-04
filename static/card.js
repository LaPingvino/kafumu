// "My card": one or more personas, each a card you can hand to people you
// meet. Stored only on this device.
(function () {
  "use strict";
  var dev = window.kafumuDevice, P = dev.personas, T = window.KAFUMU_T || {};
  var $ = function (id) { return document.getElementById(id); };
  var form = $("card-form"), status = $("card-status"), preview = $("card-preview");
  var all = [], cur = null;
  // Phone examples in the local format ("+31…" in Ede), never a fixed country.
  Array.prototype.forEach.call(document.querySelectorAll("[data-phone]"), function (i) {
    i.placeholder = P.phoneExample() + (i.dataset.phone ? " " + i.dataset.phone : "");
  });

  function el(tag, cls, text) { var e = document.createElement(tag); if (cls) e.className = cls; if (text != null) e.textContent = text; return e; }
  function title(p, i) { return p.label || (p.card && p.card.name) || (T.persona_n || "Persona {n}").replace("{n}", i + 1); }

  function drawSwitcher() {
    var bar = $("personas");
    bar.textContent = "";
    all.forEach(function (p, i) {
      var b = el("button", "chip" + (p === cur ? " on" : ""), title(p, i));
      b.type = "button";
      b.onclick = function () { save(true).then(function () { cur = p; load(); }); };
      bar.appendChild(b);
    });
    var add = el("button", "chip", "+ " + (T.new_persona || "New persona"));
    add.type = "button";
    add.onclick = function () {
      save(true).then(function () {
        var base = cur && cur.card ? { name: cur.card.name } : {};
        cur = { id: P.newID(), label: "", card: base };
        all.push(cur);
        return P.save(all);
      }).then(load);
    };
    bar.appendChild(add);
    $("delete-persona").hidden = all.length < 2;
  }

  // Local themes first: tags seen around you lately (counted on this device
  // in Around), then the general suggestions.
  function localTags() {
    var counts = {};
    try { counts = JSON.parse(localStorage.getItem("kafumu.localTags") || "{}"); } catch (e) {}
    return Object.keys(counts).filter(function (t) { return counts[t] >= 1; })
      .sort(function (a, z) { return counts[z] - counts[a]; }).slice(0, 10);
  }
  function drawTags() {
    var box = $("tag-chips"), tags = cur.card.tags || [], local = localTags();
    box.textContent = "";
    var seen = {};
    local.concat(P.SUGGESTED_TAGS).concat(tags).forEach(function (t) {
      if (seen[t.toLowerCase()]) return;
      seen[t.toLowerCase()] = true;
      var on = tags.some(function (x) { return x.toLowerCase() === t.toLowerCase(); });
      var b = el("button", "chip" + (on ? " on" : ""), (local.indexOf(t) >= 0 ? "📍 " : "") + t);
      b.type = "button";
      b.setAttribute("aria-pressed", on);
      if (local.indexOf(t) >= 0) b.title = T.local_theme || "Popular around here lately";
      b.onclick = function () {
        cur.card.tags = on ? tags.filter(function (x) { return x.toLowerCase() !== t.toLowerCase(); }) : tags.concat([t]);
        drawTags(); render();
      };
      box.appendChild(b);
    });
  }

  function read() {
    var card = { tags: cur.card.tags || [] };
    dev.FIELDS.forEach(function (f) { var v = (form.elements[f].value || "").trim(); if (v) card[f] = v; });
    var custom = [];
    Array.prototype.forEach.call($("custom-fields").children, function (row) {
      var l = row.querySelector(".cf-label").value.trim().slice(0, 30), v = row.querySelector(".cf-value").value.trim().slice(0, 200);
      if (v) custom.push({ label: l, value: v });
    });
    if (custom.length) card.custom = custom;
    return card;
  }
  // Your own fields: a label and a value (a link, an email or just text).
  function customRow(f) {
    var row = el("div", "inline-row custom-field");
    var l = el("input", "cf-label"); l.placeholder = T.cf_label || "Label"; l.maxLength = 30; l.value = (f && f.label) || "";
    var v = el("input", "cf-value"); v.placeholder = T.cf_value || "Value or link"; v.maxLength = 200; v.value = (f && f.value) || "";
    var x = el("button", "pill-sm", "×"); x.type = "button"; x.onclick = function () { row.remove(); render(); };
    l.oninput = v.oninput = render;
    row.appendChild(l); row.appendChild(v); row.appendChild(x);
    return row;
  }
  function drawCustom() {
    var box = $("custom-fields");
    box.textContent = "";
    (cur.card.custom || []).forEach(function (f) { box.appendChild(customRow(f)); });
  }
  $("add-field").onclick = function () { var r = customRow(null); $("custom-fields").appendChild(r); r.querySelector(".cf-label").focus(); };

  function render() {
    var card = read();
    preview.textContent = "";
    preview.appendChild(dev.renderContact({ id: "preview", card: card }, T, { preview: true }));
  }

  function load() {
    form.elements.label.value = cur.label || "";
    dev.FIELDS.forEach(function (f) { form.elements[f].value = (cur.card && cur.card[f]) || ""; });
    cur.card = cur.card || {};
    drawSwitcher(); drawTags(); drawCustom(); render(); drawLink();
  }

  function save(quiet) {
    if (!cur) return Promise.resolve();
    cur.label = form.elements.label.value.trim();
    cur.card = read();
    cur.card.updatedAt = new Date().toISOString();
    return P.save(all).then(function () { if (!quiet) status.textContent = T.card_saved || "Saved."; drawSwitcher(); });
  }

  // Sync: show it's on, offer "Sync now", and redraw when other devices'
  // changes arrive.
  function syncState(s) { if ($("sync-on")) $("sync-on").hidden = s !== "on"; }
  window.addEventListener("kafumu:sync", function (e) { syncState(e.detail); });
  window.addEventListener("kafumu:synced", function () {
    P.list().then(function (ps) { all = ps; cur = ps.filter(function (p) { return cur && p.id === cur.id; })[0] || ps[0]; load(); });
  });
  if (window.kafumuSync) syncState(window.kafumuSync);
  if ($("sync-now")) $("sync-now").onclick = function () {
    var b = this; b.disabled = true; b.textContent = "…";
    dev.sync().then(function (s) { b.disabled = false; b.textContent = s === "on" ? "✓" : (T.sync_now || "Sync now"); syncState(s); });
  };
  // ---- Your kafumu.com/@name link: one card answers it ----
  var hb = $("handle-on");
  function drawLink() {
    if (!hb) return;
    dev.store.get("handle").then(function (h) {
      var on = !!(h && !h.off), mine = on && cur && h.persona === cur.id;
      hb.textContent = !on ? (T.handle_on_card || "Turn on with this card") : mine ? "✓ " + (T.handle_this_card || "Live with this card") : (T.handle_use_card || "Use this card for my link");
      hb.disabled = mine;
      $("handle-off").hidden = !on;
      $("handle-status").textContent = on ? (T.handle_device || "") : "";
      if (on) watchViews();
    });
  }
  // 👀 like on Connect: someone opened your link (only when, never who).
  var viewsTimer = null;
  function watchViews() {
    clearTimeout(viewsTimer);
    fetch("/api/handle", { credentials: "same-origin" }).then(function (r) { return r.ok ? r.json() : null; }).then(function (v) {
      var el = $("handle-views");
      if (!v || !v.n) { el.hidden = true; }
      else {
        var mins = Math.max(0, Math.round((Date.now() / 1000 - v.last) / 60));
        el.textContent = "👀 " + (mins < 1 ? (T.handle_view_now || "Someone is opening your link right now") : (T.handle_view_ago || "Someone opened your link {m} min ago").replace("{m}", mins)) +
          " · " + (T.handle_views_today || "{n} today").replace("{n}", v.n);
        el.hidden = false;
      }
    }).catch(function () {}).then(function () { if (!document.hidden) viewsTimer = setTimeout(watchViews, 15000); });
  }
  if (hb) {
    var hu = $("handle-url");
    hu.value = location.origin + "/@" + hu.dataset.name;
    var pr = window.kafumuPair.create({ fetch: window.fetch.bind(window), store: dev.store, origin: location.origin });
    hb.onclick = function () {
      hb.disabled = true;
      save(true).then(function () { return pr.namedLink(); })
        .then(function (url) { return dev.store.get("handle").then(function (h) { h.persona = cur.id; h.at = Date.now(); return dev.store.set("handle", h); }).then(function () { dev.sync(); return url; }); })
        .then(function (url) {
          drawLink();
          if (navigator.share) navigator.share({ title: "Kafumu", url: url }).catch(function () {});
          else if (navigator.clipboard) navigator.clipboard.writeText(url).catch(function () {});
        }, function () { $("handle-status").textContent = T.network_retry || "Try again."; hb.disabled = false; });
    };
    $("handle-off").onclick = function () {
      fetch("/api/handle", { method: "DELETE", credentials: "same-origin" }).then(function (r) {
        if (!r.ok) throw new Error(r.status);
        return dev.store.set("handle", { off: true, at: Date.now() }).then(function () { dev.sync(); });
      }).then(drawLink, function () { $("handle-status").textContent = T.network_retry || "Try again."; });
    };
  }

  P.list().then(function (ps) { all = ps; cur = ps[0]; load(); })
    .catch(function () { status.textContent = T.no_storage || "Storage unavailable"; });

  form.addEventListener("input", function () { status.textContent = ""; render(); });
  form.addEventListener("submit", function (e) { e.preventDefault(); save(false); });
  $("custom-tag").addEventListener("keydown", function (e) {
    if (e.key !== "Enter" && e.key !== ",") return;
    e.preventDefault();
    var v = this.value.trim().slice(0, 40);
    if (!v) return;
    cur.card.tags = (cur.card.tags || []).concat([v]);
    this.value = "";
    drawTags(); render();
  });
  $("delete-persona").onclick = function () {
    if (all.length < 2 || !confirm(T.delete_persona_confirm || "Delete?")) return;
    all = all.filter(function (p) { return p !== cur; });
    cur = all[0];
    P.save(all).then(load);
  };
})();
