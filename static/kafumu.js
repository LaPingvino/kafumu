// Kafumu client. The cell is computed here; coordinates never leave the device.
(function () {
  "use strict";
  var ALPHABET = "23456789cfghjmpqrvwx";
  var CELL_DEG = 0.05;

  // cell mirrors internal/geo.Cell: first 6 plus-code chars, lowercased.
  function cell(lat, lon) {
    lat = Math.min(Math.max(lat, -90), 90 - 1e-9) + 90;
    lon = (((lon + 180) % 360) + 360) % 360;
    var out = "";
    [20, 1, CELL_DEG].forEach(function (res) {
      var la = Math.min(Math.floor(lat / res + 1e-9), 19);
      var lo = Math.min(Math.floor(lon / res + 1e-9), 19);
      out += ALPHABET[la] + ALPHABET[lo];
      lat -= la * res;
      lon -= lo * res;
    });
    return out;
  }

  function center(c) {
    var lat = 0, lon = 0;
    [20, 1, CELL_DEG].forEach(function (res, i) {
      lat += ALPHABET.indexOf(c[2 * i]) * res;
      lon += ALPHABET.indexOf(c[2 * i + 1]) * res;
    });
    return [lat - 90 + CELL_DEG / 2, lon - 180 + CELL_DEG / 2];
  }

  function validCell(s) {
    if (!/^[23456789cfghjmpqrvwx]{6}$/.test(s)) return false;
    return ALPHABET.indexOf(s[0]) <= 8 && ALPHABET.indexOf(s[1]) <= 17;
  }

  // rings mirrors internal/geo.Rings: Chebyshev rings 0..r, nearest first.
  // Returns [[cell, ring], ...].
  function rings(c, r) {
    var ctr = center(c), out = [[c, 0]], seen = {};
    seen[c] = true;
    for (var k = 1; k <= r; k++) {
      for (var dy = -k; dy <= k; dy++) {
        for (var dx = -k; dx <= k; dx++) {
          if (Math.max(Math.abs(dx), Math.abs(dy)) !== k) continue;
          var la = ctr[0] + dy * CELL_DEG;
          if (la < -90 || la >= 90) continue;
          var n = cell(la, ctr[1] + dx * CELL_DEG);
          if (!seen[n]) { seen[n] = true; out.push([n, k]); }
        }
      }
    }
    return out;
  }

  // parsePlace accepts a #geo cell, a full plus code, or "lat,lon".
  function parsePlace(s) {
    s = s.trim().toLowerCase().replace(/^#?geo/, "");
    var m = s.match(/^(-?\d+(?:\.\d+)?)\s*[, ]\s*(-?\d+(?:\.\d+)?)$/);
    if (m) return cell(parseFloat(m[1]), parseFloat(m[2]));
    s = s.replace(/[\s+]/g, "");
    if (s.length >= 6 && validCell(s.slice(0, 6))) return s.slice(0, 6);
    return null;
  }

  var $ = function (id) { return document.getElementById(id); };

  function setStatus(msg) { $("status").textContent = msg; }

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
      else if (navigator.clipboard) navigator.clipboard.writeText(url).then(function () { setStatus("Link copied."); });
    };
    setStatus(how);
    try { localStorage.setItem("kafumu.lastCell", c); } catch (e) {}
    load(c);
  }

  function load(c) {
    var near = rings(c, 2);
    var ringOf = {};
    near.forEach(function (p) { ringOf["geo" + p[0]] = p[1]; });
    $("list").innerHTML = '<li class="muted">Looking around…</li>';
    fetch("/bundle?cells=" + near.map(function (p) { return p[0]; }).join(","))
      .then(function (r) { if (!r.ok) throw new Error(r.status); return r.json(); })
      .then(function (b) { render(b.posts || [], ringOf, c); })
      .catch(function () { $("list").innerHTML = '<li class="muted">Could not load the area right now.</li>'; });
  }

  // score ranks on the device: nearer rings and fresher posts first, bots last.
  function score(p, ringOf) {
    var ageH = (Date.now() - new Date(p.createdAt).getTime()) / 36e5;
    var ring = ringOf[p.via] || 0;
    return -ageH / 24 - ring * 0.5 - (p.bot ? 5 : 0);
  }

  function render(posts, ringOf, c) {
    var list = $("list");
    list.textContent = "";
    if (!posts.length) {
      var li = document.createElement("li");
      li.className = "muted";
      li.textContent = "Nothing tagged #geo" + c + " or its neighbours yet. Be the first: post with the tag above.";
      list.appendChild(li);
      return;
    }
    posts.sort(function (a, b) { return score(b, ringOf) - score(a, ringOf); });
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
      via.textContent = "#" + p.via + (ringOf[p.via] ? "" : " · here");
      meta.appendChild(via);
      if (p.bot) { var b = document.createElement("span"); b.className = "badge"; b.textContent = "bot"; meta.appendChild(b); }
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
    if (s < 3600) return Math.max(1, Math.round(s / 60)) + " min ago";
    if (s < 86400) return Math.round(s / 3600) + " h ago";
    return Math.round(s / 86400) + " d ago";
  }

  function locate() {
    if (!navigator.geolocation) { setStatus("No location available; pick a place below."); return; }
    navigator.geolocation.getCurrentPosition(function (pos) {
      // Round immediately: only the 5 km cell is kept.
      show(cell(pos.coords.latitude, pos.coords.longitude), "Your cell, computed on this device:");
    }, function () {
      var last = null;
      try { last = localStorage.getItem("kafumu.lastCell"); } catch (e) {}
      if (last && validCell(last)) show(last, "Location unavailable; showing your last cell:");
      else { setStatus("Location unavailable; pick a place below."); $("manual").open = true; }
    }, { enableHighAccuracy: false, timeout: 10000, maximumAge: 600000 });
  }

  $("manual-form").addEventListener("submit", function (e) {
    e.preventDefault();
    var c = parsePlace(this.where.value);
    if (!c) { setStatus("That doesn't look like a cell, plus code or lat,lon."); return; }
    history.replaceState(null, "", "/?cell=" + c);
    show(c, "Showing a chosen cell:");
  });

  var given = $("here").dataset.cell;
  if (given && validCell(given)) show(given, "Showing a shared cell:");
  else locate();

  // Exposed for tests and the console.
  window.kafumu = { cell: cell, rings: rings, parsePlace: parsePlace };
})();
