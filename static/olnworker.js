// Mines an OLN v2 message in the background: finds a nonce so that the
// line "v2;<nonce>;<date>;<base64url>;<keywords>" has `bits` leading zero
// bits of Argon2id work (memory-hard: 4 MiB, 1 pass, 1 lane, 32 bytes,
// salt "OLN-v2-proofwork"). Posts progress so the page can show it.
importScripts("vendor/argon2-4.12.0.umd.min.js" + location.search); // same version as this worker
var SALT = new TextEncoder().encode("OLN-v2-proofwork");
function zeros(h) { for (var i = 0, n = 0; i < h.length; i++, n += 8) if (h[i]) return n + Math.clz32(h[i]) - 24; return n; }
onmessage = async function (e) {
  var d = e.data;
  // Either an OLN message (date, b64, keywords) or any ready-made tail
  // (the work stamp: ";<date>;<b64 sha256>;#box<id>").
  var tail = d.tail || (";" + d.date + ";" + d.b64 + ";" + d.keywords);
  var start = Math.floor(Math.random() * 1e9), t0 = Date.now();
  for (var i = 0; ; i++) {
    var raw = "v2;" + (start + i) + tail;
    var h = await self.hashwasm.argon2id({ password: raw, salt: SALT, parallelism: 1, iterations: 1, memorySize: 4096, hashLength: 32, outputType: "binary" });
    if (zeros(h) >= d.bits) { postMessage({ done: true, raw: raw, tries: i + 1, ms: Date.now() - t0 }); return; }
    if (i % 4 === 3) postMessage({ tries: i + 1, ms: Date.now() - t0 });
  }
};
