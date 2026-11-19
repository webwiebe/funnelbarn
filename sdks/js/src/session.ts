import { SESSION_EXPIRY_KEY, SESSION_KEY, SESSION_STARTED_KEY } from "./types.js";
import { generateSessionID } from "./helpers.js";

/**
 * Returns the current session ID, starting a new one when the idle window has
 * passed or the session is older than maxAge. Empty without localStorage.
 */
export function getOrCreateSessionID(timeout: number, maxAge: number): string {
  if (typeof localStorage === "undefined") {
    return "";
  }

  const now = Date.now();
  const expiry = parseInt(localStorage.getItem(SESSION_EXPIRY_KEY) ?? "0", 10);
  const startedAt = parseInt(localStorage.getItem(SESSION_STARTED_KEY) ?? "0", 10);

  // The idle window slides on every event, so on its own it never expires a
  // session that keeps seeing activity. Age it out as well.
  const idle = now >= expiry;
  const tooOld = startedAt > 0 && now - startedAt >= maxAge;
  const existing = localStorage.getItem(SESSION_KEY);

  if (!idle && !tooOld && existing) {
    // Extend session expiry on activity.
    localStorage.setItem(SESSION_EXPIRY_KEY, String(now + timeout));
    return existing;
  }

  // New session.
  const id = generateSessionID();
  localStorage.setItem(SESSION_KEY, id);
  localStorage.setItem(SESSION_EXPIRY_KEY, String(now + timeout));
  localStorage.setItem(SESSION_STARTED_KEY, String(now));
  return id;
}
