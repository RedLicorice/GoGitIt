// App-wide toast notifications. Lives outside any component so toasts survive
// repo switches and report background work (e.g. a fetch that finished while
// the user was looking at another repo).

import { writable } from 'svelte/store';

export const toasts = writable([]);

let seq = 1;
const MAX = 6;

// addToast shows a notification.
//   kind:    'success' | 'error' | 'info'
//   message: text to display
//   action:  optional { label, run } — renders a button that runs `run`
//   timeout: ms before auto-dismiss; 0 keeps it until dismissed (use for
//            actionable toasts so the user doesn't miss the button)
export function addToast({ kind = 'info', message, action = null, timeout = 6000 }) {
  const id = seq++;
  toasts.update((list) => [...list, { id, kind, message, action }].slice(-MAX));
  if (timeout > 0) {
    setTimeout(() => dismissToast(id), timeout);
  }
  return id;
}

export function dismissToast(id) {
  toasts.update((list) => list.filter((t) => t.id !== id));
}
