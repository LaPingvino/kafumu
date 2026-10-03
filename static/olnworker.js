// Mines an OLN message in the background: finds a nonce so that
// SHA-1("<nonce>;<date>;<base64url>;<keywords>") has `bits` leading zero
// bits — eolnpoc's format. Posts progress so the page can show it.
importScripts("sha1.js" + location.search); // same version as this worker
onmessage = function (e) {
  var d = e.data, enc = new TextEncoder(), H = self.kafumuSHA1;
  // Either an OLN message (date, b64, keywords) or any ready-made tail
  // (the mailbox work stamp: ";<date>;<b64 sha256>;#box<id>").
  var tail = d.tail || (";" + d.date + ";" + d.b64 + ";" + d.keywords);
  var start = Math.floor(Math.random() * 1e9), t0 = Date.now();
  for (var i = 0; ; i++) {
    var raw = (start + i) + tail;
    if (H.leadingZeros(H.sha1(enc.encode(raw))) >= d.bits) {
      postMessage({ done: true, raw: raw, tries: i + 1, ms: Date.now() - t0 });
      return;
    }
    if (i % 20000 === 0 && i) postMessage({ tries: i, ms: Date.now() - t0 });
  }
};
