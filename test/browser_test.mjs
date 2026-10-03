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
  // Area picker: no location prompt on load; search, map, tap a block.
  await A.goto(base + "/");
  await A.waitFor("!document.getElementById('picker').hidden", "picker shown on first visit");
  await A.evaluate("(() => { const q = document.getElementById('place-q'); q.value = 'Barreiro'; q.dispatchEvent(new Event('input')); return true; })()");
  await A.waitFor("document.querySelectorAll('#place-results button').length > 0", "search results");
  await A.evaluate("document.querySelector('#place-results button').click()");
  await A.waitFor("document.querySelectorAll('.area-map .cell').length === 49", "7×7 cell grid");
  // Pan east with the arrow: the grid moves, so its first block changes.
  const firstBefore = await A.evaluate("document.querySelector('.area-map .cell').title");
  await A.evaluate("[...document.querySelectorAll('.area-map .pan')].find(b => b.textContent === '→').click()");
  await A.waitFor(`document.querySelector('.area-map .cell').title !== ${JSON.stringify(firstBefore)}`, "map panned");
  const before = await A.evaluate("document.getElementById('cell-tag').textContent");
  await A.evaluate("document.querySelectorAll('.area-map .cell')[10].click()");
  await A.waitFor(`document.getElementById('cell-tag').textContent !== ${JSON.stringify(before)}`, "tapped block becomes the area");

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
  // Local messages (OLN): A says something, mined in a worker; B sees it.
  await A.goto(base + "/?cell=8ccgqx");
  await A.evaluate("(() => { document.getElementById('say').click(); const f = document.getElementById('oln-form'); f.text.value = 'Browser test: hi from the OLN'; f.requestSubmit(); return true; })()");
  await A.waitFor("document.getElementById('notes').textContent.includes('hi from the OLN')", "A's message in Here now", 30000);
  for (let i = 0; ; i++) { // B's browser may hold a 30-second-old bundle
    await B.goto(base + "/?cell=8ccgqx");
    try { await B.waitFor("document.getElementById('notes').textContent.includes('hi from the OLN')", "B sees A's message", 8000); break; }
    catch (e) { if (i >= 6) throw e; }
  }
  // Questions: A asks; B gets it (with a private-answer button) and answers
  // publicly; the answer shows under the question.
  await A.evaluate("(() => { document.getElementById('ask').click(); const f = document.getElementById('oln-form'); f.text.value = 'Browser test: best pastel de nata nearby?'; f.tags.value = 'food'; f.requestSubmit(); return true; })()");
  await A.waitFor("document.getElementById('notes').textContent.includes('pastel de nata')", "A's question", 30000);
  for (let i = 0; ; i++) {
    await B.goto(base + "/?cell=8ccgqx");
    try { await B.waitFor("[...document.querySelectorAll('#notes > li')].some(li => li.textContent.includes('pastel de nata') && li.querySelector('a[href*=\"/c#v1.\"]'))", "B sees the question with a private-answer button", 8000); break; }
    catch (e) { if (i >= 8) throw e; }
  }
  await B.evaluate("(() => { [...document.querySelectorAll('#notes > li')].find(li => li.textContent.includes('pastel de nata')).querySelectorAll('button')[0].click(); const f = document.getElementById('oln-form'); f.text.value = 'Browser test answer: Manteigaria'; f.requestSubmit(); return true; })()");
  await B.waitFor("[...document.querySelectorAll('#notes > li')].some(li => li.textContent.includes('pastel de nata') && li.querySelector('.replies') && li.querySelector('.replies').textContent.includes('Manteigaria'))", "answer threaded under the question", 30000);

  // Views: language and interest filters apply on the device, from the URL.
  await A.goto(base + "/?cell=8ccgqx&lang=eng");
  await A.waitFor("document.getElementById('notes').textContent.includes('hi from the OLN')", "message kept by lang=eng", 15000);
  await A.goto(base + "/?cell=8ccgqx&tag=zzznothing&w=3");
  await A.waitFor("document.getElementById('views').textContent.includes('#zzznothing')", "active filter chip");
  await sleep(2500);
  if (await A.evaluate("document.getElementById('notes').textContent.includes('hi from the OLN')")) throw new Error("tag filter didn't hide the message");
  await A.goto(base + "/?cell=8ccgqx&tag=zzznothing&w=1"); // a bias keeps everything
  await A.waitFor("document.getElementById('notes').textContent.includes('hi from the OLN')", "message kept with a weak bias", 15000);

  // Who's up for coffee: A asks, B joins from the message and they connect.
  await A.goto(base + "/?cell=8ccgqx");
  await A.evaluate("document.getElementById('coffee').click(); document.getElementById('coffee-go').click(); true");
  await A.waitFor("document.getElementById('coffee-status').textContent.length > 0 && !document.getElementById('coffee-status').textContent.includes('…')", "coffee asked", 30000);
  for (let i = 0; ; i++) {
    await B.goto(base + "/?cell=8ccgqx");
    try { await B.waitFor("!!document.querySelector('#notes a[href*=\"/c#v1.\"]')", "Join button for B", 8000); break; }
    catch (e) { if (i >= 8) throw e; }
  }
  await B.evaluate("location.href = document.querySelector('#notes a[href*=\"/c#v1.\"]').href; true");
  await B.waitFor("!document.getElementById('accept-area').hidden", "B on the connect page from Join");

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
  // Public inbox: A opens one at 12 bits; B writes from People with a card;
  // A reads it in Contacts and connects back.
  await A.goto(base + "/account");
  await A.evaluate("(() => { document.getElementById('inbox-bits').value = '12'; const on = document.getElementById('inbox-on'); on.checked = true; on.onchange(); return true; })()");
  await A.waitFor("document.getElementById('inbox-status').textContent.length > 1", "inbox opened");
  for (let i = 0; ; i++) {
    await B.goto(base + "/?cell=8ccgqw");
    try { await B.waitFor(`[...document.querySelectorAll('#people li')].some(li => li.textContent.includes("${nick}") && li.querySelector('button.pill-sm'))`, "write button on A", 8000); break; }
    catch (e) { if (i >= 10) throw e; }
  }
  await B.evaluate(`(() => { const li = [...document.querySelectorAll('#people li')].find(l => l.textContent.includes("${nick}")); li.querySelector('button.pill-sm').click();
    li.querySelector('textarea').value = 'Browser test: inbox hello'; li.querySelector('form').requestSubmit(); return true; })()`);
  await B.waitFor("[...document.querySelectorAll('#people li form p')].some(p => /\\(\\d+ s/.test(p.textContent))", "inbox message sent", 60000);
  await A.goto(base + "/contacts");
  await A.waitFor("document.getElementById('inbox-msgs').textContent.includes('inbox hello')", "A reads the inbox message", 20000);
  await A.evaluate("document.querySelector('#inbox-msgs button.suggested').click()");
  await A.waitFor("document.getElementById('contacts').textContent.includes('Bea')", "connected back with Bea");

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
  console.log("ok  area picker (search, 7×7 map, tap a block)");
  console.log("ok  connect pages in two browsers (A shows, B scans, both connected, both on Contacts, unticked field withheld, signal sent and seen, moved to a new device, meetup hosted and seen, findable profile seen, paid inbox message + connect back, OLN message + question/answer + views + coffee Join)");
} catch (e) {
  console.error("FAIL", e.message); process.exitCode = 1;
} finally { A.close(); B.close(); }
