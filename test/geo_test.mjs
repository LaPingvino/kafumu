// Cross-checks static/kafumu.js against internal/geo test vectors: node test/geo_test.mjs
import { readFileSync } from "node:fs";
const src = readFileSync(new URL("../static/kafumu.js", import.meta.url), "utf8");
const el = () => ({ dataset: {}, addEventListener() {}, style: {} });
globalThis.document = { getElementById: el, querySelector: el };
globalThis.window = globalThis;
globalThis.localStorage = { getItem() { return null; }, setItem() {} };
globalThis.history = { replaceState() {} };
new Function(src)();
const { cell, rings, parsePlace } = globalThis.kafumu;
const cases = [[52.3676, 4.9041, "9f469w"], [52.3731, 4.8926, "9f469v"], [-23.5505, -46.6333, "588mc9"],
  [0, 0, "6fg222"], [89.99999, 179.99999, "cvxxxx"], [-90, -180, "222222"]];
let fail = 0;
for (const [la, lo, want] of cases) {
  const got = cell(la, lo);
  if (got !== want) { console.error(`cell(${la},${lo}) = ${got}, want ${want}`); fail++; }
}
const r = rings("9f469w", 2);
if (r.length !== 25 || r[0][0] !== "9f469w" || r.filter(p => p[1] === 1).length !== 8) { console.error("rings", r); fail++; }
for (const [s, want] of [["#geo9F469V", "9f469v"], ["9F469VXJ+2C", "9f469v"], ["52.3731, 4.8926", "9f469v"], ["nonsense", null]]) {
  if (parsePlace(s) !== want) { console.error(`parsePlace(${s}) = ${parsePlace(s)}`); fail++; }
}
if (fail) process.exit(1);
console.log("ok  static/kafumu.js geo");
