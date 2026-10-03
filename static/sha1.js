// Small synchronous SHA-1 for proof of work (hashcash-style, as in
// github.com/LaPingvino/eolnpoc). WebCrypto's digest is async per call,
// far too slow for mining; this runs in a Web Worker instead.
(function (root) {
  "use strict";
  // sha1 of a byte array; returns the five 32-bit words.
  function sha1(bytes) {
    var len = bytes.length, words = ((len + 8) >> 6) + 1, w = new Int32Array(words * 16), i;
    for (i = 0; i < len; i++) w[i >> 2] |= bytes[i] << (24 - (i & 3) * 8);
    w[len >> 2] |= 0x80 << (24 - (len & 3) * 8);
    w[words * 16 - 1] = len * 8;
    var h0 = 0x67452301, h1 = 0xefcdab89, h2 = 0x98badcfe, h3 = 0x10325476, h4 = 0xc3d2e1f0;
    var x = new Int32Array(80);
    for (var b = 0; b < w.length; b += 16) {
      for (i = 0; i < 16; i++) x[i] = w[b + i];
      for (i = 16; i < 80; i++) { var t = x[i - 3] ^ x[i - 8] ^ x[i - 14] ^ x[i - 16]; x[i] = (t << 1) | (t >>> 31); }
      var a = h0, bb = h1, c = h2, d = h3, e = h4, f, k, tmp;
      for (i = 0; i < 80; i++) {
        if (i < 20) { f = (bb & c) | (~bb & d); k = 0x5a827999; }
        else if (i < 40) { f = bb ^ c ^ d; k = 0x6ed9eba1; }
        else if (i < 60) { f = (bb & c) | (bb & d) | (c & d); k = 0x8f1bbcdc; }
        else { f = bb ^ c ^ d; k = 0xca62c1d6; }
        tmp = (((a << 5) | (a >>> 27)) + f + e + k + x[i]) | 0;
        e = d; d = c; c = (bb << 30) | (bb >>> 2); bb = a; a = tmp;
      }
      h0 = (h0 + a) | 0; h1 = (h1 + bb) | 0; h2 = (h2 + c) | 0; h3 = (h3 + d) | 0; h4 = (h4 + e) | 0;
    }
    return [h0, h1, h2, h3, h4];
  }
  // leadingZeros counts the leading zero bits of a SHA-1 result.
  function leadingZeros(h) {
    for (var i = 0, n = 0; i < 5; i++, n += 32) if (h[i] !== 0) return n + Math.clz32(h[i]);
    return 160;
  }
  root.kafumuSHA1 = { sha1: sha1, leadingZeros: leadingZeros };
})(typeof self !== "undefined" ? self : globalThis);
