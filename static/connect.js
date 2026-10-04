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

  // myCard is what this share hands over: the chosen persona, chosen fields.
  function myCard() { return dev.personas.shareCard(); }

  // ensureName never blocks: scanning and showing a code work without any
  // card. With no name yet it shows the optional name form; once filled in,
  // onSaved gets the card so it can go to whoever connected meanwhile.
  function ensureName(form, then, onSaved) {
    dev.personas.choice().then(function (ch) {
      var card = ch.persona.card || (ch.persona.card = {});
      myCard().then(then);
      if (card.name) { form.hidden = true; return; }
      form.hidden = false;
      form.onsubmit = function (e) {
        e.preventDefault();
        var name = form.elements.name.value.trim(), about = form.elements.about ? form.elements.about.value.trim() : "";
        if (!name && !about) return;
        if (name) card.name = name;
        if (about) card.about = about;
        card.updatedAt = new Date().toISOString();
        dev.personas.save(ch.all).then(function () { form.hidden = true; return myCard(); }).then(function (c) { if (onSaved) onSaved(c); });
      };
    });
  }

  // sendCard hands our card over an existing connection.
  function sendCard(c, card) {
    return pair.send(c, { t: "card", card: card }).then(function () { c.cardSent = true; return dev.store.putContact(c); });
  }

  // drawPicker lets you choose, per share, which persona and which of its
  // fields to hand over. The choice is remembered for next time.
  function drawPicker(box, onChange) {
    if (!box) return;
    dev.personas.choice().then(function (ch) {
      box.textContent = "";
      if (ch.all.length > 1) {
        var bar = el("div", "chips");
        ch.all.forEach(function (p, i) {
          var b = el("button", "chip" + (p === ch.persona ? " on" : ""), p.label || (p.card && p.card.name) || "#" + (i + 1));
          b.type = "button";
          b.onclick = function () { dev.personas.setChoice(p.id, null).then(function () { drawPicker(box, onChange); onChange(); }); };
          bar.appendChild(b);
        });
        box.appendChild(bar);
      }
      var c = ch.persona.card || {}, fields = ch.fields;
      var avail = dev.FIELDS.filter(function (f) { return f !== "name" && c[f]; });
      if (c.tags && c.tags.length) avail.push("tags");
      if (!avail.length) return;
      var row = el("div", "chips");
      avail.forEach(function (f) {
        var on = !fields || fields.indexOf(f) >= 0;
        var label = f === "tags" ? c.tags.join(", ") : (T["field_" + f] || (f === "about" ? c.about : f));
        var b = el("button", "chip" + (on ? " on" : ""), (on ? "✓ " : "") + label);
        b.type = "button";
        b.setAttribute("aria-pressed", on);
        b.onclick = function () {
          var next = (fields || avail.slice()).filter(function (x) { return x !== f; });
          if (!on) next.push(f);
          dev.personas.setChoice(ch.persona.id, next).then(function () { drawPicker(box, onChange); onChange(); });
        };
        row.appendChild(b);
      });
      box.appendChild(el("p", "dim small", T.share_what || "What to share:"));
      box.appendChild(row);
    });
  }

  function contactItem(c) { return dev.renderContact(c, T); }

  // ---- /connect: show my code ----
  function showCode() {
    var qrBox = $("qr"), link = $("invite-link"), list = $("new-contacts"), status = $("connect-status");
    var poller;
    var shortBtn = $("make-short");
    function render(inv) {
      if (shortBtn) {
        $("short-code").textContent = "";
        shortBtn.hidden = false;
        shortBtn.onclick = function () {
          shortBtn.disabled = true;
          pair.shortLink(inv.payload).then(function (s) {
            shortBtn.hidden = true;
            $("short-code").textContent = s.url.replace(/^https?:\/\//, "");
          }).catch(function () { shortBtn.disabled = false; });
        };
      }
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
            added.forEach(function (c) { made.push(c); list.prepend(contactItem(c)); });
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
    var made = [];
    ensureName($("name-form"), function () { $("code-area").hidden = false; drawPicker($("share-picker"), function () {}); start(false); },
      function (card) {
        drawPicker($("share-picker"), function () {});
        made.forEach(function (c) { if (!c.cardSent) sendCard(c, card).catch(function () {}); });
      });
  }

  // ---- /c#v1.… : someone showed me their code ----
  function acceptCode() {
    var payload = location.hash.slice(1), status = $("accept-status"), list = $("accepted");
    if (!/^v1\./.test(payload)) { status.textContent = tr("bad_code"); return; }
    var made = null;
    ensureName($("name-form"), function (card) {
      function label() { myCard().then(function (c) { card = c; $("send-as").textContent = c.name ? tr("send_as", { name: c.name }) : tr("send_no_card"); }); }
      label();
      drawPicker($("share-picker"), label);
      $("accept-area").hidden = false;
      $("do-connect").onclick = function () {
        $("do-connect").disabled = true;
        status.textContent = tr("connecting");
        var tries = 0;
        (function attempt() {
          myCard().then(function (fresh) { return pair.accept(payload, fresh); }).then(function (c) {
            made = c;
            history.replaceState(null, "", "/c"); // the code has done its job
            $("accept-area").hidden = true;
            status.textContent = tr("waiting_their_card");
            var item = contactItem(c);
            list.appendChild(item);
            poll(function () {
              return pair.checkContact(c).then(function () {
                if (!c.card) return false;
                list.replaceChild(contactItem(c), item);
                status.textContent = tr("connected_with", { name: c.card.name || c.alias || tr("unnamed") });
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
    }, function (card) {
      $("send-as").textContent = tr("send_as", { name: card.name || "?" });
      drawPicker($("share-picker"), function () {
        myCard().then(function (c) { $("send-as").textContent = c.name ? tr("send_as", { name: c.name }) : tr("send_no_card"); });
      });
      if (made && !made.cardSent) sendCard(made, card).catch(function () {});
    });
  }

  if ($("qr")) showCode();
  if ($("accept-area")) acceptCode();
})();
