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
