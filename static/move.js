// Share with self: move everything to a new device by scanning a code.
// The NEW device shows the code (Contacts → Move here); the OLD device scans
// it, lands on /m#…, and sends an encrypted copy of everything.
(function () {
  "use strict";
  var T = window.KAFUMU_T || {}, dev = window.kafumuDevice;
  var pair = window.kafumuPair.create({ fetch: window.fetch.bind(window), store: dev.store, origin: location.origin });
  var $ = function (id) { return document.getElementById(id); };
  function tr(k, v) { var s = T[k] || k; Object.keys(v || {}).forEach(function (n) { s = s.split("{" + n + "}").join(v[n]); }); return s; }

  // New device: show the move code and wait for the copy.
  var start = $("move-start");
  if (start) start.onclick = function () {
    start.hidden = true;
    var area = $("move-area"), status = $("move-status");
    area.hidden = false;
    pair.invite(true, "move").then(function (inv) {
      var q = qrcode(0, "M"); q.addData(inv.url); q.make();
      $("move-qr").innerHTML = q.createSvgTag({ cellSize: 6, margin: 2, scalable: true });
      status.textContent = tr("move_waiting");
      var began = Date.now();
      (function tick() {
        pair.moveReceive().then(function (data) {
          if (!data) {
            if (Date.now() - began > 300000) { status.textContent = tr("stopped_listening"); start.hidden = false; return; }
            setTimeout(tick, Date.now() - began < 30000 ? 2000 : 5000);
            return;
          }
          $("move-qr").innerHTML = "";
          var n = (data.contacts || []).length, p = (data.personas || []).length;
          status.textContent = tr("move_received", { n: n, p: p });
          var ok = $("move-apply");
          ok.hidden = false;
          ok.onclick = function () {
            ok.hidden = true;
            dev.restore(data).then(function (count) { status.textContent = tr("restored", { n: count }); setTimeout(function () { location.reload(); }, 1200); });
          };
        }, function () { setTimeout(tick, 5000); });
      })();
    });
  };

  // Old device: it scanned the new device's code.
  var send = $("move-send");
  if (send) {
    var payload = location.hash.slice(1), status = $("send-status");
    if (!/^v1\./.test(payload)) { status.textContent = tr("bad_code"); send.hidden = true; return; }
    dev.backup().then(function (b) { $("send-summary").textContent = tr("move_summary", { n: b.contacts.length, p: (b.personas || []).length }); });
    send.onclick = function () {
      send.disabled = true;
      status.textContent = tr("connecting");
      dev.backup().then(function (b) { return pair.moveSend(payload, b); }).then(function () {
        history.replaceState(null, "", "/m");
        send.hidden = true;
        status.textContent = tr("move_sent");
      }, function (err) {
        send.disabled = false;
        status.textContent = /too big/.test(err.message) ? tr("move_too_big") : tr("network_retry");
      });
    };
  }
})();
