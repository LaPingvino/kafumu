// Checks the contact-link normalisation in static/device.js: node test/device_test.mjs
import { readFileSync } from "node:fs";
globalThis.window = globalThis;
new Function(readFileSync(new URL("../static/device.js", import.meta.url), "utf8"))();
const { links } = globalThis.kafumuDevice;
const got = Object.fromEntries(links({
  email: "a@b.pt", phone: "+351 912 345 678", whatsapp: "00351 912 345 678", signal: "+351912345678",
  telegram: "@joop", bluesky: "@joop.bsky.social", linkedin: "joopkiefte", website: "javascript:alert(1)",
}).map(l => [l.field, l.href]));
const want = {
  email: "mailto:a%40b.pt", phone: "tel:+351912345678", whatsapp: "https://wa.me/351912345678",
  signal: "https://signal.me/#p/+351912345678", telegram: "https://t.me/joop",
  bluesky: "https://bsky.app/profile/joop.bsky.social", linkedin: "https://www.linkedin.com/in/joopkiefte",
};
let fail = 0;
for (const [k, v] of Object.entries(want)) if (got[k] !== v) { console.error(k, got[k], "want", v); fail++; }
if (got.website && !got.website.startsWith("https://")) { console.error("unsafe website", got.website); fail++; }
for (const href of Object.values(got)) if (/^javascript:/i.test(href)) { console.error("javascript: link", href); fail++; }
if (fail) process.exit(1);
console.log("ok  static/device.js links");

// vCard export escapes and skips contacts without a card.
{
  const v = globalThis.kafumuDevice.vcards([
    { card: { name: "Ana; Silva", email: "a@b.pt", about: "open source, coffee" }, note: "met at\nWS", createdAt: "2026-11-10T10:00:00Z" },
    { card: null },
  ]);
  const want = ["BEGIN:VCARD", "FN:Ana\\; Silva", "EMAIL:a@b.pt", "NOTE:open source\\, coffee — met at\\nWS — Kafumu 2026-11-10", "END:VCARD"];
  for (const w of want) if (!v.includes(w)) { console.error("vcard missing", JSON.stringify(w), "in", JSON.stringify(v)); process.exit(1); }
  if (v.split("BEGIN:VCARD").length !== 2) { console.error("pending contact exported"); process.exit(1); }
  console.log("ok  static/device.js vcards");
}
