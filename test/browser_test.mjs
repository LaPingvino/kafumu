// Two-browser test of the connect pages: A shows a code, B opens it, both
// end up connected. Drives headless Chromium over the DevTools protocol (no
// dependencies). Needs a running server and chromium on PATH:
//   PORT=18082 ./kafumu & node test/browser_test.mjs http://localhost:18082
import { browser, sleep } from "./cdp.mjs";

const base = process.argv[2] || "http://localhost:18082";
const fill = (form, values) => `(() => { const f = document.getElementById(${JSON.stringify(form)});
  ${Object.entries(values).map(([k, v]) => `f.elements[${JSON.stringify(k)}].value = ${JSON.stringify(v)};`).join("")}
  f.requestSubmit(); return true; })()`;

const A = await browser(9333), B = await browser(9334);
try {
  await A.goto(base + "/connect");
  await A.waitFor("!document.getElementById('name-form').hidden", "A's name form");
  await A.evaluate(fill("name-form", { name: "Ana", about: "open source" }));
  await A.waitFor("!!document.querySelector('#qr svg')", "A's QR code");
  // Untick "about" for this share: B must not receive it.
  await A.waitFor("document.querySelectorAll('#share-picker .chip').length > 0", "A's share picker");
  await A.evaluate("[...document.querySelectorAll('#share-picker .chip')].find(b => b.textContent.includes('open source')).click()");
  await A.waitFor("[...document.querySelectorAll('#share-picker .chip')].some(b => b.getAttribute('aria-pressed') === 'false')", "about unticked");
  const url = await A.evaluate("document.getElementById('invite-link').value");
  if (!/\/c#v1\./.test(url)) throw new Error("bad invite url " + url);

  await B.goto(url);
  await B.waitFor("!document.getElementById('name-form').hidden", "B's name form");
  await B.evaluate(fill("name-form", { name: "Bea" }));
  await B.waitFor("!document.getElementById('accept-area').hidden", "B's connect button");
  await B.evaluate("document.getElementById('do-connect').click()");
  await B.waitFor("document.getElementById('accept-status').textContent.includes('Ana')", "B connected with Ana");
  await A.waitFor("document.getElementById('new-contacts').textContent.includes('Bea')", "A sees Bea");
  if (await B.evaluate("location.hash") !== "") throw new Error("code left in B's URL");
  await A.goto(base + "/contacts");
  await A.waitFor("document.getElementById('contacts').textContent.includes('Bea')", "Bea on A's contacts page");
  await B.goto(base + "/contacts");
  await B.waitFor("document.getElementById('contacts').textContent.includes('Ana')", "Ana on B's contacts page");
  if (await B.evaluate("document.getElementById('contacts').textContent.includes('open source')")) throw new Error("unticked field was shared");
  // Signals: B taps "Coffee?", A sees it in Around.
  await B.waitFor("!!document.querySelector('.signals button')", "B's signal buttons");
  await B.evaluate("document.querySelector('.signals button').click()");
  await B.waitFor("document.querySelector('.signals button').textContent.startsWith('✓')", "signal sent");
  await A.goto(base + "/?cell=8ccgqw");
  await A.waitFor("!document.getElementById('signals-section').hidden && document.getElementById('signals').textContent.includes('Bea')", "B's signal in A's Around");
  // Meetups: A makes an account on the way to hosting, B sees it in Around.
  await A.goto(base + "/meetups/new");
  await A.evaluate("document.querySelector('form[action=\"/account/start\"]').requestSubmit()");
  await A.waitFor("!!document.querySelector('a[href=\"/meetups/new\"].suggested')", "continue link after account");
  await A.goto(base + "/meetups/new");
  await A.waitFor("!!document.getElementById('meetup-form')", "meetup form");
  await A.evaluate("(() => { const f = document.getElementById('meetup-form'); f.title.value = 'Browser test kafo'; f.cell.value = '8ccgqw'; f.venue.value = 'Pavilion 2'; f.requestSubmit(); return true; })()");
  await A.waitFor("location.pathname.startsWith('/meetups/') && document.querySelector('h1').textContent.includes('Browser test kafo')", "meetup page");
  // Other instances cache a cell's meetups for up to a minute: reload until it shows.
  for (let i = 0; ; i++) {
    await B.goto(base + "/?cell=8ccgqw");
    try { await B.waitFor("document.getElementById('meetups').textContent.includes('Browser test kafo')", "meetup in B's Around", 8000); break; }
    catch (e) { if (i >= 10) throw e; }
  }
  // Clean up (this test also runs against production).
  await A.evaluate("window.confirm = () => true; document.querySelector('form[action$=\"/delete\"]').requestSubmit(); true");
  await sleep(1000);

  // Discoverable: A names itself, speaks Esperanto, becomes visible; B sees A.
  const nick = "bt" + Date.now().toString(36);
  await A.goto(base + "/account");
  await A.evaluate(`(() => { const f = document.querySelector('form[action="/account/name"]'); f.username.value = "${nick}"; f.requestSubmit(); return true; })()`);
  await A.waitFor(`document.body.textContent.includes("@${nick}")`, "username set");
  await A.evaluate(`(() => { const s = document.getElementById('add-lang'); s.value = 'epo'; s.onchange(); const f = document.getElementById('profile-form');
    f.cell.value = '8ccgqw'; f.visible_hours.value = '12'; f.where.value = 'test stand'; f.requestSubmit(); return true; })()`);
  await A.waitFor("document.querySelector('[name=where]') && document.querySelector('[name=where]').value === 'test stand' && !!document.querySelector('select[name=level_epo]')", "profile saved");
  for (let i = 0; ; i++) {
    await B.goto(base + "/?cell=8ccgqw");
    try { await B.waitFor(`document.getElementById('people').textContent.includes("${nick}")`, "A in B's People", 8000); break; }
    catch (e) { if (i >= 10) throw e; }
  }
  // Hide again.
  await A.goto(base + "/account");
  await A.evaluate("(() => { const f = document.getElementById('profile-form'); f.visible_hours.value = '0'; f.requestSubmit(); return true; })()");
  await sleep(800);


  // Share with self: a fresh browser (C) shows a move code, B sends everything.
  const C = await browser(9335);
  try {
    await C.goto(base + "/contacts");
    await C.evaluate("document.getElementById('move-start').click()");
    await C.waitFor("!!document.querySelector('#move-qr svg')", "C's move code");
    const mv = await C.evaluate("(async () => (await window.kafumuPair.create({fetch, store: kafumuDevice.store, origin: location.origin}).invite(false, 'move')).url)()");
    await B.goto(mv);
    await B.evaluate("document.getElementById('move-send').click()");
    await B.waitFor("document.getElementById('move-send').hidden", "B sent");
    await C.waitFor("!document.getElementById('move-apply').hidden", "C received");
    await C.evaluate("document.getElementById('move-apply').click()");
    await sleep(2000);
    await C.waitFor("document.getElementById('contacts').textContent.includes('Ana')", "Ana moved to C");
  } finally { C.close(); }
  console.log("ok  connect pages in two browsers (A shows, B scans, both connected, both on Contacts, unticked field withheld, signal sent and seen, moved to a new device, meetup hosted and seen, findable profile seen)");
} catch (e) {
  console.error("FAIL", e.message); process.exitCode = 1;
} finally { A.close(); B.close(); }
