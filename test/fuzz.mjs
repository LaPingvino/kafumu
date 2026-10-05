// Fuzzes the pairing and chat protocols with people who come and go
// (Joop: "several linked accounts that are online and offline at different
// times"). Seeded, so a failure can be replayed:
//
//	node test/fuzz.mjs http://localhost:18081 [seed] [steps]
//
// Each step, a random person goes on- or offline, or (when online) shows a
// code, renews it, uses someone's code (current or an old one they kept),
// chats with a contact over encrypted OLN, or takes in what's waiting. At
// the end everyone comes online and catches up; then the invariants must
// hold: every connection exists on both sides with one shared key, every
// chat line arrived exactly once, nobody has duplicates.
import { readFileSync } from "node:fs";
new Function(readFileSync(new URL("../static/sha1.js", import.meta.url), "utf8"))();
new Function(readFileSync(new URL("../static/vendor/argon2-4.12.0.umd.min.js", import.meta.url), "utf8")).call(globalThis);
new Function(readFileSync(new URL("../static/pair.js", import.meta.url), "utf8"))();
const base = process.argv[2] || "http://localhost:18081";
const seed = parseInt(process.argv[3] || String(Date.now() % 100000), 10) || Date.now() % 100000;
const steps = parseInt(process.argv[4] || "120", 10);

// mulberry32: a small seeded PRNG.
let s = seed >>> 0;
const rnd = () => { s = (s + 0x6d2b79f5) >>> 0; let t = s; t = Math.imul(t ^ (t >>> 15), t | 1); t ^= t + Math.imul(t ^ (t >>> 7), t | 61); return ((t ^ (t >>> 14)) >>> 0) / 4294967296; };
const pick = (a) => a[Math.floor(rnd() * a.length)];

// OLN v2 miner for chat lines (what the browser worker does).
const salt = new TextEncoder().encode("OLN-v2-proofwork");
async function mine(text, keywords, bits, f = fetch) {
  const date = new Date().toISOString().replace(/[-:T]/g, "").slice(0, 14);
  const b64 = Buffer.from(text, "utf8").toString("base64url");
  for (let n = Math.floor(rnd() * 1e6); ; n++) {
    const line = `v2;${n};${date};${b64};${keywords}`;
    const h = await hashwasm.argon2id({ password: line, salt, parallelism: 1, iterations: 1, memorySize: 4096, hashLength: 32, outputType: "binary" });
    let z = 0; for (const b of h) { if (b === 0) { z += 8; continue; } z += Math.clz32(b) - 24; break; }
    if (z >= bits) {
      const r = await f(base + "/api/oln", { method: "POST", body: line });
      if (r.status === 402) return mine(text, keywords, (await r.json()).need || bits + 1, f);
      if (!r.ok) throw new Error("oln post " + r.status);
      return r.json();
    }
  }
}

function memStore() {
  const kv = new Map(), contacts = new Map();
  return {
    get: async (k) => kv.get(k), set: async (k, v) => { kv.set(k, v); },
    putContact: async (c) => { contacts.set(c.id, structuredClone({ ...c })); },
    deleteContact: async (id) => { contacts.delete(id); },
    contacts: async () => [...contacts.values()].map((c) => structuredClone(c)),
  };
}
const N = 6;
// netDown: this person's device has no network (requests fail like a real
// offline browser's), independent of whether they use the app ("online").
const people = Array.from({ length: N }, (_, i) => {
  const store = memStore();
  const p = { name: "P" + i, store, online: rnd() < 0.5, netDown: false, codes: [] };
  p.fetch = (url, o) => p.netDown ? Promise.reject(new TypeError("Failed to fetch")) : fetch(url, o);
  p.mine = (t, kw, b) => mine(t, kw, b, p.fetch);
  p.pair = globalThis.kafumuPair.create({ fetch: p.fetch, store, origin: base, crypto: globalThis.crypto, mine: p.mine });
  return p;
});
const sentChat = [];  // {from, to, text}
const hellos = [];    // {from, to}: someone used someone's code
const log = [];
let offlineActs = 0;  // connects and chat lines made without a network (outbox)

async function takeIn(p) {
  const card = { name: p.name };
  const added = await p.pair.checkInvite(card);
  for (const c of await p.store.contacts()) {
    await p.pair.checkContact(c);
    const fresh = (await p.store.contacts()).find((x) => x.id === c.id);
    if (fresh) await p.pair.readChat(fresh);
  }
  return added.length;
}

for (let i = 0; i < steps; i++) {
  const p = pick(people), r = rnd();
  if (r < 0.15) { p.online = !p.online; log.push(`${p.name} ${p.online ? "online" : "offline"}`); continue; }
  if (r < 0.22) { p.netDown = !p.netDown; log.push(`${p.name} network ${p.netDown ? "lost" : "back"}`); if (!p.netDown) await p.pair.flush(); continue; }
  if (!p.online) continue;
  try {
    if (r < 0.3) {                                   // show a code (maybe a fresh one)
      const inv = await p.pair.invite(rnd() < 0.3);
      if (!p.codes.includes(inv.payload)) p.codes.push(inv.payload);
      log.push(`${p.name} shows a code`);
    } else if (r < 0.5) {                            // use someone's code (any they ever showed)
      const q = pick(people.filter((x) => x !== p && x.codes.length));
      if (!q) continue;
      // (Both may still use each other's code before either takes the other's
      // hello in: two connections between the same two people. That happens
      // in real life too; the checks below accept it, as long as each holds.)
      const already = (await p.store.contacts()).some((c) => c.card && c.card.name === q.name) || hellos.some((h) => h.from === p.name && h.to === q.name);
      if (already) continue;                         // one connection per pair keeps the check simple
      await p.pair.accept(pick(q.codes), { name: p.name });
      hellos.push({ from: p.name, to: q.name });
      if (p.netDown) offlineActs++;
      log.push(`${p.name} uses ${q.name}'s code`);
    } else if (r < 0.75) {                           // chat with a contact
      const cs = (await p.store.contacts()).filter((c) => c.card && c.card.name);
      if (!cs.length) continue;
      const c = pick(cs), text = `${p.name}→${c.card.name} #${i}`;
      await p.pair.sendChat(c, text, p.mine);
      sentChat.push({ from: p.name, to: c.card.name, text, key: c.key });
      if (p.netDown) offlineActs++;
      log.push(`${p.name} chats ${c.card.name}`);
    } else {                                         // take in what's waiting (needs a network)
      if (p.netDown) { log.push(`${p.name} can't take in (no network)`); continue; }
      await p.pair.flush();
      const n = await takeIn(p);
      log.push(`${p.name} takes in (${n} new)`);
    }
  } catch (e) {
    console.error(`FAIL seed ${seed} step ${i}: ${e.message}\n` + log.slice(-12).join("\n"));
    process.exit(1);
  }
}

// Everyone's network comes back, outboxes flush, everyone catches up
// (three rounds: hellos, cards and chat cross over).
for (const p of people) { p.netDown = false; await p.pair.flush(); }
for (let round = 0; round < 3; round++) for (const p of people) await takeIn(p);
for (const p of people) {
  const left = await p.pair.outboxSize();
  if (left) { console.error(`FAIL seed ${seed}: ${p.name}'s outbox still holds ${left}`); process.exit(1); }
}

const fail = [];
const byName = Object.fromEntries(people.map((p) => [p.name, p]));
// One person, one contact (slice 66): each pair of people that connected
// has exactly one contact on each side, holding the same set of connection
// keys (mutual scans fold into one); every chat line arrived exactly once.
const keysOf = (c) => [c.key].concat((c.altKeys || []).map((k) => k.key)).sort().join(",");
let mutual = 0;
const pairs = new Set(hellos.map((h) => [h.from, h.to].sort().join("|")));
for (const pr of pairs) {
  const [x, y] = pr.split("|");
  if (hellos.filter((h) => [h.from, h.to].sort().join("|") === pr).length > 1) mutual++;
  const cx = (await byName[x].store.contacts()).filter((c) => c.card && c.card.name === y);
  const cy = (await byName[y].store.contacts()).filter((c) => c.card && c.card.name === x);
  if (cx.length !== 1 || cy.length !== 1) fail.push(`${x}↔${y}: ${cx.length} / ${cy.length} contacts (want 1 / 1)`);
  else if (keysOf(cx[0]) !== keysOf(cy[0])) fail.push(`${x}↔${y}: connection keys differ`);
}
for (const m of sentChat) {
  const c = (await byName[m.to].store.contacts()).find((x) => x.card && x.card.name === m.from);
  const n = c ? (c.messages || []).filter((x) => !x.me && x.text === m.text).length : 0;
  if (n !== 1) fail.push(`chat "${m.text}" arrived ${n} times`);
}
if (fail.length) {
  console.error(`FAIL fuzz seed ${seed} (${steps} steps):\n  ` + fail.slice(0, 10).join("\n  ") + "\n--- last steps:\n" + log.slice(-15).join("\n"));
  process.exit(1);
}
console.log(`ok  fuzz: seed ${seed}, ${steps} steps, ${hellos.length} connections (${mutual} mutual), ${sentChat.length} chat lines, ${N} people coming and going, ${offlineActs} done offline (outbox)`);
