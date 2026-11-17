// The environment the dashboard filters on. Production is the default, so a
// project that also receives test and staging events shows its real traffic
// first (#545). "All environments" is the empty string in memory and the
// literal "all" in storage, so the choice survives a reload.

const ENV_STORAGE_KEY = 'funnelbarn_environment'
const ALL_ENVIRONMENTS = 'all'
export const DEFAULT_ENVIRONMENT = 'production'

export function readStoredEnvironment(): string {
  let raw: string | null = null
  try {
    raw = localStorage.getItem(ENV_STORAGE_KEY)
  } catch {
    // Storage can be unavailable (private windows); the default applies.
  }
  if (raw === ALL_ENVIRONMENTS) return ''
  return raw || DEFAULT_ENVIRONMENT
}

export function writeStoredEnvironment(env: string): void {
  try {
    localStorage.setItem(ENV_STORAGE_KEY, env || ALL_ENVIRONMENTS)
  } catch {
    // The choice then lasts until the page reloads.
  }
}
