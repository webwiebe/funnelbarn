/**
 * Projects the user starred in the project switcher.
 *
 * FunnelBarn has no per-user settings store on the server yet, so the set lives
 * in localStorage and follows the browser, not the account. Moving it to the
 * server later only changes this file.
 */
export const FAVOURITE_PROJECTS_KEY = 'funnelbarn:favouriteProjects'

export function readFavouriteProjectIds(): string[] {
  if (typeof window === 'undefined') return []
  try {
    const raw = window.localStorage.getItem(FAVOURITE_PROJECTS_KEY)
    const parsed: unknown = raw ? JSON.parse(raw) : []
    return Array.isArray(parsed) ? parsed.filter((v): v is string => typeof v === 'string') : []
  } catch {
    return []
  }
}

export function writeFavouriteProjectIds(ids: string[]): void {
  if (typeof window === 'undefined') return
  try {
    window.localStorage.setItem(FAVOURITE_PROJECTS_KEY, JSON.stringify(ids))
  } catch {
    // Storage can be unavailable (private window); stars then last for the session only.
  }
}

export function toggleFavouriteId(ids: readonly string[], id: string): string[] {
  return ids.includes(id) ? ids.filter((x) => x !== id) : [...ids, id]
}
