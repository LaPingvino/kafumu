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

  // friendsAround: check in (only from a real location fix) and show which
  // of your contacts were in or next to this cell this week.
  function friendsAround(c, gps) {
    var dev = window.kafumuDevice;
    if (!dev || !window.kafumuPair) return;
    var pair = window.kafumuPair.create({ fetch: window.fetch.bind(window), store: dev.store, origin: location.origin });
    dev.store.contacts().then(function (cs) {
      cs = cs.filter(function (x) { return x.card; });
      if (!cs.length) return;
      return (gps ? pair.checkIn(c, cs) : Promise.resolve()).then(function () {
        return pair.around(rings(c, 1).map(function (p) { return p[0]; }), cs.slice(0, 30));
      }).then(function (hits) {
        var sec = $("friends-section"), list = $("friends");
        sec.hidden = !hits.length;
        list.textContent = "";
        var today = new Date().toISOString().slice(0, 10), yesterday = new Date(Date.now() - 864e5).toISOString().slice(0, 10);
        hits.forEach(function (h) {
          var li = document.createElement("li"), a = document.createElement("a");
          a.href = "/contacts";
          a.className = "meetup-row";
          var who = document.createElement("strong");
          who.textContent = h.contact.card.name;
          var when = h.day === today ? tr("today") : h.day === yesterday ? tr("yesterday")
            : new Date(h.day + "T12:00:00Z").toLocaleDateString(document.documentElement.lang, { weekday: "long" });
          var meta = document.createElement("div");
          meta.className = "meta";
          meta.textContent = tr(h.near ? "was_nearby" : "was_here", { when: when });
          a.appendChild(who); a.appendChild(meta); li.appendChild(a); list.appendChild(li);
        });
      });
    }).catch(function () {});
  }

  function show(c, how, gps) {
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
    friendsAround(c, !!gps);
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
        showMeetups(b.meetups || [], b.events || []);
        showPeople(b.people || []);
        render(b.posts || [], ringOf, places, c);
        var named = (b.places || []).filter(function (pt) { return pt.weight >= 0.5; })
          .slice(0, 3).map(function (pt) { return "#" + pt.tag; });
        $("list-note").textContent = named.length
          ? tr("also_tags", { tags: named.join(", ") })
          : "";
      })
      .catch(function () { $("list").innerHTML = ""; note(tr("load_failed")); });
  }

  // myTags: the tags on your personas, used only here on the device to
  // rank meetups — the server never learns them.
  var myTags = null;
  function loadMyTags() {
    if (myTags || !window.kafumuDevice) return Promise.resolve(myTags || {});
    return window.kafumuDevice.personas.list().then(function (ps) {
      myTags = {};
      ps.forEach(function (p) { ((p.card && p.card.tags) || []).forEach(function (t) { myTags[t.toLowerCase().replace(/\s+/g, "")] = true; }); });
      return myTags;
    }).catch(function () { return (myTags = {}); });
  }

  function showMeetups(ms, events) {
    var sec = $("meetups-section"), list = $("meetups");
    sec.hidden = !ms.length;
    if (!ms.length) return;
    loadMyTags().then(function (mine) {
      var live = {};
      events.forEach(function (e) { live[e.tag] = true; });
      function rank(m) {
        var hits = (m.tags || []).filter(function (t) { return mine[t.replace(/\s+/g, "")] || live[t]; }).length;
        var hours = (new Date(m.start) - Date.now()) / 36e5;
        return hits * 24 - Math.max(hours, 0);
      }
      ms.sort(function (a, b) { return rank(b) - rank(a); });
      list.textContent = "";
      ms.slice(0, 12).forEach(function (m) {
        var li = document.createElement("li"), a = document.createElement("a");
        a.href = "/meetups/" + m.id;
        a.className = "meetup-row";
        var s = new Date(m.start), e = new Date(m.end), now = Date.now();
        var when = document.createElement("div");
        when.className = "when";
        when.textContent = (s <= now && e > now ? tr("now") + " · " : s.toLocaleDateString(document.documentElement.lang, { weekday: "short", day: "numeric", month: "short" }) + " · ") +
          s.toLocaleTimeString(document.documentElement.lang, { hour: "2-digit", minute: "2-digit" });
        var title = document.createElement("strong");
        title.textContent = m.title;
        var meta = document.createElement("div");
        meta.className = "meta";
        meta.textContent = [m.venue, m.via ? tr("via", { site: m.via }) : tr("going_n", { n: m.going })].concat((m.tags || []).map(function (t) { return "#" + t; })).filter(Boolean).join(" · ");
        a.appendChild(when); a.appendChild(title); a.appendChild(meta);
        li.appendChild(a);
        list.appendChild(li);
      });
    });
  }

  // People ranking, on the device (amikumu's insight): someone who speaks
  // what you learn and learns what you speak first; then a shared language,
  // weighted by how rare it is here; then shared interests.
  function showPeople(people) {
    var me = window.KAFUMU_ME || { langs: [], tags: [], names: {} };
    var sec = $("people-section"), list = $("people");
    sec.hidden = !people.length;
    if (!people.length) return;
    function split(ls) {
      var speak = {}, learn = {};
      (ls || []).forEach(function (l) { var p = l.split("/"); if (p[1] === "learning") learn[p[0]] = true; else speak[p[0]] = true; });
      return { speak: speak, learn: learn };
    }
    var mine = split(me.langs), myTags = {};
    (me.tags || []).forEach(function (t) { myTags[t] = true; });
    var count = {};
    people.forEach(function (p) { (p.langs || []).forEach(function (l) { var c = l.split("/")[0]; count[c] = (count[c] || 0) + 1; }); });
    var name = function (c) { return (me.names || {})[c] || c; };
    people.forEach(function (p) {
      var th = split(p.langs), score = 0, why = [];
      var teach = Object.keys(th.speak).filter(function (c) { return mine.learn[c]; });
      var learnFromMe = Object.keys(th.learn).filter(function (c) { return mine.speak[c]; });
      if (teach.length && learnFromMe.length) { score += 10; why.push(tr("exchange", { a: name(teach[0]), b: name(learnFromMe[0]) })); }
      else if (teach.length) { score += 5; why.push(tr("speaks_learning", { lang: name(teach[0]) })); }
      else if (learnFromMe.length) { score += 4; why.push(tr("learns_speak", { lang: name(learnFromMe[0]) })); }
      Object.keys(th.speak).forEach(function (c) {
        if (mine.speak[c]) { score += 4 / count[c]; if (count[c] <= 3) why.push(tr("rare_shared", { lang: name(c) })); }
      });
      (p.tags || []).forEach(function (t) { if (myTags[t]) { score += 1; why.push("#" + t); } });
      p._score = score; p._why = why;
    });
    people.sort(function (a, b) { return b._score - a._score; });
    list.textContent = "";
    people.slice(0, 20).forEach(function (p) {
      var li = document.createElement("li");
      var head = document.createElement("strong");
      head.textContent = "@" + p.name;
      li.appendChild(head);
      if (p._why.length) { var w = document.createElement("div"); w.className = "why"; w.textContent = p._why.slice(0, 3).join(" · "); li.appendChild(w); }
      var meta = document.createElement("div");
      meta.className = "meta";
      meta.textContent = [p.bio, p.where ? "📍 " + p.where : "", (p.langs || []).map(function (l) {
        var x = l.split("/"); return name(x[0]) + (x[1] === "learning" ? " (" + tr("learning") + ")" : "");
      }).join(", ")].filter(Boolean).join(" · ");
      li.appendChild(meta);
      list.appendChild(li);
    });
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

  // myLangs: the languages to favour, as 639-3 codes — from your profile, or
  // from the browser's languages when you have none. Never sent anywhere.
  var myLangs = (function () {
    var me = window.KAFUMU_ME || {}, out = {};
    (me.langs || []).forEach(function (l) { out[l.split("/")[0]] = true; });
    if (!Object.keys(out).length) (navigator.languages || []).forEach(function (l) {
      var c = (me.from1 || {})[l.slice(0, 2).toLowerCase()];
      if (c) out[c] = true;
    });
    return out;
  })();

  // langMatch: a language hashtag or the post's own language in your list.
  function langMatch(p) {
    var me = window.KAFUMU_ME || {}, tags = me.langTags || {}, from1 = me.from1 || {}, hit = "";
    (p.tags || []).forEach(function (t) {
      var lt = tags[t];
      if (lt && (lt.codes.length === 0 ? false : lt.codes.some(function (c) { return myLangs[c]; }))) hit = "#" + t;
    });
    var postLang = ((p.langs || [])[0] || "").slice(0, 2);
    return { tag: hit, lang: !!myLangs[from1[postLang]] };
  }

  // score ranks on the device: #geo posts before place-tag posts, nearer
  // rings and fresher posts first, your languages up, noisy posts last.
  function score(p, ringOf, places) {
    var ageH = (Date.now() - new Date(p.createdAt).getTime()) / 36e5;
    var lm = langMatch(p);
    var s = -ageH / 24 - (p.bot ? 5 : 0) + (lm.tag ? 2 : 0) + (lm.lang ? 0.5 : 0);
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
      var lm = langMatch(p);
      if (lm.tag) { var lb = document.createElement("span"); lb.className = "badge lang"; lb.textContent = lm.tag; meta.appendChild(lb); }
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
      show(cell(pos.coords.latitude, pos.coords.longitude), tr("your_cell"), true);
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

  // Signals from people you've connected with. Only recent contacts are
  // checked on open (one mailbox read each), to keep this cheap.
  function checkSignals() {
    var dev = window.kafumuDevice;
    if (!dev || !window.kafumuPair) return;
    var pair = window.kafumuPair.create({ fetch: window.fetch.bind(window), store: dev.store, origin: location.origin });
    dev.store.contacts().then(function (cs) {
      var recent = cs.filter(function (c) { return c.card; }).sort(function (a, b) {
        var la = ((a.signals || [])[0] || {}).at || a.createdAt || "", lb = ((b.signals || [])[0] || {}).at || b.createdAt || "";
        return lb.localeCompare(la);
      }).slice(0, 10);
      return Promise.all(recent.map(function (c) { return pair.checkContact(c).catch(function () { return []; }); })).then(function () {
        var unread = recent.filter(function (c) { return (c.signals || []).some(function (x) { return x.unread; }); });
        $("signals-section").hidden = !unread.length;
        var list = $("signals");
        list.textContent = "";
        unread.forEach(function (c) {
          var li = document.createElement("li"), a = document.createElement("a");
          a.href = "/contacts";
          a.className = "meetup-row";
          var who = document.createElement("strong");
          who.textContent = c.card.name || "?";
          var what = document.createElement("div");
          what.className = "meta";
          what.textContent = dev.signalText(c.signals[0], window.KAFUMU_T || {});
          a.appendChild(who); a.appendChild(what); li.appendChild(a); list.appendChild(li);
        });
      });
    }).catch(function () {});
  }
  checkSignals();

  var given = $("here").dataset.cell;
  if (given && validCell(given)) show(given, tr("shared_cell"));
  else locate();

})();
