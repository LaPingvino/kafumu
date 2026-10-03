// Area picker: a map of #geo cells around a place, on OpenStreetMap tiles.
// Tap a block to make it your area. Only the cell leaves the device.
(function (root) {
  "use strict";
  var G = root.kafumuGeo, N = 3; // rings around the centre: a 7×7 grid

  function x(lon, z) { return (lon + 180) / 360 * 256 * Math.pow(2, z); }
  function y(lat, z) {
    var r = lat * Math.PI / 180;
    return (1 - Math.log(Math.tan(r) + 1 / Math.cos(r)) / Math.PI) / 2 * 256 * Math.pow(2, z);
  }

  // render draws the grid around centre cell `around` into el, marks
  // `selected`, and calls onPick(cell) when a block is tapped.
  function render(el, around, selected, onPick) {
    var ctr = G.center(around), half = (N + 0.5) * 0.05;
    var west = ctr[1] - half, east = ctr[1] + half, north = ctr[0] + half, south = ctr[0] - half;
    var width = Math.min(el.clientWidth || 340, 520);
    var z = Math.max(8, Math.min(14, Math.floor(Math.log2(width * 360 / (256 * (east - west))))));
    var ox = x(west, z), oy = y(north, z), w = x(east, z) - ox, h = y(south, z) - oy;
    el.textContent = "";
    var map = document.createElement("div");
    map.className = "area-map";
    map.style.width = w + "px";
    map.style.height = h + "px";
    for (var tx = Math.floor(ox / 256); tx <= Math.floor((ox + w) / 256); tx++) {
      for (var ty = Math.floor(oy / 256); ty <= Math.floor((oy + h) / 256); ty++) {
        var img = document.createElement("img");
        img.src = "https://tile.openstreetmap.org/" + z + "/" + tx + "/" + ty + ".png";
        img.alt = ""; img.loading = "lazy"; img.referrerPolicy = "no-referrer-when-downgrade";
        img.style.left = (tx * 256 - ox) + "px";
        img.style.top = (ty * 256 - oy) + "px";
        map.appendChild(img);
      }
    }
    G.rings(around, N).forEach(function (pr) {
      var c = pr[0], cc = G.center(c);
      var b = document.createElement("button");
      b.type = "button";
      b.className = "cell" + (c === selected ? " on" : "");
      b.title = "#geo" + c;
      b.setAttribute("aria-label", "#geo" + c);
      var l = x(cc[1] - 0.025, z) - ox, t = y(cc[0] + 0.025, z) - oy;
      b.style.left = l + "px"; b.style.top = t + "px";
      b.style.width = (x(cc[1] + 0.025, z) - ox - l) + "px";
      b.style.height = (y(cc[0] - 0.025, z) - oy - t) + "px";
      b.onclick = function () { onPick(c); };
      map.appendChild(b);
    });
    var attr = document.createElement("small");
    attr.className = "attr";
    attr.innerHTML = '© <a href="https://www.openstreetmap.org/copyright" target="_blank" rel="noopener">OpenStreetMap</a>';
    map.appendChild(attr);
    el.appendChild(map);
  }

  root.kafumuArea = { render: render };
})(window);
