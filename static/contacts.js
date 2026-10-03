// /contacts: everyone you've connected with, kept on this device.
(function () {
  "use strict";
  var T = window.KAFUMU_T || {}, dev = window.kafumuDevice;
  var pair = window.kafumuPair.create({ fetch: window.fetch.bind(window), store: dev.store, origin: location.origin });
  var $ = function (id) { return document.getElementById(id); };
  var list = $("contacts"), empty = $("contacts-empty"), filter = $("contacts-filter");

  function show() {
    dev.store.contacts().then(function (cs) {
      cs.sort(function (a, b) { return (b.createdAt || "").localeCompare(a.createdAt || ""); });
      list.textContent = "";
      empty.hidden = cs.length > 0;
      $("contacts-tools").hidden = cs.length === 0;
      cs.forEach(function (c) {
        var li = dev.renderContact(c, T, { onDelete: function () { if (!list.children.length) show(); } });
        li.dataset.search = JSON.stringify([c.card, c.note]).toLowerCase();
        list.appendChild(li);
        // A card may still be on its way (they scanned, we haven't heard back).
        if (!c.card) pair.checkContact(c).then(function () { if (c.card) list.replaceChild(dev.renderContact(c, T, { onDelete: show }), li); }).catch(function () {});
      });
    });
  }

  filter.addEventListener("input", function () {
    var q = filter.value.trim().toLowerCase();
    Array.prototype.forEach.call(list.children, function (li) { li.hidden = q && li.dataset.search.indexOf(q) < 0; });
  });
  $("export-vcf").onclick = function () {
    dev.store.contacts().then(function (cs) { dev.download("kafumu-contacts.vcf", "text/vcard", dev.vcards(cs)); });
  };
  $("export-json").onclick = function () {
    dev.backup().then(function (b) { dev.download("kafumu-backup-" + b.exportedAt.slice(0, 10) + ".json", "application/json", JSON.stringify(b, null, 1)); });
  };
  $("import-json").onchange = function () {
    var f = this.files[0];
    if (!f) return;
    f.text().then(function (t) { return dev.restore(JSON.parse(t)); })
      .then(function (n) { $("contacts-status").textContent = (T.restored || "Restored {n}").replace("{n}", n); show(); })
      .catch(function () { $("contacts-status").textContent = T.restore_failed || "Not a Kafumu backup."; });
  };
  show();
})();
