/**
 * The project the user last switched to in the header picker.
 *
 * This lives on its own so that the picker (which writes it) and
 * ProjectProvider (which reads it back for pages that carry no `/:projectId`
 * in their URL, e.g. /settings) agree on one value. They used to disagree: the
 * picker remembered its own id while the provider only knew about the "default
 * project" chip, so /settings could show one project in the header and mint an
 * API key for another.
 */
export const LAST_PROJECT_ID_KEY = 'funnelbarn:lastProjectId'

export function readLastProjectId(): string | undefined {
  if (typeof window === 'undefined') return undefined
  try {
    return window.localStorage.getItem(LAST_PROJECT_ID_KEY) ?? undefined
  } catch {
    return undefined
  }
}

/**
 * The one definition of which project is "current": the URL's own id, then the
 * project last chosen in the header picker, then the "default project"
 * preference, then the first project.
 *
 * Every candidate is checked against the loaded projects, so a remembered id
 * for a project that has since been deleted falls through instead of
 * addressing nothing. Kept as a plain function so the landing redirect and
 * every project-aware page resolve identically and cannot drift apart.
 */
export function resolveProjectId(
  projects: { id: string }[],
  urlProjectId?: string,
  selectedProjectId?: string | null,
  defaultProjectId?: string | null,
): string | undefined {
  if (urlProjectId) return urlProjectId
  const known = (id?: string | null) => !!id && projects.some((p) => p.id === id)
  if (known(selectedProjectId)) return selectedProjectId!
  if (known(defaultProjectId)) return defaultProjectId!
  return projects[0]?.id
}

/**
 * Subscribers notified when the selection changes. localStorage alone is not
 * enough: a write from the picker has to re-render the page underneath it in
 * the same tab, and the `storage` event only fires in *other* tabs.
 */
const listeners = new Set<(id: string) => void>()

export function subscribeLastProjectId(fn: (id: string) => void): () => void {
  listeners.add(fn)
  return () => { listeners.delete(fn) }
}

export function writeLastProjectId(id: string) {
  if (typeof window !== 'undefined') {
    try {
      window.localStorage.setItem(LAST_PROJECT_ID_KEY, id)
    } catch {
      /* ignore — quota / disabled storage */
    }
  }
  listeners.forEach((fn) => fn(id))
}
