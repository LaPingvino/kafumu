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
    var vis = function (sel) { return Array.prototype.filter.call(document.querySelectorAll(sel), function (e) { return !e.hidden && e.getClientRects().length > 0; })[0] || null; };
    var shown = function () { return steps.filter(function (s) { return !!vis(s[0]); }); };
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
      var el = vis(ss[i][0]);
      el.classList.add("tour-focus");
      el.scrollIntoView({ block: "start", behavior: "smooth" }); // near the top: the box fits below it
      var put = function () { var r = el.getBoundingClientRect(); box.classList.toggle("at-top", r.top + r.height / 2 > window.innerHeight / 2); }; // out of the way
      put(); setTimeout(put, 400);
      box.querySelector(".t").textContent = tourText(ss[i][1]);
      box.querySelector(".n").textContent = (i + 1) + " / " + ss.length;
      box.querySelector(".b").disabled = i === 0;
      box.querySelector(".f").hidden = i === ss.length - 1;
      box.hidden = false;
    };
    document.getElementById("tour-start").onclick = function () { go(0); };
  }

  // Scrolling chip rows (91a): each sits in a wrapper that shows a fade and
  // a ‹ › when there's more to that side; it keeps up with new chips.
  function hint(row) {
    if (row.dataset.hinted) return;
    row.dataset.hinted = "1";
    var wrap = document.createElement("div"); wrap.className = "scroll-wrap";
    row.parentNode.insertBefore(wrap, row); wrap.appendChild(row);
    // ‹ › are buttons too (Joop: on a computer there's no swipe): each moves
    // the row most of its width.
    var mk = function (side, label) {
      var b = document.createElement("button");
      b.type = "button"; b.className = "scroll-btn " + side; b.textContent = label; b.tabIndex = -1;
      b.setAttribute("aria-hidden", "true"); // the chips themselves stay reachable by keyboard
      b.onclick = function () { row.scrollBy({ left: (side === "right" ? 1 : -1) * row.clientWidth * 0.8, behavior: "smooth" }); };
      wrap.appendChild(b);
      return b;
    };
    var left = mk("left", "‹"), right = mk("right", "›");
    var upd = function () {
      var more = row.scrollWidth - row.clientWidth > 4;
      var r = more && row.scrollLeft < row.scrollWidth - row.clientWidth - 4, l = more && row.scrollLeft > 4;
      wrap.classList.toggle("more-right", r); wrap.classList.toggle("more-left", l);
      right.hidden = !r; left.hidden = !l;
      wrap.hidden = row.hidden;
    };
    row.addEventListener("scroll", upd, { passive: true });
    // The mouse wheel scrolls the row sideways while you point at it (only
    // while there's more that way; otherwise the page scrolls as usual).
    row.addEventListener("wheel", function (e) {
      if (Math.abs(e.deltaY) <= Math.abs(e.deltaX) || row.scrollWidth - row.clientWidth <= 4) return;
      var atEnd = e.deltaY > 0 ? row.scrollLeft >= row.scrollWidth - row.clientWidth - 2 : row.scrollLeft <= 2;
      if (atEnd) return;
      e.preventDefault();
      row.scrollLeft += e.deltaY;
    }, { passive: false });
    window.addEventListener("resize", upd);
    var later = function () { requestAnimationFrame(upd); }; // measure once laid out
    if (window.MutationObserver) new MutationObserver(later).observe(row, { childList: true, subtree: true, attributes: true, attributeFilter: ["hidden"] });
    if (window.ResizeObserver) new ResizeObserver(later).observe(row);
    upd();
  }
  document.querySelectorAll(".chips.scroll-row").forEach(hint);

  // Esc closes what's open on top (92b): the tour, then the last panel with
  // a ✕ (its own close button, so each panel's own clean-up runs).
  document.addEventListener("keydown", function (e) {
    if (e.key !== "Escape" || e.defaultPrevented) return;
    var tour = document.querySelector(".tour:not([hidden])");
    if (tour) { var d = tour.querySelector(".d, #tour-done"); if (d) d.click(); return; }
    var open = Array.prototype.filter.call(document.querySelectorAll("[data-close]"), function (x) { return x.parentElement && !x.parentElement.hidden && x.getClientRects().length; });
    if (open.length) { open[open.length - 1].click(); }
  });

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
