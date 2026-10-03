// Local messages in Joop's Open Location Network format: no account,
// paid for with proof of work, mined in a Web Worker.
(function (root) {
  "use strict";
  function b64url(text) {
    var bytes = new TextEncoder().encode(text), s = "";
    bytes.forEach(function (b) { s += String.fromCharCode(b); });
    return btoa(s).replace(/\+/g, "-").replace(/\//g, "_"); // padded, like Go's URLEncoding
  }
  // Versioned like every asset, or a cached old worker answers new requests.
  // (read at use: the version is set at the end of the page)
  function workerURL() { return "/static/olnworker.js?v=" + encodeURIComponent(root.KAFUMU_V || ""); }
  function utcStamp(d) { return d.toISOString().replace(/[-:T]/g, "").slice(0, 14); }

  // mine resolves with the raw message; onProgress(tries, ms) while working.
  function mine(text, keywords, bits, onProgress) {
    return new Promise(function (resolve, reject) {
      var w = new Worker(workerURL());
      w.onmessage = function (e) {
        if (e.data.done) { w.terminate(); resolve(e.data); } else if (onProgress) onProgress(e.data.tries, e.data.ms);
      };
      w.onerror = function (e) { w.terminate(); reject(e); };
      w.postMessage({ date: utcStamp(new Date()), b64: b64url(text), keywords: keywords, bits: bits });
    });
  }

  // post mines at `bits` and sends; when the area got busier meanwhile
  // (402), it mines again with one more bit, up to twice.
  function post(text, keywords, bits, onProgress, tries) {
    tries = tries || 0;
    return mine(text, keywords, bits, onProgress).then(function (m) {
      return fetch("/api/oln", { method: "POST", body: m.raw, credentials: "omit" }).then(function (r) {
        if (r.status === 402 && tries < 2) return post(text, keywords, bits + 1, onProgress, tries + 1);
        return r.json().then(function (j) { if (!r.ok) throw new Error(j.error || r.status); return j; });
      });
    });
  }

  // mineTail mines any "<nonce><tail>" in the worker (for priced inboxes).
  function mineTail(tail, bits, onProgress) {
    return new Promise(function (resolve, reject) {
      var w = new Worker(workerURL());
      w.onmessage = function (e) { if (e.data.done) { w.terminate(); resolve(e.data.raw); } else if (onProgress) onProgress(e.data.tries, e.data.ms); };
      w.onerror = function (e) { w.terminate(); reject(e); };
      w.postMessage({ tail: tail, bits: bits });
    });
  }

  root.kafumuOLN = { mineTail: mineTail, mine: mine, post: post, b64url: b64url, utcStamp: utcStamp };
})(window);
