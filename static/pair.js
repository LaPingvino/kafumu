// Kafumu pairing: "scan to connect" without the server learning who met whom.
//
//   A shows a QR: origin/c#v1.<A's ephemeral P-256 public key>.
//   B opens it, makes its own ephemeral key, and both sides derive the same
//   pair key with ECDH + HKDF. B posts a hello (its public key in clear, its
//   card encrypted) to A's invite box; A answers with its card in B's box.
//
// Every scanner of the same QR gets its own pair key, so scanners can't read
// each other's hellos. Box ids are HMACs of the pair key; the direction is
// bound into the AES-GCM additional data. The server only ever sees random
// ids and ciphertext (VISION.md, rules 1 and 3).
(function (root) {
  "use strict";
  var enc = new TextEncoder(), dec = new TextDecoder();
  var INVITE_TTL = 60 * 60 * 1000;

  function b64(bytes) {
    var s = "";
    new Uint8Array(bytes).forEach(function (b) { s += String.fromCharCode(b); });
    return btoa(s).replace(/\+/g, "-").replace(/\//g, "_").replace(/=+$/, "");
  }
  function unb64(s) {
    s = s.replace(/-/g, "+").replace(/_/g, "/");
    while (s.length % 4) s += "=";
    var bin = atob(s), out = new Uint8Array(bin.length);
    for (var i = 0; i < bin.length; i++) out[i] = bin.charCodeAt(i);
    return out;
  }
  function hex(bytes) {
    return Array.from(new Uint8Array(bytes)).map(function (b) { return b.toString(16).padStart(2, "0"); }).join("");
  }
  function concat(a, b) { var o = new Uint8Array(a.length + b.length); o.set(a); o.set(b, a.length); return o; }

  function create(opts) {
    var subtle = (opts.crypto || root.crypto).subtle, cryptoObj = opts.crypto || root.crypto;
    var fetchFn = opts.fetch, store = opts.store, base = opts.origin || "";
    var ECDH = { name: "ECDH", namedCurve: "P-256" };

    function genKey() { return subtle.generateKey(ECDH, false, ["deriveBits"]); }
    function rawPub(k) { return subtle.exportKey("raw", k.publicKey).then(function (r) { return new Uint8Array(r); }); }

    // pairKey: ECDH → HKDF, salted with both public keys (A's first).
    function pairKey(priv, theirRaw, aRaw, bRaw) {
      return subtle.importKey("raw", theirRaw, ECDH, false, []).then(function (pub) {
        return subtle.deriveBits({ name: "ECDH", public: pub }, priv, 256);
      }).then(function (shared) {
        return subtle.importKey("raw", shared, "HKDF", false, ["deriveBits"]);
      }).then(function (ikm) {
        return subtle.deriveBits({ name: "HKDF", hash: "SHA-256", salt: concat(aRaw, bRaw), info: enc.encode("kafumu pair v1") }, ikm, 256);
      }).then(function (bits) { return new Uint8Array(bits); });
    }

    function hmacHex(keyBytes, label) {
      return subtle.importKey("raw", keyBytes, { name: "HMAC", hash: "SHA-256" }, false, ["sign"])
        .then(function (k) { return subtle.sign("HMAC", k, enc.encode(label)); }).then(hex);
    }
    // inbox of a role: 0 = the one who showed the QR, 1 = the scanner.
    function boxOf(key, role) { return hmacHex(key, "box" + role); }
    function contactID(key) { return hmacHex(key, "id").then(function (h) { return h.slice(0, 32); }); }

    function aesKey(key) {
      return subtle.importKey("raw", key, "HKDF", false, ["deriveKey"]).then(function (ikm) {
        return subtle.deriveKey({ name: "HKDF", hash: "SHA-256", salt: new Uint8Array(0), info: enc.encode("kafumu enc v1") },
          ikm, { name: "AES-GCM", length: 256 }, false, ["encrypt", "decrypt"]);
      });
    }
    function seal(key, box, obj) {
      var iv = cryptoObj.getRandomValues(new Uint8Array(12));
      return aesKey(key).then(function (k) {
        return subtle.encrypt({ name: "AES-GCM", iv: iv, additionalData: enc.encode("kafumu|" + box) }, k, enc.encode(JSON.stringify(obj)));
      }).then(function (ct) { return b64(concat(iv, new Uint8Array(ct))); });
    }
    function open(key, box, data) {
      var raw = unb64(data);
      return aesKey(key).then(function (k) {
        return subtle.decrypt({ name: "AES-GCM", iv: raw.slice(0, 12), additionalData: enc.encode("kafumu|" + box) }, k, raw.slice(12));
      }).then(function (pt) { return JSON.parse(dec.decode(pt)); });
    }

    // Box API. No cookies: the server must not link boxes to accounts.
    function post(box, body) {
      return fetchFn(base + "/api/box/" + box, { method: "POST", body: body, credentials: "omit" })
        .then(function (r) { if (!r.ok) throw new Error("box post " + r.status); });
    }
    function list(box) {
      return fetchFn(base + "/api/box/" + box, { credentials: "omit", cache: "no-store" })
        .then(function (r) { if (!r.ok) throw new Error("box get " + r.status); return r.json(); })
        .then(function (j) { return j.messages || []; });
    }
    function ack(box, ids) {
      if (!ids.length) return Promise.resolve();
      return fetchFn(base + "/api/box/" + box + "/ack", { method: "POST", body: JSON.stringify(ids), credentials: "omit" });
    }

    function inviteBox(pubRaw, kind) {
      return subtle.digest("SHA-256", concat(enc.encode("kafumu " + (kind || "invite") + " v1|"), pubRaw)).then(hex);
    }

    // invite returns the current invite (new one if none or expired). Its
    // private key stays in the device store, so late hellos still arrive
    // after the page is closed and reopened within the hour.
    // kind "move" is a share-with-self code: same keys, its own box and URL.
    // kind "badge" is a connect code meant for printing: same box and URL as
    // a normal invite, but it lives two weeks.
    var TTL = { invite: INVITE_TTL, badge: 14 * 864e5, move: INVITE_TTL };
    function invite(fresh, kind) {
      kind = kind || "invite";
      var key = kind === "invite" ? "invite" : "invite:" + kind;
      return store.get(key).then(function (inv) {
        if (!fresh && inv && Date.now() - inv.createdAt < TTL[kind]) return inv;
        return genKey().then(function (k) {
          return rawPub(k).then(function (pub) {
            return inviteBox(pub, kind === "badge" ? "invite" : kind).then(function (box) {
              var inv = { priv: k.privateKey, pub: b64(pub), box: box, createdAt: Date.now() };
              return store.set(key, inv).then(function () { return inv; });
            });
          });
        });
      }).then(function (inv) {
        inv.payload = "v1." + inv.pub;
        inv.url = base + (kind === "move" ? "/m#" : "/c#") + inv.payload;
        return inv;
      });
    }

    var CHUNK = 5000; // characters of JSON per message; ciphertext stays under the 8 KB box limit

    // moveSend runs on the old device: send obj (a backup) to the new
    // device that shows the move code, in encrypted chunks.
    function moveSend(payload, obj) {
      var m = /^v1\.([A-Za-z0-9_-]{80,100})$/.exec((payload || "").trim());
      if (!m) return Promise.reject(new Error("not a Kafumu code"));
      var aRaw = unb64(m[1]), text = JSON.stringify(obj), parts = [];
      for (var i = 0; i < text.length; i += CHUNK) parts.push(text.slice(i, i + CHUNK));
      if (parts.length > 30) return Promise.reject(new Error("too big to move by code; use a backup file"));
      return genKey().then(function (k) {
        return rawPub(k).then(function (bRaw) {
          return Promise.all([pairKey(k.privateKey, aRaw, aRaw, bRaw), inviteBox(aRaw, "move")]).then(function (r) {
            var key = r[0], box = r[1], pub = b64(bRaw);
            return parts.reduce(function (p, part, i) {
              return p.then(function () {
                return seal(key, box, { t: "move", i: i, n: parts.length, part: part })
                  .then(function (ct) { return post(box, JSON.stringify({ v: 1, pub: pub, ct: ct })); });
              });
            }, Promise.resolve()).then(function () { return parts.length; });
          });
        });
      });
    }

    // moveReceive runs on the new device: returns the received object once
    // every chunk from one sender has arrived, else null.
    function moveReceive() {
      return store.get("invite:move").then(function (inv) {
        if (!inv) return null;
        var aRaw = unb64(inv.pub);
        return list(inv.box).then(function (msgs) {
          var bySender = {}, ids = {};
          return msgs.reduce(function (p, msg) {
            return p.then(function () {
              var env = JSON.parse(msg.data), bRaw = unb64(env.pub);
              return pairKey(inv.priv, bRaw, aRaw, bRaw).then(function (key) { return open(key, inv.box, env.ct); })
                .then(function (body) {
                  if (body.t !== "move") return;
                  (bySender[env.pub] = bySender[env.pub] || { n: body.n, parts: {} }).parts[body.i] = body.part;
                  (ids[env.pub] = ids[env.pub] || []).push(msg.id);
                }).catch(function () {});
            });
          }, Promise.resolve()).then(function () {
            for (var pub in bySender) {
              var s = bySender[pub], got = Object.keys(s.parts).length;
              if (got < s.n) continue;
              var text = "";
              for (var i = 0; i < s.n; i++) text += s.parts[i];
              return ack(inv.box, ids[pub]).then(function () { return JSON.parse(text); });
            }
            return null;
          });
        });
      });
    }

    // accept is run by the scanner: send our card to the inviter.
    function accept(payload, myCard) {
      var m = /^v1\.([A-Za-z0-9_-]{80,100})$/.exec((payload || "").trim());
      if (!m) return Promise.reject(new Error("not a Kafumu code"));
      var aRaw = unb64(m[1]);
      return genKey().then(function (k) {
        return rawPub(k).then(function (bRaw) {
          return Promise.all([pairKey(k.privateKey, aRaw, aRaw, bRaw), inviteBox(aRaw)]).then(function (r) {
            var key = r[0], ibox = r[1];
            return seal(key, ibox, { t: "hello", card: myCard || {} }).then(function (ct) {
              return post(ibox, JSON.stringify({ v: 1, pub: b64(bRaw), ct: ct }));
            }).then(function () { return contactID(key); }).then(function (id) {
              var c = { id: id, key: b64(key), role: 1, card: null, note: "", createdAt: new Date().toISOString() };
              return store.putContact(c).then(function () { return c; });
            });
          });
        });
      });
    }

    // checkInvite is run by the inviter: turn hellos into contacts and answer
    // each with our card. Returns the new contacts.
    function checkInvite(myCard, kind) {
      var key = kind === "badge" ? "invite:badge" : "invite";
      return store.get(key).then(function (inv) {
        if (!inv || Date.now() - inv.createdAt > TTL[kind || "invite"] + 7 * 864e5) return [];
        var aRaw = unb64(inv.pub);
        return list(inv.box).then(function (msgs) {
          var done = [], added = [];
          return msgs.reduce(function (p, msg) {
            return p.then(function () {
              done.push(msg.id);
              var hello = JSON.parse(msg.data), bRaw = unb64(hello.pub);
              return pairKey(inv.priv, bRaw, aRaw, bRaw).then(function (key) {
                return open(key, inv.box, hello.ct).then(function (body) {
                  if (body.t !== "hello") return;
                  return contactID(key).then(function (id) {
                    var c = { id: id, key: b64(key), role: 0, card: body.card || {}, note: "", createdAt: new Date().toISOString() };
                    return boxOf(key, 1).then(function (theirs) {
                      return seal(key, theirs, { t: "card", card: myCard || {} }).then(function (ct) { return post(theirs, ct); });
                    }).then(function () { return store.putContact(c); }).then(function () { added.push(c); });
                  });
                });
              }).catch(function () { /* junk or not for us: drop it */ });
            });
          }, Promise.resolve()).then(function () { return ack(inv.box, done); }).then(function () { return added; });
        });
      });
    }

    // checkContact reads a contact's inbox for us; returns the messages and
    // applies card updates to the stored contact.
    function checkContact(c) {
      var key = unb64(c.key);
      return boxOf(key, c.role).then(function (mine) {
        return list(mine).then(function (msgs) {
          var done = [], got = [];
          return msgs.reduce(function (p, msg) {
            return p.then(function () {
              done.push(msg.id);
              return open(key, mine, msg.data).then(function (body) {
                got.push(body);
                if (body.t === "card") c.card = body.card || {};
                if (body.t === "signal") {
                  // Keep the last ten, newest first; "unread" until seen.
                  c.signals = [{ s: String(body.s || "").slice(0, 20), text: String(body.text || "").slice(0, 140), at: body.at || new Date().toISOString(), unread: true }]
                    .concat(c.signals || []).slice(0, 10);
                }
              })
                .catch(function () {});
            });
          }, Promise.resolve()).then(function () {
            return (got.length ? store.putContact(c) : Promise.resolve()).then(function () { return ack(mine, done); });
          }).then(function () { return got; });
        });
      });
    }

    // send delivers a message to a paired contact.
    function send(c, obj) {
      var key = unb64(c.key);
      return boxOf(key, 1 - c.role).then(function (theirs) {
        return seal(key, theirs, obj).then(function (ct) { return post(theirs, ct); });
      });
    }

    // ---- Friends around: day-level, cell-level, compared on the device ----
    function day(d) { return (d || new Date()).toISOString().slice(0, 10); }
    function beacon(key, role, cell, dayStr) {
      return hmacHex(key, "beacon|" + role + "|" + cell + "|" + dayStr).then(function (h) { return h.slice(0, 32); });
    }
    function slotID(key, role) { return hmacHex(key, "slot" + role); }

    // checkIn records that we were in cell today and, for every contact whose
    // slot doesn't reflect that yet, rewrites our slot for them: the tokens of
    // each (cell, day) of the last week. Returns the number of slots written.
    function checkIn(cell, contacts, now) {
      now = now || new Date();
      var today = day(now), weekAgo = day(new Date(now.getTime() - 7 * 864e5));
      return store.get("checkins").then(function (hist) {
        hist = (hist || []).filter(function (h) { return h.day > weekAgo; });
        if (!hist.some(function (h) { return h.cell === cell && h.day === today; })) hist.push({ cell: cell, day: today });
        hist = hist.slice(-40);
        var sig = hist.map(function (h) { return h.cell + h.day; }).join(",");
        return store.set("checkins", hist).then(function () {
          var due = contacts.filter(function (c) { return c.card && c.slotSig !== sig; });
          return Promise.all(due.map(function (c) {
            var key = unb64(c.key);
            return Promise.all(hist.map(function (h) { return beacon(key, c.role, h.cell, h.day); })).then(function (toks) {
              return slotID(key, c.role).then(function (id) {
                return fetchFn(base + "/api/slot/" + id, { method: "PUT", body: JSON.stringify(toks), credentials: "omit" });
              });
            }).then(function (r) {
              if (r.ok) { c.slotSig = sig; return store.putContact(c); }
            }).catch(function () {});
          })).then(function () { return due.length; });
        });
      });
    }

    // around reports which contacts were in or next to cells during the last
    // week, most recent first: [{contact, day, near}] (near: a neighbour
    // cell rather than cells[0]). One slot read per contact.
    function around(cells, contacts, now) {
      now = now || new Date();
      var days = [];
      for (var i = 0; i < 7; i++) days.push(day(new Date(now.getTime() - i * 864e5)));
      return Promise.all(contacts.filter(function (c) { return c.card; }).map(function (c) {
        var key = unb64(c.key), theirs = 1 - c.role;
        return slotID(key, theirs).then(function (id) {
          return fetchFn(base + "/api/slot/" + id, { credentials: "omit", cache: "no-store" });
        }).then(function (r) { return r.ok ? r.json() : { tokens: [] }; }).then(function (j) {
          var have = {};
          (j.tokens || []).forEach(function (t) { have[t] = true; });
          if (!(j.tokens || []).length) return null;
          var tries = [];
          days.forEach(function (d) { cells.forEach(function (cl, ci) { tries.push({ d: d, near: ci > 0, cl: cl }); }); });
          return Promise.all(tries.map(function (t) { return beacon(key, theirs, t.cl, t.d); })).then(function (toks) {
            for (var i = 0; i < toks.length; i++) if (have[toks[i]]) return { contact: c, day: tries[i].d, near: tries[i].near };
            return null;
          });
        }).catch(function () { return null; });
      })).then(function (rs) { return rs.filter(Boolean).sort(function (a, b) { return b.day.localeCompare(a.day); }); });
    }

    return { checkIn: checkIn, around: around, invite: invite, accept: accept, moveSend: moveSend, moveReceive: moveReceive, checkInvite: checkInvite, checkContact: checkContact, send: send,
      _open: open, _boxOf: boxOf, _inviteBox: inviteBox, _unb64: unb64 };
  }

  root.kafumuPair = { create: create };
})(typeof window !== "undefined" ? window : globalThis);
