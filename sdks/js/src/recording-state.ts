import { RECORDING_STATE_KEY } from "./types.js";
import type { RecordingState } from "./types.js";

// Per-tab recording continuity. Persisted to sessionStorage on unload so a
// full-page navigation resumes the same recording instead of starting a new
// one: a visitor's whole journey becomes a single, continuous replay.

export function saveRecordingState(state: RecordingState): void {
  if (typeof sessionStorage === "undefined" || !state.id) return;
  try {
    sessionStorage.setItem(RECORDING_STATE_KEY, JSON.stringify(state));
  } catch {
    // sessionStorage may be full or unavailable; continuity is best-effort.
  }
}

export function resumeRecordingState(sessionTimeout: number): RecordingState | null {
  if (typeof sessionStorage === "undefined") return null;
  let raw: string | null;
  try {
    raw = sessionStorage.getItem(RECORDING_STATE_KEY);
  } catch {
    return null;
  }
  if (!raw) return null;
  try {
    const state = JSON.parse(raw) as RecordingState;
    // Discard stale state so a tab left idle past the session window starts a
    // new recording rather than stitching onto an abandoned one.
    if (!state.id || Date.now() - state.savedAt > sessionTimeout) {
      return null;
    }
    return state;
  } catch {
    return null;
  }
}

export function clearRecordingState(): void {
  if (typeof sessionStorage === "undefined") return;
  try {
    sessionStorage.removeItem(RECORDING_STATE_KEY);
  } catch {
    // ignore
  }
}
