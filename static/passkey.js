// Passkeys: register one on the account page, or sign in with one.
(function () {
  "use strict";
  var T = window.KAFUMU_T || {};
  if (!window.PublicKeyCredential) return;
  function b64u(buf) { return btoa(String.fromCharCode.apply(null, new Uint8Array(buf))).replace(/\+/g, "-").replace(/\//g, "_").replace(/=+$/, ""); }
  function unb64u(s) { s = s.replace(/-/g, "+").replace(/_/g, "/"); while (s.length % 4) s += "="; return Uint8Array.from(atob(s), function (c) { return c.charCodeAt(0); }); }
  function post(url, body) {
    return fetch(url, { method: "POST", credentials: "same-origin", headers: { "Content-Type": "application/json" }, body: body ? JSON.stringify(body) : "" })
      .then(function (r) { if (!r.ok) return r.text().then(function (t) { throw new Error(t); }); return r.json(); });
  }
  function status(id, msg) { var el = document.getElementById(id); if (el) el.textContent = msg; }

  var add = document.getElementById("passkey-add");
  if (add) {
    add.hidden = false;
    add.onclick = function () {
      post("/auth/passkey/register/begin").then(function (o) {
        var pk = o.publicKey;
        pk.challenge = unb64u(pk.challenge);
        pk.user.id = unb64u(pk.user.id);
        (pk.excludeCredentials || []).forEach(function (c) { c.id = unb64u(c.id); });
        return navigator.credentials.create({ publicKey: pk });
      }).then(function (c) {
        return post("/auth/passkey/register/finish", {
          id: c.id, rawId: b64u(c.rawId), type: c.type,
          response: { attestationObject: b64u(c.response.attestationObject), clientDataJSON: b64u(c.response.clientDataJSON) }
        });
      }).then(function () { status("passkey-status", T.passkey_added || "Passkey added."); })
        .catch(function (e) { status("passkey-status", (T.passkey_failed || "Didn't work:") + " " + e.message); });
    };
  }

  var login = document.getElementById("passkey-login");
  if (login) {
    login.hidden = false;
    login.onclick = function () {
      post("/auth/passkey/login/begin").then(function (o) {
        var pk = o.publicKey;
        pk.challenge = unb64u(pk.challenge);
        (pk.allowCredentials || []).forEach(function (c) { c.id = unb64u(c.id); });
        return navigator.credentials.get({ publicKey: pk });
      }).then(function (c) {
        return post("/auth/passkey/login/finish", {
          id: c.id, rawId: b64u(c.rawId), type: c.type,
          response: {
            authenticatorData: b64u(c.response.authenticatorData), clientDataJSON: b64u(c.response.clientDataJSON),
            signature: b64u(c.response.signature), userHandle: c.response.userHandle ? b64u(c.response.userHandle) : ""
          }
        });
      }).then(function () { location.href = "/account"; })
        .catch(function (e) { status("passkey-status", (T.passkey_failed || "Didn't work:") + " " + e.message); });
    };
  }
})();
