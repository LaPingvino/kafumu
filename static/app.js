// On every page: register the service worker, offer to install, and pick up
// connect requests that arrived through a printed badge while we were away.
(function () {
  "use strict";
  var T = window.KAFUMU_T || {};
  if ("serviceWorker" in navigator) navigator.serviceWorker.register("/sw.js").catch(function () {});

  var standalone = window.matchMedia("(display-mode: standalone)").matches || navigator.standalone;
  var bar = document.getElementById("install-bar");
  var dismissed = false;
  try { dismissed = localStorage.getItem("kafumu.installDismissed") === "1"; } catch (e) {}
  function offer(text, onInstall) {
    if (!bar || standalone || dismissed) return;
    bar.querySelector(".text").textContent = text;
    var go = bar.querySelector(".go");
    go.hidden = !onInstall;
    go.onclick = onInstall;
    bar.querySelector(".no").onclick = function () { bar.hidden = true; try { localStorage.setItem("kafumu.installDismissed", "1"); } catch (e) {} };
    bar.hidden = false;
  }
  window.addEventListener("beforeinstallprompt", function (e) {
    e.preventDefault();
    offer(T.install_offer || "Install Kafumu", function () { e.prompt(); bar.hidden = true; });
  });
  if (/iphone|ipad|ipod/i.test(navigator.userAgent) && !standalone) offer(T.install_ios || "Share → Add to Home Screen");

  // What this page is (82a, 87a): a bar until you close it (per page);
  // the ? in the header brings it back.
  var intro = document.getElementById("intro"), again = document.getElementById("help-again");
  if (intro) {
    var key = "kafumu.helpSeen." + intro.dataset.help, seen = false;
    try { seen = localStorage.getItem(key) === "1" || (intro.dataset.help === "around" && localStorage.getItem("kafumu.introSeen") === "1"); } catch (e) {}
    var showIntro = function (on) { intro.hidden = !on; if (again) again.hidden = on; };
    showIntro(!seen);
    intro.querySelector(".no").onclick = function () { showIntro(false); try { localStorage.setItem(key, "1"); } catch (e) {} };
    if (again) again.onclick = function () { showIntro(true); try { localStorage.removeItem(key); localStorage.removeItem("kafumu.introSeen"); } catch (e) {} };
  }

  // Tours for the other pages (87b): "👀 Show me" on the page's bar walks
  // through what's on it, one thing at a time (Around has its own, richer one).
  var TOURS = {
    connect: [["#qr", "tour_connect_qr"], ["#share-picker", "tour_connect_pick"], ["#share-link", "tour_connect_share"], ["#make-short", "tour_connect_short"]],
    contacts: [["#contacts-tools", "tour_contacts_tools"], ["#contacts li.contact", "tour_contacts_card"], ["#export-vcf", "tour_contacts_backup"]],
    card: [["#personas", "tour_card_personas"], ["#card-form", "tour_card_form"], ["#tag-chips", "tour_card_tags"], ["#link", "tour_card_link"]],
    activity: [["#thread-filter", "tour_act_filter"], ["#threads > li", "tour_act_row"]]
  };
  var page = intro && intro.dataset.help, steps = TOURS[page];
  if (steps && document.getElementById("tour-start")) {
    var box, at = -1;
    var tourText = function (k) { return T[k] || k; };
    var shown = function () { return steps.filter(function (s) { var e = document.querySelector(s[0]); return e && !e.hidden && e.getClientRects().length > 0; }); /* offsetParent is null for fixed elements (the tab bar) */ };
    var go = function (i) {
      var old = document.querySelector(".tour-focus");
      if (old) old.classList.remove("tour-focus");
      var ss = shown();
      if (i < 0 || i >= ss.length) { if (box) box.hidden = true; at = -1; return; }
      if (!box) {
        box = document.createElement("div"); box.className = "tour card"; box.setAttribute("role", "dialog"); box.setAttribute("aria-live", "polite");
        box.innerHTML = '<p class="t"></p><div class="actions"><span class="dim small n"></span><button type="button" class="pill-sm b">←</button><button type="button" class="pill-sm suggested f">→</button><button type="button" class="pill-sm d">✓</button></div>';
        box.querySelector(".b").onclick = function () { go(at - 1); };
        box.querySelector(".f").onclick = function () { go(at + 1); };
        box.querySelector(".d").onclick = function () { go(-1); };
        document.body.appendChild(box);
      }
      at = i;
      var el = document.querySelector(ss[i][0]);
      el.classList.add("tour-focus");
      el.scrollIntoView({ block: "center", behavior: "smooth" });
      box.querySelector(".t").textContent = tourText(ss[i][1]);
      box.querySelector(".n").textContent = (i + 1) + " / " + ss.length;
      box.querySelector(".b").disabled = i === 0;
      box.querySelector(".f").hidden = i === ss.length - 1;
      box.hidden = false;
    };
    document.getElementById("tour-start").onclick = function () { go(0); };
  }

  // Badge hellos: one mailbox read, only if a badge code exists, on pages
  // that load the device store (Around, Connect, Contacts…).
  if (window.kafumuDevice && window.kafumuPair) {
    var dev = window.kafumuDevice;
    dev.store.get("invite:badge").then(function (inv) {
      if (!inv) return;
      var pair = window.kafumuPair.create({ fetch: window.fetch.bind(window), store: dev.store, origin: location.origin });
      return dev.personas.shareCard().then(function (card) { return pair.checkInvite(card, "badge"); });
    }).catch(function () {});
  }
})();
