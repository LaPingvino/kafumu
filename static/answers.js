// Private answers (LOOP-STATE 69), in the mailbox (83b): the conversations
// that started as a private answer to a post, both ways, each with its
// reply form. Drawn on /activity; the home page only links here.
(function () {
  "use strict";
  var T = window.KAFUMU_T || {};
  function tr(k, v) { var s = T[k] || k; for (var n in v || {}) s = s.replace("{" + n + "}", v[n]); return s; }

  // mount draws the threads into sec (a section with an h2 and a ul) and
  // keeps them fresh: new answers are read every minute while visible.
  function mount(sec, aPair, post) {
    var ul = sec.querySelector("ul"), linked = false;
    function draw() {
      return aPair.threads().then(function (ts) {
        sec.hidden = !ts.length;
        ul.textContent = "";
        ts.slice().sort(function (a, b) {
          var la = (a.messages || []).slice(-1)[0] || {}, lb = (b.messages || []).slice(-1)[0] || {};
          return (lb.at || "").localeCompare(la.at || "");
        }).slice(0, 20).forEach(function (t) {
          var li = document.createElement("li");
          li.dataset.thread = t.id;
          li.id = "answer-" + t.id;
          var about = document.createElement("div"); about.className = "dim small";
          about.textContent = tr("answers_on", { text: (t.post && t.post.text || "").slice(0, 80) });
          if (t.unreadMsgs) { // new since you last looked: a small count badge
            var nb = document.createElement("span"); nb.className = "badge new-count"; nb.textContent = String(t.unreadMsgs);
            about.appendChild(document.createTextNode(" ")); about.appendChild(nb);
          }
          li.appendChild(about);
          (t.messages || []).slice(-6).forEach(function (m) {
            var p = document.createElement("p"); p.className = m.me ? "chat-me" : "chat-them";
            p.textContent = (m.me ? "→ " : "← ") + m.text;
            li.appendChild(p);
          });
          var form = document.createElement("form"); form.className = "inline-row answer-form-row";
          var inp = document.createElement("input"); inp.maxLength = 500; inp.placeholder = tr("answer_reply"); inp.required = true;
          inp.setAttribute("aria-label", tr("answer_reply"));
          var go = document.createElement("button"); go.type = "submit"; go.className = "pill-sm"; go.textContent = tr("answer_send");
          form.appendChild(inp); form.appendChild(go); li.appendChild(form);
          form.onsubmit = function (e) {
            e.preventDefault(); go.disabled = true;
            var text = inp.value;
            aPair.sendChat(t, text, post).then(function (at) {
              t.messages = (t.messages || []).concat([{ me: true, text: text, at: at }]).slice(-200);
              return aPair.putThread(t);
            }).then(draw, function () { go.disabled = false; });
          };
          ul.appendChild(li);
          if (t.unreadMsgs) { t.unreadMsgs = 0; aPair.putThread(t); }
          // Opened at one thread ("#answer-<id>", from the list or a notification): ready to answer.
          if (!linked && decodeURIComponent(location.hash) === "#answer-" + t.id) {
            linked = true;
            setTimeout(function () {
              li.scrollIntoView({ block: "center", behavior: "smooth" });
              li.classList.add("flash"); inp.focus({ preventScroll: true });
              setTimeout(function () { li.classList.remove("flash"); }, 2500);
            }, 0);
          }
        });
      });
    }
    function poll() {
      aPair.readAnswers().then(draw, draw).then(function () {
        setTimeout(function () { if (!document.hidden) poll(); }, 60000);
      });
    }
    poll();
    document.addEventListener("visibilitychange", function () { if (!document.hidden) poll(); });
    window.addEventListener("hashchange", function () { linked = false; draw(); });
    return { draw: draw };
  }

  window.kafumuAnswers = { mount: mount };
})();
