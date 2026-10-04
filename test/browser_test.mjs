// Two-browser test of the connect pages: A shows a code, B opens it, both
// end up connected. Drives headless Chromium over the DevTools protocol (no
// dependencies). Needs a running server and chromium on PATH:
//   PORT=18082 ./kafumu & node test/browser_test.mjs http://localhost:18082
import { browser, sleep } from "./cdp.mjs";

const base = process.argv[2] || "http://localhost:18082";
const fill = (form, values) => `(() => { const f = document.getElementById(${JSON.stringify(form)});
  ${Object.entries(values).map(([k, v]) => `f.elements[${JSON.stringify(k)}].value = ${JSON.stringify(v)};`).join("")}
  f.requestSubmit(); return true; })()`;

// Unique per run: production keeps earlier runs' messages for a while.
const RUN = Date.now().toString(36);
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

  // B follows the short code instead of the long link.
  await A.evaluate("document.getElementById('make-short').click()");
  await A.waitFor("document.getElementById('short-code').textContent.includes('/j/')", "short code");
  const short = await A.evaluate("document.getElementById('short-code').textContent");
  await B.goto(base + "/j/" + short.split("/j/")[1]);
  await B.waitFor("location.hash === '#' + " + JSON.stringify(url.split("#")[1]), "short code leads to the connect link");
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
  await A.goto(base + "/?cell=6fg222");
  await A.waitFor("document.getElementById('feed').textContent.includes('Bea')", "B's signal in A's Around");
  // Chat: B writes to Ana; A opens the chat with Bea and reads it.
  await B.goto(base + "/contacts");
  await B.waitFor("[...document.querySelectorAll('#contacts li button')].some(b => b.textContent.includes('Chat'))", "B's chat button");
  await B.evaluate("(() => { const li = [...document.querySelectorAll('#contacts li')].find(l => l.textContent.includes('Ana')); [...li.querySelectorAll('button')].find(b => b.textContent.includes('Chat')).click(); const f = li.querySelector('.chat-form'); f.querySelector('input').value = 'hallo " + RUN + "'; f.requestSubmit(); return true; })()");
  await B.waitFor("[...document.querySelectorAll('.bubble.me')].some(b => b.textContent === 'hallo " + RUN + "')", "B's message in the thread");
  await A.goto(base + "/contacts");
  await A.waitFor("[...document.querySelectorAll('#contacts li button')].some(b => b.textContent.includes('Chat (1)'))", "A sees an unread chat", 15000);
  await A.evaluate("(() => { const li = [...document.querySelectorAll('#contacts li')].find(l => l.textContent.includes('Bea')); [...li.querySelectorAll('button')].find(b => b.textContent.includes('Chat')).click(); return true; })()");
  await A.waitFor("[...document.querySelectorAll('.bubble:not(.me)')].some(b => b.textContent === 'hallo " + RUN + "')", "A reads B's message");
  // Local messages (OLN): A says something, mined in a worker; B sees it.
  await A.goto(base + "/?cell=6fg223");
  await A.evaluate("(() => { document.getElementById('say').click(); const f = document.getElementById('oln-form'); f.text.value = 'Browser test: hi from the OLN " + RUN + "'; f.requestSubmit(); return true; })()");
  await A.waitFor("document.getElementById('feed').textContent.includes('hi from the OLN " + RUN + "')", "A's message in Here now", 30000);
  // Joop saw messages twice: two loads in a row must still show each once.
  await A.evaluate("document.getElementById('view-apply').click(); document.getElementById('view-apply').click(); true");
  await sleep(3000);
  const copies = await A.evaluate("document.getElementById('feed').textContent.split('hi from the OLN " + RUN + "').length - 1");
  if (copies !== 1) throw new Error("message shown " + copies + " times");
  for (let i = 0; ; i++) { // B's browser may hold a 30-second-old bundle
    await B.goto(base + "/?cell=6fg223");
    try { await B.waitFor("document.getElementById('feed').textContent.includes('hi from the OLN " + RUN + "')", "B sees A's message", 8000); break; }
    catch (e) { if (i >= 6) throw e; }
  }
  // Questions: A asks; B gets it (with a private-answer button) and answers
  // publicly; the answer shows under the question.
  await A.evaluate("(() => { document.getElementById('ask').click(); const f = document.getElementById('oln-form'); f.text.value = 'Browser test: best pastel de nata nearby?'; f.tags.value = 'food'; f.requestSubmit(); return true; })()");
  await A.waitFor("document.getElementById('feed').textContent.includes('pastel de nata')", "A's question", 30000);
  for (let i = 0; ; i++) {
    await B.goto(base + "/?cell=6fg223");
    try { await B.waitFor("[...document.querySelectorAll('#feed > li')].some(li => li.textContent.includes('pastel de nata') && li.querySelector('a[href*=\"/c#v1.\"]'))", "B sees the question with a private-answer button", 8000); break; }
    catch (e) { if (i >= 8) throw e; }
  }
  await B.evaluate("(() => { [...document.querySelectorAll('#feed > li')].find(li => li.textContent.includes('pastel de nata')).querySelectorAll('button')[0].click(); const f = document.getElementById('oln-form'); f.text.value = 'Browser test answer: Manteigaria'; f.requestSubmit(); return true; })()");
  await B.waitFor("[...document.querySelectorAll('#feed > li')].some(li => li.textContent.includes('pastel de nata') && li.querySelector('.replies') && li.querySelector('.replies').textContent.includes('Manteigaria'))", "answer threaded under the question", 30000);

  // Card themes from local activity: the "food" question tag shows up as a
  // 📍 suggestion in A's card editor.
  await A.goto(base + "/?cell=6fg223");
  await A.waitFor("document.getElementById('feed').textContent.includes('pastel de nata')", "Around loaded for tag counting", 15000);
  await A.goto(base + "/card");
  await A.waitFor("[...document.querySelectorAll('#tag-chips .chip')].some(c => c.textContent === '📍 food')", "local theme suggested");

  // Learn the local language: Lisbon (PT) with an English UI offers Portuguese.
  await A.goto(base + "/?cell=8ccgqx");
  await A.waitFor("!document.getElementById('learn').hidden && document.getElementById('learn').textContent.includes('Portug')", "learn Portuguese offered", 20000);
  await A.evaluate("document.getElementById('learn').click(); true");
  await A.waitFor("document.getElementById('learn-view').href.includes('lang=por')", "learn view links to Portuguese");

  // Views: language and interest filters apply on the device, from the URL.
  await A.goto(base + "/?cell=6fg223&lang=eng");
  // Another instance may not have this run's message yet (60 s cache).
  for (let i = 0; ; i++) {
    try { await A.waitFor("document.getElementById('feed').textContent.includes('hi from the OLN " + RUN + "')", "message kept by lang=eng", 15000); break; }
    catch (e) { if (i >= 4) throw e; await A.goto(base + "/?cell=6fg223&lang=eng"); }
  }
  await A.goto(base + "/?cell=6fg223&tag=zzznothing&w=3");
  await A.waitFor("document.getElementById('views').textContent.includes('#zzznothing')", "active filter chip");
  await sleep(2500);
  if (await A.evaluate("document.getElementById('feed').textContent.includes('hi from the OLN " + RUN + "')")) throw new Error("tag filter didn't hide the message");
  await A.goto(base + "/?cell=6fg223&tag=zzznothing&w=1"); // a bias keeps everything
  await A.waitFor("document.getElementById('feed').textContent.includes('hi from the OLN " + RUN + "')", "message kept with a weak bias", 15000);
  // React to any card: 👍 on A's message shows as a count under it.
  await A.evaluate("(() => { const li = [...document.querySelectorAll('#feed > li')].find(li => li.textContent.includes('hi from the OLN " + RUN + "')); li.querySelector('button.react').click(); return true; })()");
  for (let i = 0; ; i++) {
    try { await A.waitFor("[...document.querySelectorAll('#feed > li')].some(li => li.textContent.includes('hi from the OLN " + RUN + "') && (li.querySelector('.reactions') || {}).textContent === '👍 1')", "👍 under the message", 15000); break; }
    catch (e) { if (i >= 4) throw e; await A.goto(base + "/?cell=6fg223&lang=eng"); }
  }
  // Report: ⚑ on the card, a reason, a stamped report; the card goes away here.
  await A.evaluate("(() => { const li = [...document.querySelectorAll('#feed > li')].find(li => li.textContent.includes('hi from the OLN " + RUN + "')); li.querySelector('button.report').click(); li.querySelector('.report-reasons .chip').click(); return true; })()");
  await A.waitFor("document.getElementById('feed').textContent.includes('Reported')", "report sent");

  // Who's up for coffee: A asks, B joins from the message and they connect.
  await A.goto(base + "/?cell=6fg223");
  await A.evaluate("document.getElementById('coffee').click(); document.getElementById('coffee-go').click(); true");
  await A.waitFor("document.getElementById('coffee-status').textContent.length > 0 && !document.getElementById('coffee-status').textContent.includes('…')", "coffee asked", 30000);
  for (let i = 0; ; i++) {
    await B.goto(base + "/?cell=6fg223");
    try { await B.waitFor("!!document.querySelector('#feed a[href*=\"/c#v1.\"]')", "Join button for B", 8000); break; }
    catch (e) { if (i >= 8) throw e; }
  }
  await B.evaluate("location.href = document.querySelector('#feed a[href*=\"/c#v1.\"]').href; true");
  await B.waitFor("!document.getElementById('accept-area').hidden", "B on the connect page from Join");

  // Meetups: A makes an account on the way to hosting, B sees it in Around.
  await A.goto(base + "/meetups/new");
  await A.evaluate("document.querySelector('form[action=\"/account/start\"]').requestSubmit()");
  await A.waitFor("!!document.querySelector('a[href=\"/meetups/new\"].suggested')", "continue link after account");
  await A.goto(base + "/meetups/new");
  await A.waitFor("!!document.getElementById('meetup-form')", "meetup form");
  await A.evaluate("(() => { const f = document.getElementById('meetup-form'); f.title.value = 'Browser test kafo'; f.cell.value = '6fg222'; f.venue.value = 'Pavilion 2'; f.requestSubmit(); return true; })()");
  await A.waitFor("location.pathname.startsWith('/meetups/') && document.querySelector('h1').textContent.includes('Browser test kafo')", "meetup page");
  // Other instances cache a cell's meetups for up to a minute: reload until it shows.
  for (let i = 0; ; i++) {
    await B.goto(base + "/?cell=6fg222");
    try { await B.waitFor("document.getElementById('feed').textContent.includes('Browser test kafo')", "meetup in B's Around", 8000); break; }
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
    f.cell.value = '6fg222'; f.visible_hours.value = '12'; f.where.value = 'test stand'; f.requestSubmit(); return true; })()`);
  await A.waitFor("document.querySelector('[name=where]') && document.querySelector('[name=where]').value === 'test stand' && !!document.querySelector('select[name=level_epo]')", "profile saved");
  for (let i = 0; ; i++) {
    await B.goto(base + "/?cell=6fg222");
    try { await B.waitFor(`document.getElementById('feed').textContent.includes("${nick}")`, "A in B's People", 8000); break; }
    catch (e) { if (i >= 10) throw e; }
  }
  // Public inbox: A opens one at 12 bits; B writes from People with a card;
  // A reads it in Contacts and connects back.
  await A.goto(base + "/account");
  await A.evaluate("(() => { document.getElementById('inbox-bits').value = '12'; const on = document.getElementById('inbox-on'); on.checked = true; on.onchange(); return true; })()");
  await A.waitFor("document.getElementById('inbox-status').textContent.length > 1", "inbox opened");
  for (let i = 0; ; i++) {
    await B.goto(base + "/?cell=6fg222");
    try { await B.waitFor(`[...document.querySelectorAll('#feed li')].some(li => li.textContent.includes("${nick}") && [...li.querySelectorAll('button')].find(b => b.textContent.startsWith('✉️')))`, "write button on A", 8000); break; }
    catch (e) { if (i >= 10) throw e; }
  }
  await B.evaluate(`(() => { const li = [...document.querySelectorAll('#feed li')].find(l => l.textContent.includes("${nick}")); [...li.querySelectorAll('button')].find(b => b.textContent.startsWith('✉️')).click();
    li.querySelector('textarea').value = 'Browser test: inbox hello'; li.querySelector('form').requestSubmit(); return true; })()`);
  await B.waitFor("[...document.querySelectorAll('#feed li form p')].some(p => /\\(\\d+ s/.test(p.textContent))", "inbox message sent", 60000);
  await A.goto(base + "/contacts");
  await A.waitFor("document.getElementById('inbox-msgs').textContent.includes('inbox hello')", "A reads the inbox message", 20000);
  await A.evaluate("document.querySelector('#inbox-msgs button.suggested').click()");
  await A.waitFor("document.getElementById('contacts').textContent.includes('Bea')", "connected back with Bea");

  // Hide again.
  await A.goto(base + "/account");
  await A.evaluate("(() => { const f = document.getElementById('profile-form'); f.visible_hours.value = '0'; f.requestSubmit(); return true; })()");
  await sleep(800);


  // Move to a new device, bound to the account: B makes an account; C signs
  // in with B's link and asks; B sees the request and sends; C gets B's data.
  const C = await browser(9335);
  try {
    await B.goto(base + "/account");
    await B.evaluate("document.querySelector('form[action=\"/account/start\"]').requestSubmit()");
    await B.waitFor("!!document.querySelector('.magic input')", "B's account");
    const link = await B.evaluate("document.querySelector('.magic input').value");
    await C.goto(link.replace(/^https?:\/\/[^/]+/, base));
    await C.goto(base + "/contacts");
    // Signed in, but the sync key is still on B: C is asked to get it once.
    await C.waitFor("!document.getElementById('sync-needs-key').hidden", "C asked to fetch the sync key");
    await C.evaluate("document.getElementById('move-start').click()");
    await C.waitFor("document.getElementById('move-code').textContent.length > 0", "C's move code");
    const emoji = await C.evaluate("document.getElementById('move-code').textContent");
    await B.goto(base + "/contacts");
    await B.waitFor("!document.getElementById('move-offer').hidden", "B sees the move request");
    if (await B.evaluate("document.getElementById('move-offer-code').textContent") !== emoji) throw new Error("emoji differ");
    await B.evaluate("document.getElementById('move-send').click()");
    await C.waitFor("!document.getElementById('move-apply').hidden", "C received", 30000);
    await C.evaluate("document.getElementById('move-apply').click()");
    await sleep(2000);
    await C.waitFor("document.getElementById('contacts').textContent.includes('Ana')", "Ana moved to C");
    // Cards came along (one merge for moves and sync), and a rename on B
    // reaches C.
    await C.goto(base + "/card");
    await C.waitFor("document.getElementById('card-form').elements.name.value === 'Bea'", "B's card on C");
    await B.goto(base + "/card");
    await B.waitFor("document.getElementById('card-form').elements.name.value === 'Bea'", "B's card form");
    await B.evaluate("(() => { const f = document.getElementById('card-form'); f.elements.name.value = 'Bea " + RUN + "'; f.requestSubmit(); return true; })()");
    await sleep(4000);
    for (let i = 0; ; i++) {
      await C.goto(base + "/card");
      try { await C.waitFor("document.getElementById('card-form').elements.name.value === 'Bea " + RUN + "'", "B's renamed card on C", 6000); break; }
      catch (e) { if (i >= 3) throw e; }
    }
    // From now on B and C stay in sync by themselves: a note B writes shows
    // up on C; a contact C removes stays removed on B (tombstone).
    await B.goto(base + "/contacts");
    await B.waitFor("!document.getElementById('sync-on').hidden", "B syncing");
    await B.evaluate("(() => { const i = document.querySelector('#contacts li input:not(.chip-input):not(.alias)'); i.value = 'synced " + RUN + "'; i.dispatchEvent(new Event('change')); return true; })()");
    await sleep(4000);
    for (let i = 0; ; i++) {
      await C.goto(base + "/contacts");
      try { await C.waitFor("[...document.querySelectorAll('#contacts input')].some(i => i.value === 'synced " + RUN + "')", "B's note synced to C", 6000); break; }
      catch (e) { if (i >= 3) throw e; }
    }
    await C.evaluate("(() => { window.confirm = () => true; [...document.querySelectorAll('#contacts li')].find(li => [...li.querySelectorAll('input')].some(i => i.value === 'synced " + RUN + "')).querySelector('button.contrast').click(); return true; })()");
    await sleep(4000);
    for (let i = 0; ; i++) {
      await B.goto(base + "/contacts");
      await sleep(2500);
      if (!(await B.evaluate("[...document.querySelectorAll('#contacts input')].some(i => i.value === 'synced " + RUN + "')"))) break;
      if (i >= 3) throw new Error("contact removed on C came back on B");
    }
  } finally { C.close(); }

  // Connecting needs no data (Joop): D scans A's code with no name at all;
  // A gives D a name; D's card follows later over the same connection.
  const D = await browser(9336);
  try {
    // First visit, no location: "Are you in or near …?" from App Engine's
    // city header (set by hand locally; production sets its own).
    if (!/kafumu\.com/.test(base)) await D.send("Network.enable"), await D.send("Network.setExtraHTTPHeaders", { headers: { "X-Appengine-Citylatlong": "52.040000,5.665000" } });
    await D.goto(base + "/");
    if (!/kafumu\.com/.test(base)) await D.waitFor("!document.getElementById('guess').hidden", "area guess offered");
    else { await sleep(2500); if (await D.evaluate("document.getElementById('guess').hidden")) console.log("note: no area guess for this connection"); }
    if (!/kafumu\.com/.test(base)) {
      await D.waitFor("document.getElementById('guess-q').textContent.includes('Ede')", "guess names Ede");
      await D.evaluate("document.getElementById('guess-yes').click()");
      await D.waitFor("document.getElementById('place-name').textContent === 'Ede'", "Yes shows Ede");
      await D.send("Network.setExtraHTTPHeaders", { headers: {} });
    }

    await A.goto(base + "/connect");
    await A.waitFor("!!document.querySelector('#qr svg')", "A's QR again");
    // A leaves Connect (app "closed" for this code) before D uses it: D's
    // hello waits in the mailbox, and A takes it in on opening Contacts.
    await A.goto(base + "/about");
    await D.goto(url);
    await D.waitFor("!document.getElementById('accept-area').hidden", "D can connect without a name");
    await D.evaluate("document.getElementById('do-connect').click()");
    await sleep(2000);
    await A.goto(base + "/contacts");
    await A.waitFor("!!document.querySelector('#contacts input.alias')", "A takes in D's hello from the queue", 30000);
    await D.waitFor("document.getElementById('accept-status').textContent.includes('Ana')", "D gets A's card later", 30000);
    await A.evaluate("(() => { const i = document.querySelector('#contacts input.alias'); i.value = 'Dee from the queue'; i.dispatchEvent(new Event('change')); return true; })()");
    await sleep(1000);
    await A.goto(base + "/contacts");
    await A.waitFor("[...document.querySelectorAll('#contacts input.alias')].some(i => i.value === 'Dee from the queue')", "A's own name for D kept");
    await D.evaluate(fill("name-form", { name: "Dee Late" }));
    await sleep(2000);
    for (let i = 0; ; i++) {
      await A.goto(base + "/contacts");
      try { await A.waitFor("document.getElementById('contacts').textContent.includes('Dee Late')", "D's late card arrives", 6000); break; }
      catch (e) { if (i >= 3) throw e; }
    }
  } finally { D.close(); }
  // Leave no test accounts behind in production (they cluttered /admin).
  for (const X of [A, B]) {
    await X.evaluate("fetch('/account/delete', { method: 'POST', body: new URLSearchParams({ confirm: 'yes' }), credentials: 'same-origin' }).then(() => true)");
  }
  console.log("ok  area picker (search, 7×7 map, tap a block)");
  console.log("ok  connect pages in two browsers (A shows, B follows a short code, both connected, both on Contacts, unticked field withheld, signal sent and seen, chat both ways, moved to a new device + synced both ways (card rename, note, removal), first-visit area guess + connected without a card while A was away (queued) + named + late card, meetup hosted and seen, findable profile seen, paid inbox message + connect back, OLN message + reaction + report + question/answer + local themes + learn the local language + views + coffee Join)");
} catch (e) {
  console.error("FAIL", e.message); process.exitCode = 1;
} finally { A.close(); B.close(); }
