// "My card": edit what you hand to people you meet. Stored only on this device.
(function () {
  "use strict";
  var dev = window.kafumuDevice, T = window.KAFUMU_T || {};
  var form = document.getElementById("card-form"), status = document.getElementById("card-status");
  var preview = document.getElementById("card-preview");

  function read() {
    var card = {};
    dev.FIELDS.forEach(function (f) { var v = (form.elements[f].value || "").trim(); if (v) card[f] = v; });
    return card;
  }

  function render(card) {
    preview.textContent = "";
    var h = document.createElement("h3");
    h.textContent = card.name || "…";
    preview.appendChild(h);
    if (card.about) { var p = document.createElement("p"); p.textContent = card.about; preview.appendChild(p); }
    var ul = document.createElement("ul");
    dev.links(card).forEach(function (l) {
      var li = document.createElement("li"), a = document.createElement("a");
      a.href = l.href; a.target = "_blank"; a.rel = "noopener";
      a.textContent = (T["field_" + l.field] || l.field) + ": " + l.label;
      li.appendChild(a); ul.appendChild(li);
    });
    preview.appendChild(ul);
  }

  dev.store.get("card").then(function (card) {
    card = card || {};
    dev.FIELDS.forEach(function (f) { if (card[f]) form.elements[f].value = card[f]; });
    render(card);
  }).catch(function () { status.textContent = T.no_storage || "Storage unavailable"; });

  form.addEventListener("input", function () { render(read()); });
  form.addEventListener("submit", function (e) {
    e.preventDefault();
    var card = read();
    card.updatedAt = new Date().toISOString();
    dev.store.set("card", card).then(function () {
      status.textContent = T.card_saved || "Saved on this device.";
    }).catch(function () { status.textContent = T.no_storage || "Storage unavailable"; });
  });
})();
