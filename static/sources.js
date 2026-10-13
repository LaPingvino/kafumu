// Account → "Your sources" (81c): the sources you added, kept on this
// device; renew one before it fades (mined again, a week's worth) or take
// it down (only the account that added it can).
(function () {
  "use strict";
  var sec = document.getElementById("my-sources"), dev = window.kafumuDevice;
  if (!sec || !dev) return;
  var T = window.KAFUMU_T || {}, ul = sec.querySelector("ul");
  function tr(k, v) { var s = T[k] || k; for (var n in v || {}) s = s.split("{" + n + "}").join(v[n]); return s; }
  function el(tag, cls, text) { var e = document.createElement(tag); if (cls) e.className = cls; if (text != null) e.textContent = text; return e; }
  function save(ms) { return dev.store.set("mySources", ms); }
  function draw() {
    dev.store.get("mySources").then(function (ms) {
      ms = (ms || []).filter(function (m) { return !m.removed; });
      sec.hidden = !ms.length;
      ul.textContent = "";
      ms.forEach(function (m) {
        var li = el("li"), live = new Date(m.expires) > Date.now();
        var a = el("a", "", m.text.replace(/^📰\s*/, "")); a.href = "/?cell=" + m.cell;
        li.appendChild(a);
        li.appendChild(el("p", "dim small", "#geo" + m.cell + " · " + (live ? tr("source_until", { when: new Date(m.expires).toLocaleString(window.KAFUMU_LOCALE, { day: "numeric", month: "short", hour: "2-digit", minute: "2-digit" }) }) : tr("source_faded"))));
        var row = el("div", "actions"), st = el("p", "dim small");
        st.setAttribute("role", "status");
        var renew = el("button", "pill-sm", "↻ " + tr("source_renew")); renew.type = "button";
        renew.onclick = function () {
          renew.disabled = true;
          fetch("/api/oln/required?cell=" + m.cell).then(function (r) { return r.json(); }).then(function (j) {
            return window.kafumuOLN.post(m.text, m.kw, (j.bits || 4) + (m.extra || 8), function (tries) { st.textContent = tr("oln_working", { n: tries }); }, 0, true);
          }).then(function (n) {
            return dev.store.get("mySources").then(function (all) {
              all = (all || []).map(function (x) { return x.id === m.id ? Object.assign({}, x, { id: n.id, expires: n.expires }) : x; });
              return save(all);
            });
          }).then(draw, function (err) { st.textContent = /already here/.test(err.message) ? tr("source_still") : tr("network_retry"); renew.disabled = false; });
        };
        var rm = el("button", "pill-sm", tr("remove")); rm.type = "button";
        rm.onclick = function () {
          if (!confirm(tr("source_remove_confirm"))) return;
          rm.disabled = true;
          var gone = function () { return dev.store.get("mySources").then(function (all) { return save((all || []).filter(function (x) { return x.id !== m.id; })); }).then(draw); };
          if (!live) { gone(); return; }
          fetch("/api/oln/source/remove?id=" + encodeURIComponent(m.id), { method: "POST", credentials: "same-origin", headers: { "X-Kafumu-As": "1" } })
            .then(function (r) { if (!r.ok && r.status !== 403) throw new Error(r.status); return gone(); })
            .catch(function () { st.textContent = tr("network_retry"); rm.disabled = false; });
        };
        row.appendChild(renew); row.appendChild(rm);
        li.appendChild(row); li.appendChild(st);
        ul.appendChild(li);
      });
    }).catch(function () {});
  }
  draw();
})();
