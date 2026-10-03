// Minimal Chrome DevTools Protocol driver for tests: no dependencies.
import { spawn } from "node:child_process";
import { mkdtempSync, rmSync } from "node:fs";
import { tmpdir } from "node:os";
import { join } from "node:path";

export const sleep = (ms) => new Promise((r) => setTimeout(r, ms));

export async function browser(port) {
  const dir = mkdtempSync(join(tmpdir(), "kafumu-chr-"));
  const proc = spawn("chromium", ["--headless=new", "--disable-gpu", `--remote-debugging-port=${port}`,
    `--user-data-dir=${dir}`, "--no-first-run", "about:blank"], { stdio: "ignore" });
  let target;
  for (let i = 0; i < 50 && !target; i++) {
    await sleep(200);
    try { target = (await (await fetch(`http://127.0.0.1:${port}/json`)).json()).find((t) => t.type === "page"); } catch {}
  }
  const ws = new WebSocket(target.webSocketDebuggerUrl);
  await new Promise((r) => ws.addEventListener("open", r, { once: true }));
  let id = 0; const pending = new Map();
  ws.addEventListener("message", (e) => { const m = JSON.parse(e.data); if (pending.has(m.id)) { pending.get(m.id)(m); pending.delete(m.id); } });
  const send = (method, params = {}) => new Promise((r) => { pending.set(++id, r); ws.send(JSON.stringify({ id, method, params })); });
  // Headless Chrome says "HeadlessChrome", which Kafumu treats as a bot.
  await send("Network.setUserAgentOverride", {
    userAgent: "Mozilla/5.0 (X11; Linux x86_64) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/140.0 Safari/537.36 kafumu-test",
    acceptLanguage: "en",
  });
  const evaluate = async (expr) => {
    const m = await send("Runtime.evaluate", { expression: expr, awaitPromise: true, returnByValue: true });
    if (m.result?.exceptionDetails) throw new Error(m.result.exceptionDetails.exception?.description || "eval failed");
    return m.result?.result?.value;
  };
  return {
    send,
    goto: async (url) => { await send("Page.navigate", { url }); await sleep(800); },
    evaluate,
    waitFor: async (expr, what, ms = 20000) => {
      for (const end = Date.now() + ms; Date.now() < end; await sleep(250)) if (await evaluate(expr)) return;
      throw new Error("timed out waiting for " + what + "; page says: " + await evaluate("document.body.innerText.slice(0, 400)"));
    },
    close: () => { ws.close(); proc.kill(); setTimeout(() => rmSync(dir, { recursive: true, force: true }), 500); },
  };
}

