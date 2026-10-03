// Passkeys end to end with Chrome's virtual authenticator:
//   PORT=18095 KAFUMU_ORIGIN=http://localhost:18095 KAFUMU_PASSKEYS=1 ./kafumu &
//   node test/passkey_test.mjs http://localhost:18095
import { browser, sleep } from "./cdp.mjs";
const base = process.argv[2] || "http://localhost:18095";
const A = await browser(9390);
try {
  await A.send("WebAuthn.enable");
  await A.send("WebAuthn.addVirtualAuthenticator", { options: { protocol: "ctap2", transport: "internal",
    hasResidentKey: true, hasUserVerification: true, isUserVerified: true, automaticPresenceSimulation: true } });
  await A.goto(base + "/account");
  await A.evaluate("document.querySelector('form[action=\"/account/start\"]').requestSubmit()");
  await A.waitFor("!!document.getElementById('passkey-add') && !document.getElementById('passkey-add').hidden", "add-passkey button");
  const link = await A.evaluate("document.querySelector('.magic input').value");
  await A.evaluate("document.getElementById('passkey-add').click()");
  await A.waitFor("document.getElementById('passkey-status').textContent.length > 0", "registration result");
  const reg = await A.evaluate("document.getElementById('passkey-status').textContent");
  if (!/added/i.test(reg)) throw new Error("register: " + reg);

  await A.send("Network.clearBrowserCookies");
  await A.goto(base + "/account");
  await A.waitFor("!!document.getElementById('passkey-login') && !document.getElementById('passkey-login').hidden", "passkey sign-in button");
  await A.evaluate("document.getElementById('passkey-login').click()");
  await A.waitFor("!!document.querySelector('.magic input')", "signed in with the passkey");

  await A.send("Network.clearBrowserCookies");
  await A.goto(link);
  await A.waitFor("!!document.querySelector('.magic input')", "magic link still works");
  console.log("ok  passkeys (register, sign in, magic link still valid)");
} catch (e) {
  console.error("FAIL", e.message); process.exitCode = 1;
} finally { A.close(); }
