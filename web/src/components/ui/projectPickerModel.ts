import type { Project } from '../../lib/api'

const collator = new Intl.Collator(undefined, { sensitivity: 'base', numeric: true })

export function sortProjects(projects: readonly Project[]): Project[] {
  return [...projects].sort((a, b) => collator.compare(a.name, b.name))
}

/** Case-insensitive match on name and slug. An empty query keeps everything. */
export function filterProjects(projects: readonly Project[], query: string): Project[] {
  const q = query.trim().toLowerCase()
  if (!q) return [...projects]
  return projects.filter((p) => p.name.toLowerCase().includes(q) || p.slug.toLowerCase().includes(q))
}

export interface ProjectGroups {
  favourites: Project[]
  others: Project[]
}

/** Starred projects first in their own group; both groups sorted alphabetically. */
export function groupProjects(projects: readonly Project[], favouriteIds: readonly string[]): ProjectGroups {
  const sorted = sortProjects(projects)
  return {
    favourites: sorted.filter((p) => favouriteIds.includes(p.id)),
    others: sorted.filter((p) => !favouriteIds.includes(p.id)),
  }
}

/** The order the menu shows rows in, which is also the order Enter picks from. */
export function menuOrder(projects: readonly Project[], favouriteIds: readonly string[], query: string): Project[] {
  const { favourites, others } = groupProjects(filterProjects(projects, query), favouriteIds)
  return [...favourites, ...others]
}
