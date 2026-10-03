// #geo cells on the device: mirrors internal/geo (test/geo_test.mjs keeps
// them in sync). Coordinates never leave the device; only cells do.
(function (root) {
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

  root.kafumuGeo = { cell: cell, center: center, validCell: validCell, rings: rings, parsePlace: parsePlace };
})(typeof window !== "undefined" ? window : globalThis);
