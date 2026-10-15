// The chat-list model (85a): node test/threads_test.mjs
import { readFileSync } from "node:fs";
import assert from "node:assert/strict";
new Function(readFileSync(new URL("../static/threads.js", import.meta.url), "utf8"))();
const { build } = globalThis.kafumuThreads;
const T = { act_connected: "Connected", unnamed: "someone new", act_inbox: "Public inbox" };
const ts = build({
  contacts: [
    { id: "ana", alias: "Ana", createdAt: "2026-10-01T10:00:00Z", messages: [{ me: true, text: "hoi", at: "2026-10-02T10:00:00Z" }, { me: false, text: "hallo", at: "2026-10-02T11:00:00Z" }], unreadMsgs: 1,
      signals: [{ s: "coffee", at: "2026-10-01T12:00:00Z", unread: true }] },
    { id: "bea", card: { name: "Bea" }, createdAt: "2026-10-05T10:00:00Z" },
    { id: "cas", createdAt: "2026-10-06T10:00:00Z" },
  ],
  answers: [{ id: "t1", post: { text: "Coffee at the market?" }, messages: [{ me: false, text: "yes!", at: "2026-10-03T09:00:00Z" }], unreadMsgs: 1 }],
  notices: [
    { thread: "p:abc", kind: "post", title: "My post", icon: "💬", who: "@x", text: "nice", at: "2026-10-04T08:00:00Z", href: "/?cell=9f469g#re-abc", unread: true },
    { thread: "p:abc", kind: "post", title: "My post", icon: "👍", text: "👍", at: "2026-10-04T07:00:00Z", href: "/?cell=9f469g#re-abc" },
    { thread: "e:m1", kind: "meetup", title: "Kafo", icon: "🙋", text: "2 going", at: "2026-10-01T08:00:00Z", href: "/meetups/m1" },
  ],
  inbox: [{ id: "i1", card: { name: "Dan" }, text: "hi shop", at: "2026-10-02T08:00:00Z", unread: true }],
  signalText: (s) => "☕ " + s.s,
}, T);

assert.deepEqual(ts.map(t => t.id), ["p:abc", "a:t1", "c:ana", "i:inbox", "e:m1", "c:cas", "c:bea"], "newest first, quiet contacts last (newest connection first)");
const ana = ts.find(t => t.id === "c:ana");
assert.equal(ana.twoWay, true);
assert.equal(ana.unread, 2, "an unread message and an unread signal");
assert.deepEqual(ana.items.map(i => i.text), ["Connected", "☕ coffee", "hoi", "hallo"], "oldest first, signals among the messages");
assert.equal(ts.find(t => t.id === "c:cas").title, "someone new", "never a bare ?");
const post = ts.find(t => t.id === "p:abc");
assert.equal(post.twoWay, false);
assert.equal(post.items.length, 2, "a post's replies and reactions in one thread");
assert.equal(post.unread, 1);
assert.equal(ts.find(t => t.id === "a:t1").title, "🔒 Coffee at the market?");
// Group boards (97g): a group shows before its first post; its posts land in it.
const gs = build({ groups: [{ id: "g1", name: "Esperantistoj" }, { id: "g2", name: "Quiet" }],
  notices: [{ thread: "g:g1", kind: "group", title: "x", text: "kafo?", who: "@ana", at: "2026-10-16T08:00:00Z", unread: true }] }, T);
assert.deepEqual(gs.map(t => t.id), ["g:g1", "g:g2"], "a group with posts first, an empty one after");
assert.equal(gs[0].title, "👥 Esperantistoj");
assert.equal(gs[0].unread, 1);
assert.equal(gs[0].url, "/chat?k=b:g1");
assert.equal(gs[1].quiet, true);
console.log("ok  static/threads.js (contacts always, two-way chats, one-way notices per thing)");
