// End-to-end pairing test: A shows a code, B and C scan it, everyone ends up
// with the right cards, and C cannot read B's hello. Runs against a local
// server: PORT=18081 go run . & node test/pair_test.mjs http://localhost:18081
import { readFileSync } from "node:fs";
new Function(readFileSync(new URL("../static/sha1.js", import.meta.url), "utf8"))();
new Function(readFileSync(new URL("../static/vendor/argon2-4.12.0.umd.min.js", import.meta.url), "utf8")).call(globalThis);
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

// Friends around: B checks in at the venue today; A, one cell away, sees B.
{
  const venue = "8ccgqw", next = "8ccgqx";
  const bc = (await B.store.contacts())[0], ac = (await A.store.contacts()).find(c => c.card.name === "Bea");
  check(await B.pair.checkIn(venue, [bc]) === 1, "B wrote one slot");
  check(await B.pair.checkIn(venue, [bc]) === 0, "same cell-day: no rewrite");
  const hits = await A.pair.around([next, venue], [ac]);
  check(hits.length === 1 && hits[0].contact.card.name === "Bea" && hits[0].near, "A sees Bea nearby: " + JSON.stringify(hits.map(h => h.day)));
  check(hits[0].cell === venue, "the hit says where (for Nearest in Contacts)");
  const far = await A.pair.around(["9f469v"], [ac]);
  check(far.length === 0, "not seen from Amsterdam");
  const raw = await (await fetch(`${base}/api/slot/${"0".repeat(64)}`)).json();
  check(Array.isArray(raw.tokens), "slot api answers");
}

// An old link still works after A made a new code (Joop lost two contacts
// this way): the replaced code's key is kept and checked too.
{
  const G = device("G"), H = device("H");
  const old = await G.pair.invite();
  await G.pair.invite(true); // "New code" (or an hour passed and Connect opened)
  await H.pair.accept(old.payload, { name: "Hal" });
  const got = await G.pair.checkInvite({ name: "Gus" });
  check(got.length === 1 && got[0].card.name === "Hal", "hello on a replaced code still arrives, got " + got.length);
}

// Badge: a long-lived printed code works like a normal one.
{
  const badge = await A.pair.invite(false, "badge");
  check(badge.url.includes("/c#v1.") && badge.payload !== (await A.pair.invite()).payload, "badge is its own code");
  const D = device("D");
  await D.pair.accept(badge.payload, { name: "Dai" });
  const got = await A.pair.checkInvite({ name: "Ana" }, "badge");
  check(got.length === 1 && got[0].card.name === "Dai", "badge scan arrives");
}

// Public inbox: E opens one; F (a stranger) writes with a card at E's
// price; E reads it, connects back, and F receives E's card.
{
  const E = device("E"), F = device("F");
  const ib = await E.pair.inbox();
  check(/^[0-9a-f]{64}$/.test(ib.box), "inbox box id");
  const pending = await F.pair.writeTo({ box: ib.box, pub: ib.pub, bits: 2 }, "Hi, saw you're into Esperanto!", { name: "Fay" });
  check(pending && pending.card === null, "F waits for E's card");
  const msgs = await E.pair.readInbox();
  check(msgs.length === 1 && msgs[0].text.includes("Esperanto") && msgs[0].card.name === "Fay", "E reads F's message");
  const c = await E.pair.connectBack(msgs[0], { name: "Eve" });
  check(c.card.name === "Fay", "E has Fay as a contact");
  await F.pair.checkContact(pending);
  check((await F.store.contacts())[0].card.name === "Eve", "F gets Eve's card");
}

// Share with self: the new device shows a move code, the old one sends a
// backup big enough to need several chunks.
const N = device("new"), O = device("old");
const mv = await N.pair.invite(false, "move");
check(mv.url.includes("/m#v1."), "move url " + mv.url);
check(mv.box !== (await N.pair.invite()).box, "move and invite boxes differ");
const backup = { kafumu: 1, contacts: Array.from({ length: 40 }, (_, i) => ({ id: "c" + i, key: "k".repeat(43), card: { name: "Person " + i, about: "x".repeat(200) } })) };
const chunks = await O.pair.moveSend(mv.payload, backup);
check(chunks > 1, "backup split into chunks, got " + chunks);
const got2 = await N.pair.moveReceive();
check(got2 && got2.contacts.length === 40 && got2.contacts[39].card.name === "Person 39", "new device got the whole backup");
check((await N.pair.moveReceive()) === null, "move chunks acked");

if (fail) process.exit(1);
// Private answers to an anonymous post (slice 69): A posts with a reply
// key, B answers privately, A reads it and answers back on the same thread.
{
  const salt = new TextEncoder().encode("OLN-v2-proofwork");
  const mineOLN = async (text, keywords, bits) => {
    const date = new Date().toISOString().replace(/[-:T]/g, "").slice(0, 14), b64t = Buffer.from(text, "utf8").toString("base64url");
    for (let n = 0; ; n++) {
      const line = `v2;${n};${date};${b64t};${keywords}`;
      const h = await hashwasm.argon2id({ password: line, salt, parallelism: 1, iterations: 1, memorySize: 4096, hashLength: 32, outputType: "binary" });
      let z = 0; for (const b of h) { if (b === 0) { z += 8; continue; } z += Math.clz32(b) - 24; break; }
      if (z < bits) continue;
      const r = await fetch(base + "/api/oln", { method: "POST", body: line });
      if (r.status === 402) return mineOLN(text, keywords, (await r.json()).need);
      if (!r.ok) throw new Error("oln " + r.status + " " + await r.text());
      return r.json();
    }
  };
  const P = device("Poster"), Q = device("Answerer");
  const text = "Anyone up for a run along the river at 7? " + Date.now();
  const kw = await P.pair.replyKey(text, 36e5);
  check(/^#rka[0-9a-f]{33} #rkb[0-9a-f]{33}$/.test(kw), "reply key keywords " + kw);
  const note = await mineOLN(text, "#geo6fg222 " + kw, 4);
  check(Q.pair.replyKeyOf(note.tags) !== null, "the post carries its reply key");
  const t = await Q.pair.answer(note, "Yes! Meet at the bridge? 🙂", mineOLN);
  check(t.answer && t.role === 1, "answerer keeps a thread");
  check(await P.pair.readAnswers() === 1, "poster takes in one answer");
  const [pt] = await P.pair.threads();
  check(pt && pt.messages[0].text === "Yes! Meet at the bridge? 🙂" && pt.post.text === text, "poster reads the answer next to the post");
  check(await P.pair.readAnswers() === 0, "no answer twice");
  await P.pair.sendChat(pt, "See you there.", mineOLN);
  check(await Q.pair.readAnswers() === 1, "answerer gets the reply");
  const [qt] = await Q.pair.threads();
  check(qt.messages.some((m) => !m.me && m.text === "See you there."), "reply on the same thread");
  check((await P.store.contacts()).length === 0 && (await Q.store.contacts()).length === 0, "answers are not contacts");
  let refused = false;
  try { await Q.pair.answer({ id: "x", text: "no key", tags: ["geo6fg222"] }, "hi", mineOLN); } catch { refused = true; }
  check(refused, "a post without a reply key takes no private answers");
}

console.log("ok  static/pair.js handshake (A↔B, A↔C, isolation, signal) + private answers to an anonymous post + friends around + replaced codes + badge + public inbox + move to new device");
