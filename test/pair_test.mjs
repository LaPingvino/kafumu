// End-to-end pairing test: A shows a code, B and C scan it, everyone ends up
// with the right cards, and C cannot read B's hello. Runs against a local
// server: PORT=18081 go run . & node test/pair_test.mjs http://localhost:18081
import { readFileSync } from "node:fs";
new Function(readFileSync(new URL("../static/pair.js", import.meta.url), "utf8"))();
const base = process.argv[2] || "http://localhost:18081";

function memStore() {
  const kv = new Map(), contacts = new Map();
  return {
    get: async (k) => kv.get(k), set: async (k, v) => { kv.set(k, v); },
    putContact: async (c) => { contacts.set(c.id, structuredClone({ ...c })); },
    contacts: async () => [...contacts.values()],
  };
}
const device = (name) => {
  const store = memStore();
  return { name, store, pair: globalThis.kafumuPair.create({ fetch, store, origin: base, crypto: globalThis.crypto }) };
};
let fail = 0;
const check = (ok, msg) => { if (!ok) { console.error("FAIL", msg); fail++; } };

const A = device("A"), B = device("B"), C = device("C");
const inv = await A.pair.invite();
check(inv.url.startsWith(base + "/c#v1."), "invite url " + inv.url);
check((await A.pair.invite()).payload === inv.payload, "invite is reused within the hour");

const cb = await B.pair.accept(inv.payload, { name: "Bea", about: "robots" });
const cc = await C.pair.accept(inv.payload, { name: "Cai" });
check(cb.card === null && cb.role === 1, "B waits for A's card");

// C reads the invite box (anyone with the QR can) but can't open B's hello.
const raw = await (await fetch(`${base}/api/box/${inv.box}`)).json();
check(raw.messages.length === 2, "two hellos waiting, got " + raw.messages.length);
const helloB = JSON.parse(raw.messages[0].data);
let leaked = false;
try { await C.pair._open(C.pair._unb64(cc.key), inv.box, helloB.ct); leaked = true; } catch {}
check(!leaked, "C must not decrypt B's hello");
check(!raw.messages.some(m => m.data.includes("Bea")), "card travels encrypted");

const added = await A.pair.checkInvite({ name: "Ana", whatsapp: "+351900000000" });
check(added.length === 2, "A got two contacts, got " + added.length);
const names = added.map(c => c.card.name).sort().join(",");
check(names === "Bea,Cai", "A has Bea and Cai, got " + names);
check((await (await fetch(`${base}/api/box/${inv.box}`)).json()).messages.length === 0, "hellos acked");

await B.pair.checkContact(cb);
await C.pair.checkContact(cc);
const bView = (await B.store.contacts())[0], cView = (await C.store.contacts())[0];
check(bView.card && bView.card.name === "Ana" && bView.card.whatsapp === "+351900000000", "B has Ana's card");
check(cView.card && cView.card.name === "Ana", "C has Ana's card");
const aB = added.find(c => c.card.name === "Bea");
check(aB.id === cb.id && aB.key === cb.key, "A and B agree on the pair key and id");
check(cb.key !== cc.key, "B and C have different pair keys");

// A message from B to A arrives in A's inbox only, and can't be reflected.
await B.pair.send(cb, { t: "signal", s: "coffee?" });
const got = await A.pair.checkContact(aB);
check(got.length === 1 && got[0].s === "coffee?", "A got B's signal");
check((await B.pair.checkContact(cb)).length === 0, "B's own message is not in B's inbox");

if (fail) process.exit(1);
console.log("ok  static/pair.js handshake (A↔B, A↔C, isolation, signal)");
