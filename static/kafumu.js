// Kafumu client. The cell is computed here; coordinates never leave the device.
(function () {
  "use strict";
  var G = window.kafumuGeo, cell = G.cell, rings = G.rings, validCell = G.validCell, parsePlace = G.parsePlace;

  var $ = function (id) { return document.getElementById(id); };

  // tr looks up a UI string handed over by the server (window.KAFUMU_T).
  function tr(key, vars) {
    var s = (window.KAFUMU_T || {})[key] || key;
    Object.keys(vars || {}).forEach(function (k) { s = s.split("{" + k + "}").join(vars[k]); });
    return s;
  }

  function setStatus(msg) { $("status").textContent = msg; }
  function note(msg) { $("list-note").textContent = msg; }

  function show(c, how) {
    var tag = "#geo" + c;
    $("cell-tag").textContent = tag;
    document.querySelector(".cell-tag").hidden = false;
    $("cell-actions").hidden = false;
    $("compose").href = "https://bsky.app/intent/compose?text=" + encodeURIComponent("\n\n" + tag);
    $("share").onclick = function (e) {
      e.preventDefault();
      var url = location.origin + "/?cell=" + c;
      if (navigator.share) navigator.share({ title: tag, url: url }).catch(function () {});
      else if (navigator.clipboard) navigator.clipboard.writeText(url).then(function () { setStatus(tr("link_copied")); });
    };
    setStatus(how);
    try { localStorage.setItem("kafumu.lastCell", c); } catch (e) {}
    load(c);
  }

  function load(c) {
    var near = rings(c, 2);
    var ringOf = {};
    near.forEach(function (p) { ringOf["geo" + p[0]] = p[1]; });
    $("list").innerHTML = "";
    note(tr("looking"));
    fetch("/bundle?cells=" + near.map(function (p) { return p[0]; }).join(","))
      .then(function (r) { if (!r.ok) throw new Error(r.status); return r.json(); })
      .then(function (b) {
        var places = {};
        (b.places || []).forEach(function (pt) { places[pt.tag] = pt; });
        // Event tags (#websummit) count as local as a #geo tag while they run.
        (b.events || []).forEach(function (e) { ringOf[e.tag] = e.live ? 0 : 1; });
        showEvents(b.events || [], c);
        render(b.posts || [], ringOf, places, c);
        var named = (b.places || []).filter(function (pt) { return pt.weight >= 0.5; })
          .slice(0, 3).map(function (pt) { return "#" + pt.tag; });
        $("list-note").textContent = named.length
          ? tr("also_tags", { tags: named.join(", ") })
          : "";
      })
      .catch(function () { $("list").innerHTML = ""; note(tr("load_failed")); });
  }

  function showEvents(events, c) {
    var box = $("events");
    box.textContent = "";
    box.hidden = !events.length;
    events.forEach(function (e) {
      var p = document.createElement("p");
      var strong = document.createElement("strong");
      strong.textContent = e.name;
      p.appendChild(strong);
      p.appendChild(document.createTextNode(e.live
        ? " " + tr("event_live", { name: "", tag: "#" + e.tag }).trim()
        : " " + tr("event_upcoming", { name: "", from: e.from, to: e.to, tag: "#" + e.tag }).trim()));
      var a = document.createElement("a");
      a.href = "https://bsky.app/intent/compose?text=" + encodeURIComponent("\n\n#geo" + c + " #" + e.tag);
      a.target = "_blank"; a.rel = "noopener";
      a.textContent = " " + tr("post_with", { tag: "#" + e.tag });
      p.appendChild(a);
      box.appendChild(p);
    });
  }

  // score ranks on the device: #geo posts before place-tag posts, nearer
  // rings and fresher posts first, noisy (ambiguous, bot) posts last.
  function score(p, ringOf, places) {
    var ageH = (Date.now() - new Date(p.createdAt).getTime()) / 36e5;
    var s = -ageH / 24 - (p.bot ? 5 : 0);
    if (p.via in ringOf) return s - ringOf[p.via] * 0.5;
    var pt = places[p.via];
    // A place-tag post that also carries a #geo tag of this area is strong.
    var geoToo = (p.tags || []).some(function (t) { return t in ringOf; });
    return s - (geoToo ? 0.5 : 2) - (1 - (pt ? pt.weight : 0)) * 3;
  }

  function render(posts, ringOf, places, c) {
    var list = $("list");
    list.textContent = "";
    if (!posts.length) {
      var li = document.createElement("li");
      li.className = "muted";
      li.textContent = tr("empty", { tag: "#geo" + c });
      list.appendChild(li);
      return;
    }
    posts.sort(function (a, b) { return score(b, ringOf, places) - score(a, ringOf, places); });
    // At most two posts per author, so one busy account (news feeds, flight
    // trackers on #ams) can't fill the list.
    var perAuthor = {};
    posts = posts.filter(function (p) {
      perAuthor[p.handle] = (perAuthor[p.handle] || 0) + 1;
      return perAuthor[p.handle] <= 2;
    });
    posts.slice(0, 50).forEach(function (p) {
      var li = document.createElement("li");
      if (p.bot) li.className = "bot";
      var meta = document.createElement("div");
      meta.className = "meta";
      var who = document.createElement("a");
      who.href = p.url; who.target = "_blank"; who.rel = "noopener";
      who.textContent = (p.name || p.handle) + " · " + ago(p.createdAt);
      meta.appendChild(who);
      var via = document.createElement("span");
      via.className = "badge";
      if (p.via in ringOf) via.textContent = "#" + p.via + (ringOf[p.via] ? "" : " · " + tr("here"));
      else via.textContent = tr("from", { tag: "#" + p.via }) + (places[p.via] && places[p.via].ambiguous ? " " + tr("maybe_elsewhere") : "");
      meta.appendChild(via);
      if (p.bot) { var b = document.createElement("span"); b.className = "badge"; b.textContent = tr("bot"); meta.appendChild(b); }
      var text = document.createElement("p");
      text.className = "text";
      text.textContent = p.text;
      li.appendChild(meta);
      li.appendChild(text);
      list.appendChild(li);
    });
  }

  function ago(iso) {
    var s = (Date.now() - new Date(iso).getTime()) / 1000;
    if (s < 3600) return tr("min_ago", { n: Math.max(1, Math.round(s / 60)) });
    if (s < 86400) return tr("h_ago", { n: Math.round(s / 3600) });
    return tr("d_ago", { n: Math.round(s / 86400) });
  }

  function locate() {
    if (!navigator.geolocation) { setStatus(tr("no_location")); return; }
    navigator.geolocation.getCurrentPosition(function (pos) {
      // Round immediately: only the 5 km cell is kept.
      show(cell(pos.coords.latitude, pos.coords.longitude), tr("your_cell"));
    }, function () {
      var last = null;
      try { last = localStorage.getItem("kafumu.lastCell"); } catch (e) {}
      if (last && validCell(last)) show(last, tr("last_cell"));
      else { setStatus(tr("unavailable")); $("manual").open = true; }
    }, { enableHighAccuracy: false, timeout: 10000, maximumAge: 600000 });
  }

  $("manual-form").addEventListener("submit", function (e) {
    e.preventDefault();
    var c = parsePlace(this.where.value);
    if (!c) { setStatus(tr("bad_place")); return; }
    history.replaceState(null, "", "/?cell=" + c);
    show(c, tr("chosen_cell"));
  });

  var given = $("here").dataset.cell;
  if (given && validCell(given)) show(given, tr("shared_cell"));
  else locate();

})();
