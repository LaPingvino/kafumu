// static/board.js (97b): a group's board posts seal and open with the group
// key, a wrong key can't read them, the tag is a pair-style tag, and reading
// skips what doesn't open.
import { readFileSync } from "node:fs";
const root = {};
new Function("window", readFileSync(new URL("../static/board.js", import.meta.url), "utf8"))(root);
const B = root.kafumuBoard;
const fail = (m) => { console.error("FAIL board:", m); process.exit(1); };
const key = Buffer.from(crypto.getRandomValues(new Uint8Array(32))).toString("base64url");
const other = Buffer.from(crypto.getRandomValues(new Uint8Array(32))).toString("base64url");

const t = await B.tag(key);
if (!/^p[0-9a-f]{32}$/.test(t) || t !== await B.tag(key) || t === await B.tag(other)) fail("tag " + t);
const sealed = await B.seal(key, { kind: "offer", text: "Bike repair", price: 5000000 });
const back = await B.open(key, sealed);
if (back.text !== "Bike repair" || back.price !== 5000000) fail("round trip " + JSON.stringify(back));
if (sealed.includes("Bike")) fail("plain text in the ciphertext");
let opened = true;
try { await B.open(other, sealed); } catch { opened = false; }
if (opened) fail("a wrong key opened a post");

// read: the server's list, mixed with a post under another key and garbage.
const posts = [
  { id: "a", text: sealed, at: "2026-10-15T10:00:00Z" },
  { id: "b", text: await B.seal(other, { kind: "note", text: "not ours" }), at: "2026-10-15T11:00:00Z" },
  { id: "c", text: "garbage", at: "2026-10-15T12:00:00Z" },
  { id: "d", text: await B.seal(key, { kind: "request", text: "Need a ladder", at: "2026-10-15T13:00:00Z" }), at: "2026-10-15T13:00:00Z" },
];
let asked = "";
globalThis.fetch = async (u, opts) => { asked = u; if (opts.credentials !== "omit") fail("read with cookies"); return { ok: true, json: async () => posts }; };
const got = await B.read(key);
if (asked !== "/api/oln/pair/" + t) fail("fetched " + asked);
if (got.length !== 2 || got[0].text !== "Need a ladder" || got[1].text !== "Bike repair") fail("read " + JSON.stringify(got));

// post: what goes to the miner is ciphertext under the tag.
let mined;
const p = await B.post(key, { kind: "offer", text: "Bread", price: 1 }, async (text, kw, bits) => { mined = { text, kw, bits }; return { id: "x" }; });
if (mined.kw !== "#" + t || mined.text.includes("Bread") || p.id !== "x") fail("post " + JSON.stringify(mined));
console.log("ok  static/board.js (seal/open, wrong key, tag, read skips the unreadable, post sends ciphertext)");
