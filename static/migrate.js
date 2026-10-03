// On the old address: move this device's Kafumu data to the new one.
// Opens <new>/import, hands over a backup (and the sign-in link) with
// postMessage — origin-checked on both ends — then goes there.
(function () {
  "use strict";
  var bar = document.getElementById("moved-bar");
  if (!bar) return;
  var to = bar.dataset.to, dev = window.kafumuDevice;
  function go() { location.href = to + location.pathname + location.search + location.hash; }
  function move() {
    Promise.all([dev.backup(), fetch("/account/link.json", { credentials: "same-origin" }).then(function (r) { return r.json(); }).catch(function () { return {}; })])
      .then(function (r) {
        var backup = r[0], link = r[1].link || "";
        var empty = !backup.contacts.length && !(backup.personas || []).some(function (p) { return p.card && p.card.name; });
        if (empty && !link) { go(); return; }
        var w = window.open(to + "/import", "_blank");
        // Never leave without the data: if the pop-up is blocked, say so.
        if (!w) { bar.querySelector(".text").textContent = (window.KAFUMU_T || {}).moved_popup || "Please allow pop-ups for this site, then tap again."; return; }
        window.addEventListener("message", function onMsg(e) {
          if (e.origin !== to || !e.data || !e.data.kafumu) return;
          if (e.data.kafumu === "ready") w.postMessage({ kafumu: "migrate", backup: backup, link: link }, to);
          if (e.data.kafumu === "done") { window.removeEventListener("message", onMsg); bar.querySelector(".text").textContent = "✓"; }
        });
      });
  }
  document.getElementById("moved-go").onclick = move;
})();
