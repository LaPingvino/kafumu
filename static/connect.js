// /connect (show your code) and /c#… (someone else's code): the scan-to-
// connect pages. Pairing logic lives in pair.js; this is only the UI.
(function () {
  "use strict";
  var T = window.KAFUMU_T || {}, dev = window.kafumuDevice;
  var pair = window.kafumuPair.create({ fetch: window.fetch.bind(window), store: dev.store, origin: location.origin });
  var $ = function (id) { return document.getElementById(id); };
  function tr(k, v) { var s = T[k] || k; Object.keys(v || {}).forEach(function (n) { s = s.split("{" + n + "}").join(v[n]); }); return s; }
  function el(tag, cls, text) { var e = document.createElement(tag); if (cls) e.className = cls; if (text != null) e.textContent = text; return e; }

  // poll runs fn with backoff: every 2 s for 30 s, then every 5 s, and gives
  // up after 3 minutes (one Datastore read per poll; see LOOP-STATE).
  function poll(fn, onGiveUp) {
    var start = Date.now(), timer, stopped = false;
    function tick() {
      if (stopped) return;
      fn().then(function (done) {
        if (done || stopped) return;
        var age = Date.now() - start;
        if (age > 180000) { onGiveUp(); return; }
        timer = setTimeout(tick, age < 30000 ? 2000 : 5000);
      }, function () { timer = setTimeout(tick, 5000); });
    }
    tick();
    return { stop: function () { stopped = true; clearTimeout(timer); }, restart: function () { stopped = false; start = Date.now(); clearTimeout(timer); tick(); } };
  }

  function myCard() { return dev.store.get("card").then(function (c) { return c || {}; }); }

  // ensureName shows the inline name form when the card has no name yet.
  function ensureName(form, then) {
    myCard().then(function (card) {
      if (card.name) { form.hidden = true; then(card); return; }
      form.hidden = false;
      form.onsubmit = function (e) {
        e.preventDefault();
        card.name = form.elements.name.value.trim();
        if (form.elements.about && form.elements.about.value.trim()) card.about = form.elements.about.value.trim();
        if (!card.name) return;
        card.updatedAt = new Date().toISOString();
        dev.store.set("card", card).then(function () { form.hidden = true; then(card); });
      };
    });
  }

  function contactItem(c) { return dev.renderContact(c, T); }

  // ---- /connect: show my code ----
  function showCode() {
    var qrBox = $("qr"), link = $("invite-link"), list = $("new-contacts"), status = $("connect-status");
    var poller;
    function render(inv) {
      var q = qrcode(0, "M");
      q.addData(inv.url);
      q.make();
      qrBox.innerHTML = q.createSvgTag({ cellSize: 6, margin: 2, scalable: true });
      link.value = inv.url;
    }
    function start(fresh) {
      pair.invite(fresh).then(function (inv) {
        render(inv);
        status.textContent = tr("waiting_scan");
        if (poller) poller.stop();
        poller = poll(function () {
          return myCard().then(function (card) { return pair.checkInvite(card); }).then(function (added) {
            added.forEach(function (c) { list.prepend(contactItem(c)); });
            if (added.length) status.textContent = tr("connected_n", { n: list.children.length });
            return false; // keep listening: more people may scan the same code
          });
        }, function () { status.textContent = tr("stopped_listening"); $("listen-again").hidden = false; });
      });
    }
    $("new-code").onclick = function () { start(true); };
    $("listen-again").onclick = function () { $("listen-again").hidden = true; status.textContent = tr("waiting_scan"); poller.restart(); };
    $("share-link").onclick = function () {
      if (navigator.share) navigator.share({ title: "Kafumu", url: link.value }).catch(function () {});
      else if (navigator.clipboard) navigator.clipboard.writeText(link.value).then(function () { status.textContent = tr("link_copied"); });
    };
    ensureName($("name-form"), function () { $("code-area").hidden = false; start(false); });
  }

  // ---- /c#v1.… : someone showed me their code ----
  function acceptCode() {
    var payload = location.hash.slice(1), status = $("accept-status"), list = $("accepted");
    if (!/^v1\./.test(payload)) { status.textContent = tr("bad_code"); return; }
    ensureName($("name-form"), function (card) {
      $("send-as").textContent = tr("send_as", { name: card.name });
      $("accept-area").hidden = false;
      $("do-connect").onclick = function () {
        $("do-connect").disabled = true;
        status.textContent = tr("connecting");
        var tries = 0;
        (function attempt() {
          pair.accept(payload, card).then(function (c) {
            history.replaceState(null, "", "/c"); // the code has done its job
            $("accept-area").hidden = true;
            status.textContent = tr("waiting_their_card");
            var item = contactItem(c);
            list.appendChild(item);
            poll(function () {
              return pair.checkContact(c).then(function () {
                if (!c.card) return false;
                list.replaceChild(contactItem(c), item);
                status.textContent = tr("connected_with", { name: c.card.name || "?" });
                $("install-hint").hidden = false;
                return true;
              });
            }, function () { status.textContent = tr("their_card_later"); });
          }, function (err) {
            if (++tries < 3 && !/Kafumu code/.test(err.message)) { setTimeout(attempt, 1500 * tries); return; }
            $("do-connect").disabled = false;
            status.textContent = /Kafumu code/.test(err.message) ? tr("bad_code") : tr("network_retry");
          });
        })();
      };
    });
  }

  if ($("qr")) showCode();
  if ($("accept-area")) acceptCode();
})();
