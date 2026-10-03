// Moving to a new device, bound to your account: the new device (signed in
// with your link or passkey) asks; another device signed into the same
// account sees the request and sends everything, end-to-end encrypted to
// the new device's key. Both show the same four emoji to compare.
(function () {
  "use strict";
  var T = window.KAFUMU_T || {}, dev = window.kafumuDevice;
  var pair = window.kafumuPair.create({ fetch: window.fetch.bind(window), store: dev.store, origin: location.origin });
  var $ = function (id) { return document.getElementById(id); };
  function tr(k, v) { var s = T[k] || k; Object.keys(v || {}).forEach(function (n) { s = s.split("{" + n + "}").join(v[n]); }); return s; }
  var EMOJI = "🐙🎸🌵🚲🍋🦊🌙⚓🍄🎈🐝🧭🍉🦉🌻🚂🐳🎲🍩🦋🌈🔔🥥🐢🎻🍒🦒🌊🔭🍀🐧🎯🍕🦀🌋🧩🍇🐼🎺🍓🦜🌵🚀🐞🎨🍑🦔🌍🔑🍔🐬🎤🍐🦄❄️🧲🍪🐌🎹🍊🦩🌴🛶🍯".match(/(\p{Extended_Pictographic}\uFE0F?)/gu);
  function code(pub) {
    return crypto.subtle.digest("SHA-256", new TextEncoder().encode(pub)).then(function (h) {
      return Array.from(new Uint8Array(h).slice(0, 4), function (b) { return EMOJI[b % EMOJI.length]; }).join(" ");
    });
  }
  function account(method, body) {
    return fetch("/account/move", { method: method, body: body, credentials: "same-origin" });
  }

  // New device: ask.
  var start = $("move-start");
  if (start) start.onclick = function () {
    var status = $("move-status");
    $("move-area").hidden = false;
    start.hidden = true;
    pair.invite(true, "move").then(function (inv) {
      return account("POST", new URLSearchParams({ box: inv.box, pub: inv.pub })).then(function (r) {
        if (r.status === 401) { status.textContent = tr("move_sign_in"); start.hidden = false; throw new Error("signed out"); }
        return code(inv.pub);
      }).then(function (c) {
        $("move-code").textContent = c;
        status.textContent = tr("move_waiting_other");
        var began = Date.now();
        (function tick() {
          pair.moveReceive().then(function (data) {
            if (!data) {
              if (Date.now() - began > 15 * 60 * 1000) { status.textContent = tr("stopped_listening"); start.hidden = false; return; }
              setTimeout(tick, Date.now() - began < 60000 ? 2000 : 5000);
              return;
            }
            status.textContent = tr("move_received", { n: (data.contacts || []).length, p: (data.personas || []).length });
            var ok = $("move-apply");
            ok.hidden = false;
            ok.onclick = function () {
              ok.hidden = true;
              dev.restore(data).then(function (n) { status.textContent = tr("restored", { n: n }); account("DELETE"); setTimeout(function () { location.reload(); }, 1200); });
            };
          }, function () { setTimeout(tick, 5000); });
        })();
      });
    }).catch(function () {});
  };

  // Old device: is another device of ours asking?
  var offer = $("move-offer");
  if (offer) {
    Promise.all([account("GET").then(function (r) { return r.ok ? r.json() : {}; }), dev.store.get("invite:move")]).then(function (r) {
      var req = r[0], mine = r[1];
      if (!req.box || (mine && mine.box === req.box)) return; // none, or it's this device's own request
      return code(req.pub).then(function (c) {
        $("move-offer-code").textContent = c;
        offer.hidden = false;
        $("move-send").onclick = function () {
          var b = this;
          b.disabled = true;
          $("move-offer-status").textContent = tr("connecting");
          dev.backup().then(function (bk) { return pair.moveSend("v1." + req.pub, bk); }).then(function () {
            $("move-offer-status").textContent = tr("move_sent");
            b.hidden = true;
          }, function (err) {
            b.disabled = false;
            $("move-offer-status").textContent = /too big/.test(err.message) ? tr("move_too_big") : tr("network_retry");
          });
        };
        $("move-ignore").onclick = function () { account("DELETE"); offer.hidden = true; };
      });
    }).catch(function () {});
  }
})();
