// "My card": one or more personas, each a card you can hand to people you
// meet. Stored only on this device.
(function () {
  "use strict";
  var dev = window.kafumuDevice, P = dev.personas, T = window.KAFUMU_T || {};
  var $ = function (id) { return document.getElementById(id); };
  var form = $("card-form"), status = $("card-status"), preview = $("card-preview");
  var all = [], cur = null;

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

  function drawTags() {
    var box = $("tag-chips"), tags = cur.card.tags || [];
    box.textContent = "";
    var seen = {};
    P.SUGGESTED_TAGS.concat(tags).forEach(function (t) {
      if (seen[t.toLowerCase()]) return;
      seen[t.toLowerCase()] = true;
      var on = tags.some(function (x) { return x.toLowerCase() === t.toLowerCase(); });
      var b = el("button", "chip" + (on ? " on" : ""), t);
      b.type = "button";
      b.setAttribute("aria-pressed", on);
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
    return card;
  }

  function render() {
    var card = read();
    preview.textContent = "";
    preview.appendChild(dev.renderContact({ id: "preview", card: card }, T, { preview: true }));
  }

  function load() {
    form.elements.label.value = cur.label || "";
    dev.FIELDS.forEach(function (f) { form.elements[f].value = (cur.card && cur.card[f]) || ""; });
    cur.card = cur.card || {};
    drawSwitcher(); drawTags(); render();
  }

  function save(quiet) {
    if (!cur) return Promise.resolve();
    cur.label = form.elements.label.value.trim();
    cur.card = read();
    cur.card.updatedAt = new Date().toISOString();
    return P.save(all).then(function () { if (!quiet) status.textContent = T.card_saved || "Saved."; drawSwitcher(); });
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
