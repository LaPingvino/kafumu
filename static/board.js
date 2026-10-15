// A CoffeeBucks group's board (97b): offers, requests and notes for the
// members only. Each post is an OLN line under a pair-style tag derived from
// the group's key, its text AES-GCM ciphertext: the server stores what it
// can't read, and it fades like any private line (a week).
(function (root) {
  "use strict";
  var enc = new TextEncoder(), dec = new TextDecoder(), subtle = crypto.subtle;
  function unb64(s) { s = s.replace(/-/g, "+").replace(/_/g, "/"); while (s.length % 4) s += "="; return Uint8Array.from(atob(s), function (c) { return c.charCodeAt(0); }); }
  function b64(b) { return btoa(String.fromCharCode.apply(null, b)).replace(/\+/g, "-").replace(/\//g, "_").replace(/=+$/, ""); }
  function hex(b) { return Array.from(new Uint8Array(b)).map(function (x) { return x.toString(16).padStart(2, "0"); }).join(""); }

  // tag: "p" + 32 hex of HMAC(key, "board") — fetched like a pair's mailbox.
  function tag(keyB64) {
    return subtle.importKey("raw", unb64(keyB64), { name: "HMAC", hash: "SHA-256" }, false, ["sign"])
      .then(function (k) { return subtle.sign("HMAC", k, enc.encode("board")); })
      .then(function (h) { return "p" + hex(h).slice(0, 32); });
  }
  function aes(keyB64) { return subtle.importKey("raw", unb64(keyB64), "AES-GCM", false, ["encrypt", "decrypt"]); }
  function seal(keyB64, obj) {
    var iv = crypto.getRandomValues(new Uint8Array(12));
    return aes(keyB64).then(function (k) {
      return subtle.encrypt({ name: "AES-GCM", iv: iv, additionalData: enc.encode("kafumu|board") }, k, enc.encode(JSON.stringify(obj)));
    }).then(function (ct) { var all = new Uint8Array(12 + ct.byteLength); all.set(iv); all.set(new Uint8Array(ct), 12); return b64(all); });
  }
  function open(keyB64, data) {
    var raw = unb64(data);
    return aes(keyB64).then(function (k) {
      return subtle.decrypt({ name: "AES-GCM", iv: raw.slice(0, 12), additionalData: enc.encode("kafumu|board") }, k, raw.slice(12));
    }).then(function (pt) { return JSON.parse(dec.decode(pt)); });
  }

  // post: {kind: offer|request|note|reply, text, price (micro), by, re, connect}
  function post(keyB64, p, mine) {
    var obj = { kind: p.kind || "note", text: String(p.text || "").slice(0, 500), price: p.price || 0, by: p.by || "", re: p.re || "", connect: p.connect || "", at: new Date().toISOString() };
    return Promise.all([tag(keyB64), seal(keyB64, obj)]).then(function (r) { return mine(r[1], "#" + r[0], 4); }).then(function (n) { obj.id = n && n.id; return obj; });
  }
  // read: the posts still there, opened; ones that don't open (another key,
  // garbage) are skipped. Newest first.
  function read(keyB64) {
    return tag(keyB64).then(function (t) { return fetch("/api/oln/pair/" + t, { credentials: "omit", cache: "no-store" }); })
      .then(function (r) { return r.ok ? r.json() : []; })
      .then(function (ms) {
        return Promise.all(ms.map(function (m) { return open(keyB64, m.text).then(function (o) { o.id = m.id; o.at = o.at || m.at; return o; }, function () { return null; }); }));
      }).then(function (os) { return os.filter(Boolean).sort(function (a, b) { return String(b.at).localeCompare(String(a.at)); }); });
  }
  root.kafumuBoard = { tag: tag, seal: seal, open: open, post: post, read: read };
})(typeof window !== "undefined" ? window : this);
