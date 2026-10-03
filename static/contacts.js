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
      function send(c, msg) { return pair.send(c, msg); }
      cs.forEach(function (c) {
        var opts = { onDelete: function () { if (!list.children.length) show(); }, send: send };
        var li = dev.renderContact(c, T, opts);
        li.dataset.search = JSON.stringify([c.card, c.note]).toLowerCase();
        list.appendChild(li);
        // Cards on their way and new signals: one mailbox read per contact.
        pair.checkContact(c).then(function (got) {
          if (!got.length) return;
          var fresh = dev.renderContact(c, T, opts);
          fresh.dataset.search = li.dataset.search;
          list.replaceChild(fresh, li);
          li = fresh;
        }).catch(function () {});
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
  // Opening Contacts marks signals as seen.
  setTimeout(function () {
    dev.store.contacts().then(function (cs) {
      cs.forEach(function (c) {
        if ((c.signals || []).some(function (x) { return x.unread; })) {
          c.signals.forEach(function (x) { x.unread = false; });
          dev.store.putContact(c);
        }
      });
    });
  }, 4000);
})();
