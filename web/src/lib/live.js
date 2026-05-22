// WebSocket client for live status updates. The server watches repo working
// trees and pushes a status summary whenever one changes; this keeps the
// sidebar indicators current without polling. Auto-reconnects.

import { writable } from 'svelte/store';
import { repoStatuses } from './repostatus.js';

// Bumped on each live update — components watch it to soft-refresh.
export const liveSignal = writable(null);

let ws = null;
let retry = 0;

export function startLive() {
  connect();
}

function connect() {
  const proto = location.protocol === 'https:' ? 'wss' : 'ws';
  try {
    ws = new WebSocket(`${proto}://${location.host}/api/v1/ws`);
  } catch {
    scheduleReconnect();
    return;
  }
  ws.onopen = () => {
    retry = 0;
  };
  ws.onmessage = (ev) => {
    let msg;
    try {
      msg = JSON.parse(ev.data);
    } catch {
      return;
    }
    if (msg.type === 'status' && msg.status && msg.status.id) {
      repoStatuses.update((m) => ({ ...m, [msg.status.id]: msg.status }));
      liveSignal.set({ repoId: msg.status.id, ts: Date.now() });
    }
  };
  ws.onclose = () => scheduleReconnect();
  ws.onerror = () => {
    try {
      ws.close();
    } catch {
      /* ignore */
    }
  };
}

function scheduleReconnect() {
  retry = Math.min(retry + 1, 6);
  setTimeout(connect, retry * 1000);
}
