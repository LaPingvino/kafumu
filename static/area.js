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

  // shift moves a cell by (dx, dy) cells.
  function shift(c, dx, dy) {
    var ctr = G.center(c);
    return G.cell(Math.max(-89.9, Math.min(89.9, ctr[0] + dy * 0.05)), ctr[1] + dx * 0.05);
  }

  // render draws the grid around centre cell `around` into el, marks
  // `selected`, and calls onPick(cell) when a block is tapped. Arrows and
  // dragging move the grid (onMove(newAround)).
  function render(el, around, selected, onPick, onMove) {
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
    if (onMove) {
      [["↑", 0, 3, "top:4px;left:50%;transform:translateX(-50%)"], ["↓", 0, -3, "bottom:22px;left:50%;transform:translateX(-50%)"],
       ["←", -3, 0, "left:4px;top:50%;transform:translateY(-50%)"], ["→", 3, 0, "right:4px;top:50%;transform:translateY(-50%)"]].forEach(function (a) {
        var b = document.createElement("button");
        b.type = "button"; b.className = "pan"; b.textContent = a[0]; b.style.cssText = a[3];
        b.onclick = function (e) { e.stopPropagation(); onMove(shift(around, a[1], a[2])); };
        map.appendChild(b);
      });
      // Drag to move: a drag of a block's width moves one cell.
      var start = null, moved = false, cellPx = (x(ctr[1] + 0.025, z) - x(ctr[1] - 0.025, z));
      map.addEventListener("pointerdown", function (e) { if (e.target.className === "pan") return; start = [e.clientX, e.clientY]; moved = false; });
      map.addEventListener("pointermove", function (e) {
        if (!start) return;
        var dx = e.clientX - start[0], dy = e.clientY - start[1];
        if (Math.abs(dx) + Math.abs(dy) > 8) { moved = true; map.style.transform = "translate(" + dx + "px," + dy + "px)"; }
      });
      var end = function (e) {
        if (!start) return;
        var dx = Math.round((e.clientX - start[0]) / cellPx), dy = Math.round((e.clientY - start[1]) / cellPx);
        start = null; map.style.transform = "";
        if (moved && (dx || dy)) onMove(shift(around, -dx, dy));
      };
      map.addEventListener("pointerup", end);
      map.addEventListener("pointercancel", function () { start = null; map.style.transform = ""; });
      // A drag must not also count as tapping a block.
      map.addEventListener("click", function (e) { if (moved) { e.stopPropagation(); e.preventDefault(); moved = false; } }, true);
    }
    var attr = document.createElement("small");
    attr.className = "attr";
    attr.innerHTML = '© <a href="https://www.openstreetmap.org/copyright" target="_blank" rel="noopener">OpenStreetMap</a>';
    map.appendChild(attr);
    el.appendChild(map);
  }

  root.kafumuArea = { render: render, shift: shift };
})(window);
