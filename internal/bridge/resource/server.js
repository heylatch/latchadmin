// LatchAdmin bridge: shows the panel's announcements and restart warnings on players' screens.
// Installed and updated by LatchAdmin; edits here are overwritten. https://github.com/heylatch/latchadmin
//
// It only listens: it asks the panel (on this machine, address and key from the environment LatchAdmin starts the
// server with) for notices, and passes each one to every connected player. It sends nothing about players back.
"use strict";

const BASE = process.env.LATCHADMIN_BRIDGE_URL || "";
const KEY = process.env.LATCHADMIN_BRIDGE_TOKEN || "";
const EVENT = "latchadmin:notice";
const sleep = (ms) => new Promise((resolve) => setTimeout(resolve, ms));

function players() {
  try {
    if (typeof PlayerManager !== "undefined" && typeof PlayerManager.getAll === "function") return PlayerManager.getAll() || [];
  } catch (e) {}
  return [];
}

function show(notice) {
  let count = 0;
  for (const player of players()) {
    try {
      player.emit(EVENT, notice);
      count++;
    } catch (e) {}
  }
  console.log(`${notice.kind === "restart" ? "Restart warning" : "Announcement"} shown to ${count} player${count === 1 ? "" : "s"}`);
}

async function listen() {
  if (!BASE || !KEY) {
    console.log("Not started by LatchAdmin; in-game notices are off.");
    return;
  }
  let failures = 0;
  for (;;) {
    try {
      // The panel holds this request open until there's a notice (or about 25 seconds pass).
      const res = await fetch(`${BASE}/poll?v=1`, { headers: { Authorization: `Bearer ${KEY}` }, signal: AbortSignal.timeout(40000) });
      if (res.status === 401) {
        console.log("The panel didn't accept this server's key. Restart the server from LatchAdmin.");
        await sleep(60000);
        continue;
      }
      if (!res.ok) throw new Error(`HTTP ${res.status}`);
      const body = await res.json();
      failures = 0;
      for (const notice of body.notices || []) show(notice);
    } catch (e) {
      failures++;
      await sleep(Math.min(30000, 1000 * 2 ** Math.min(failures, 5)));
    }
  }
}

listen();
