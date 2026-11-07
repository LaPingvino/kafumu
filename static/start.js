// Home (99): Your Kafumu for a device that knows you, the entry page for a
// newcomer. Everything here is read from this device (contacts, meetups,
// what came back, the areas opened) plus one anonymous bundle for the last
// area — the same one the area page loads, cached per cell.
(function () {
  "use strict";
  var T = window.KAFUMU_T || {}, dev = window.kafumuDevice, geo = window.kafumuGeo;
  function tr(k, v) { var s = T[k] || k; for (var n in v || {}) s = s.replace("{" + n + "}", v[n]); return s; }
  function $(id) { return document.getElementById(id); }
  function el(tag, cls, text) { var e = document.createElement(tag); if (cls) e.className = cls; if (text != null) e.textContent = text; return e; }
  function pref(k) { try { return localStorage.getItem(k); } catch (e) { return null; } }
  function json(k, d) { try { return JSON.parse(localStorage.getItem(k) || "null") || d; } catch (e) { return d; } }
  function short(s, n) { s = String(s || "").replace(/\s+/g, " ").trim(); return s.length > n ? s.slice(0, n - 1) + "…" : s; }
  function when(at, withDay) {
    var d = new Date(at); if (isNaN(d)) return "";
    var o = withDay ? { weekday: "short", day: "numeric", month: "short", hour: "2-digit", minute: "2-digit" } : { day: "numeric", month: "short", hour: "2-digit", minute: "2-digit" };
    return d.toLocaleString(window.KAFUMU_LOCALE, o);
  }
  var root = document.documentElement, body = document.body;
  var last = pref("kafumu.lastCell"), names = json("kafumu.cellNames", {});
  function placeName(c) { return names[c] || "#geo" + c; }

  // A row: an icon, a line in bold, a smaller line under it; the whole row is a link.
  function row(ul, href, icon, title, meta, hot) {
    var li = el("li"), a = el("a", "home-row" + (hot ? " hot" : ""));
    a.href = href;
    a.appendChild(el("span", "ico", icon));
    var t = el("span", "txt");
    t.appendChild(el("strong", "", title));
    if (meta) t.appendChild(el("span", "meta", meta));
    a.appendChild(t); li.appendChild(a); ul.appendChild(li);
  }
  function fill(id, n) { // the section's list or its empty line
    var sec = $(id), ul = sec.querySelector(".home-list");
    sec.querySelector(".empty").hidden = n > 0;
    if (ul) ul.hidden = !n;
  }

  // --- The place search, shared by the entry page and ✎ on Your Kafumu ---
  function go(c) { location.href = "/?cell=" + c; }
  function placeSearch(box) {
    var q = box.querySelector("input"), res = box.querySelector(".place-results"), status = box.querySelector("[role=status]"), seq = 0, timer;
    function search() {
      var v = q.value.trim(), my = ++seq;
      res.textContent = "";
      if (v.length < 2) return;
      var direct = geo && geo.parsePlace ? geo.parsePlace(v) : null; // a #geo cell, a plus code, lat,lon
      if (direct) { var b = el("button", "pill-sm", "#geo" + direct); b.type = "button"; b.onclick = function () { go(direct); }; var li = el("li"); li.appendChild(b); res.appendChild(li); }
      fetch("/places?q=" + encodeURIComponent(v)).then(function (r) { return r.ok ? r.json() : []; }).then(function (ms) {
        if (my !== seq) return;
        (ms || []).slice(0, 6).forEach(function (m) {
          var li = el("li"), b = el("button", "place-hit"); b.type = "button";
          b.appendChild(el("strong", "", m.name)); if (m.country) b.appendChild(el("span", "dim", " " + m.country));
          b.onclick = function () { names[m.cell] = m.name; try { localStorage.setItem("kafumu.cellNames", JSON.stringify(names)); } catch (e) {} go(m.cell); };
          li.appendChild(b); res.appendChild(li);
        });
      }).catch(function () {});
    }
    q.addEventListener("input", function () { clearTimeout(timer); timer = setTimeout(search, 200); });
    q.addEventListener("keydown", function (e) { if (e.key === "Enter") { e.preventDefault(); var b = res.querySelector("button"); if (b) b.click(); } });
    var loc = box.querySelector(".locate-btn");
    if (loc) loc.onclick = function () {
      if (!navigator.geolocation) { status.textContent = tr("no_location"); return; }
      status.textContent = tr("locating");
      navigator.geolocation.getCurrentPosition(function (pos) {
        // Rounded at once: only the ~5 km cell is kept. The area page takes it from here.
        try { localStorage.setItem("kafumu.autoLocate", "1"); } catch (e) {}
        go(geo.cell(pos.coords.latitude, pos.coords.longitude));
      }, function () { status.textContent = tr("unavailable"); }, { enableHighAccuracy: false, timeout: 10000, maximumAge: 600000 });
    };
  }
  placeSearch($("entry-pick"));

  // --- Entry page: "Are you in Lisbon?" from the server's guess ---
  var start = $("start"), g = start.dataset.guess, gn = start.dataset.guessName;
  if (g && gn && geo.validCell(g)) {
    $("entry-guess").querySelector("span").textContent = tr("guess_q", { place: gn });
    $("entry-guess").hidden = false;
    $("entry-guess-yes").onclick = function () { names[g] = gn; try { localStorage.setItem("kafumu.cellNames", JSON.stringify(names)); } catch (e) {} go(g); };
  }

  // --- Your Kafumu ---
  function mine() {
    root.classList.add("returning");
    if (body.dataset.me) $("mine-title").textContent = tr("start_welcome_name", { name: "@" + body.dataset.me });

    // No login saved here (99e): no account, or one without a passkey. "Later" lasts this visit only.
    var later = false; try { later = sessionStorage.getItem("kafumu.saveLater") === "1"; } catch (e) {}
    if (!body.dataset.passkey && !later) $("save-login").hidden = false;
    $("save-later").onclick = function () { $("save-login").hidden = true; try { sessionStorage.setItem("kafumu.saveLater", "1"); } catch (e) {} };

    // The place, and ✎ to change it right here.
    var pl = $("mine-place");
    if (last && geo.validCell(last)) { pl.href = "/?cell=" + last; pl.querySelector("span").textContent = placeName(last); }
    else pl.hidden = true;
    $("mine-change").onclick = function () {
      var box = $("mine-pick");
      if (!box.firstChild) {
        var src = $("entry-pick").cloneNode(true); src.id = ""; src.querySelectorAll("[id]").forEach(function (e) { e.removeAttribute("id"); });
        while (src.firstChild) box.appendChild(src.firstChild);
        placeSearch(box);
      }
      box.hidden = !box.hidden;
      if (!box.hidden) box.querySelector("input").focus();
    };

    // Your places: the areas opened lately (the area page keeps the list).
    var recent = json("kafumu.recentCells", []);
    if (last && recent.indexOf(last) < 0) recent.unshift(last);
    var chips = document.querySelector("#sec-places .place-chips");
    recent.slice(0, 6).forEach(function (c) {
      if (!geo.validCell(c)) return;
      var a = el("a", "chip" + (c === last ? " on" : ""), placeName(c)); a.href = "/?cell=" + c; chips.appendChild(a);
    });
    $("sec-places").hidden = !chips.children.length;

    if (!dev) return;
    // What came back to you: the newest three, unread first (Activity has the rest).
    var back = document.querySelector("#sec-back .home-list");
    (window.kafumuActivity ? window.kafumuActivity.gather() : Promise.resolve([])).then(function (all) {
      // A contact's message is in Your chats below; the rest (replies, reactions, answers, meetups, new contacts) here.
      var items = all.filter(function (it) { return !/^m:/.test(it.id); });
      items.sort(function (a, b) { return (b.unread - a.unread) || (new Date(b.at) - new Date(a.at)); });
      items.slice(0, 3).forEach(function (it) {
        // Someone did something: "Ana: text". Something happened to a thing of yours: the thing, then what.
        if (it.who) row(back, it.href, it.icon, it.who + ": " + short(it.text, 80), [it.about, when(it.at)].filter(Boolean).join(" · "), it.unread);
        else row(back, it.href, it.icon, short(it.about || it.text, 80), [it.about ? short(it.text, 60) : "", when(it.at)].filter(Boolean).join(" · "), it.unread);
      });
      fill("sec-back", Math.min(items.length, 3));
      if (window.kafumuActivity.badge) window.kafumuActivity.badge(all.filter(function (i) { return i.unread; }).length);
    }).catch(function () { fill("sec-back", 0); });

    // Your meetups: going or hosting, the next first.
    dev.store.get("myMeetups").then(function (ms) {
      var now = Date.now(), ul = document.querySelector("#sec-meetups .home-list");
      ms = (ms || []).filter(function (m) { return new Date(m.end || m.start).getTime() > now; })
        .sort(function (a, b) { return new Date(a.start) - new Date(b.start); });
      ms.slice(0, 3).forEach(function (m) {
        row(ul, "/meetups/" + encodeURIComponent(m.id), m.role === "host" ? "🎤" : "📅", short(m.title, 70),
          [when(m.start, true), m.venue, m.role === "host" ? tr("start_hosting") : tr("start_going")].filter(Boolean).join(" · "));
      });
      fill("sec-meetups", Math.min(ms.length, 3));
    }).catch(function () { fill("sec-meetups", 0); });

    // Your chats and your people, both from the contacts on this device.
    dev.store.contacts().then(function (cs) {
      if (!root.classList.contains("returning")) return;
      var chats = cs.filter(function (c) { return (c.messages || []).length; }).map(function (c) {
        var lm = c.messages.slice(-1)[0] || {}; return { c: c, at: lm.at || "", text: lm.text || "", me: !!lm.me };
      }).sort(function (a, b) { return (!!b.c.unreadMsgs - !!a.c.unreadMsgs) || String(b.at).localeCompare(String(a.at)); });
      var ul = document.querySelector("#sec-chats .home-list");
      chats.slice(0, 4).forEach(function (x) {
        row(ul, "/chat?k=c:" + encodeURIComponent(x.c.id), "💬", dev.nameOf(x.c, T), (x.me ? tr("start_you") + ": " : "") + short(x.text, 70) + (x.at ? " · " + when(x.at) : ""), !!x.c.unreadMsgs);
      });
      fill("sec-chats", Math.min(chats.length, 4));

      var week = new Date(Date.now() - 7 * 864e5).toISOString().slice(0, 10);
      var near = cs.filter(function (c) { return c.lastSeen && c.lastSeen.day >= week; })
        .sort(function (a, b) { return b.lastSeen.day.localeCompare(a.lastSeen.day); });
      var pu = document.querySelector("#sec-people .home-list");
      near.slice(0, 5).forEach(function (c) {
        row(pu, "/contacts#c-" + encodeURIComponent(c.id), "👋", dev.nameOf(c, T), tr("start_seen_near", { place: placeName(c.lastSeen.cell), when: new Date(c.lastSeen.day + "T12:00:00Z").toLocaleDateString(window.KAFUMU_LOCALE, { weekday: "long" }) }));
      });
      if (!near.length && cs.length) row(pu, "/contacts", "👥", tr("start_contacts_n", { n: cs.length }), tr("start_none_near"));
      fill("sec-people", pu.children.length);
    }).catch(function () { fill("sec-chats", 0); fill("sec-people", 0); });

    // Near you now: the last area's next meetups and newest messages (one bundle).
    if (!last || !geo.validCell(last)) { fill("sec-near", 0); return; }
    var nu = document.querySelector("#sec-near .home-list");
    document.querySelector("#sec-near .more").href = "/?cell=" + last;
    document.querySelectorAll("#sec-near .empty a, #sec-meetups .empty a").forEach(function (a) { a.href = "/?cell=" + last; });
    fetch("/bundle?cells=" + last).then(function (r) { if (!r.ok) throw new Error(r.status); return r.json(); }).then(function (b) {
      if (b.near && b.near.name) {
        var n = b.near.name + (b.near.city ? ", " + b.near.city : "");
        pl.querySelector("span").textContent = n;
        if (names[last] !== n) { names[last] = n; try { localStorage.setItem("kafumu.cellNames", JSON.stringify(names)); } catch (e) {} }
        var chip = document.querySelector("#sec-places .chip.on"); if (chip) chip.textContent = n;
      }
      var now = Date.now(), n = 0;
      (b.meetups || []).filter(function (m) { return new Date(m.end || m.start).getTime() > now; }).slice(0, 3).forEach(function (m) {
        row(nu, "/meetups/" + encodeURIComponent(m.id), "📅", short(m.title, 70), [when(m.start, true), m.venue, m.going ? tr("start_going_n", { n: m.going }) : ""].filter(Boolean).join(" · ")); n++;
      });
      (b.notes || []).filter(function (x) { return !(x.tags || []).some(function (t) { return /^re[0-9a-f]{10}$/.test(t); }); })
        .sort(function (a, c) { return new Date(c.at) - new Date(a.at); }).slice(0, 5 - n).forEach(function (x) {
          row(nu, "/?cell=" + last + "#re-" + String(x.id).slice(0, 10), "💬", short(String(x.text).split("\n")[0], 90), when(x.at)); n++;
        });
      fill("sec-near", n);
    }).catch(function () { fill("sec-near", 0); });
  }

  if (root.classList.contains("returning")) mine();
  else if (dev) dev.store.contacts().then(function (cs) { if (cs.length) mine(); }).catch(function () {}); // contacts, but no area yet
})();
