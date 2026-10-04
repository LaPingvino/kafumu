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
// New fields and your own ones (Joop): handles, Mastodon, links/emails/text.
{
  const more = links({ instagram: "@joop", facebook: "facebook.com/joop.k", mastodon: "@joop@mastodon.social", tiktok: "joop", youtube: "@joopk",
    custom: [{ label: "Blog", value: "www.example.org" }, { label: "Work mail", value: "j@example.org" }, { label: "Shoe size", value: "44" }, { label: "Evil", value: "javascript:alert(1)" }] });
  const by = (f, i = 0) => more.filter(l => l.field === f)[i];
  const exp = [["instagram", "https://instagram.com/joop"], ["facebook", "https://facebook.com/joop.k"], ["mastodon", "https://mastodon.social/@joop"],
    ["tiktok", "https://www.tiktok.com/@joop"], ["youtube", "https://www.youtube.com/@joopk"]];
  for (const [f, href] of exp) if (!by(f) || by(f).href !== href) { console.error(f, by(f) && by(f).href, "want", href); fail++; }
  const customs = more.filter(l => l.field === "custom");
  if (customs.length !== 2 || customs[0].href !== "https://www.example.org/" || customs[1].href !== "mailto:j@example.org" || customs[0].label !== "Blog") { console.error("custom links", JSON.stringify(customs)); fail++; }
  for (const l of more) if (/^javascript:/i.test(l.href)) { console.error("javascript: link", l.href); fail++; }
}
if (fail) process.exit(1);
console.log("ok  static/device.js links (+ instagram, facebook, mastodon, tiktok, youtube, your own fields)");

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

// Personas: share hands over the name plus only the ticked fields.
{
  const { personas } = globalThis.kafumuDevice;
  const p = { card: { name: "Ana", email: "a@b.pt", phone: "+351", linkedin: "ana", tags: ["AI", "Esperanto"] } };
  const all = personas.share(p, null);
  const some = personas.share(p, ["email"]);
  const bad = [];
  if (!(all.email && all.phone && all.linkedin && all.tags.length === 2)) bad.push("share all: " + JSON.stringify(all));
  if (JSON.stringify(some) !== JSON.stringify({ name: "Ana", email: "a@b.pt" })) bad.push("share some: " + JSON.stringify(some));
  if (bad.length) { console.error(bad.join("\n")); process.exit(1); }
  console.log("ok  static/device.js personas.share");
}
