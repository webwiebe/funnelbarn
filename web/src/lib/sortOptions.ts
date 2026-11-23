const collator = new Intl.Collator(undefined, { sensitivity: 'base', numeric: true })

/** Sorts select options alphabetically by label: case-insensitive, locale-aware, digits in natural order. */
export function sortByLabel<T>(items: readonly T[], label: (item: T) => string): T[] {
  return [...items].sort((a, b) => collator.compare(label(a), label(b)))
}

/** Sorts plain string options alphabetically. */
export function sortStrings(items: readonly string[]): string[] {
  return sortByLabel(items, (s) => s)
}

/** Alphabetical, with the items matching `pinned` (an "All ..." entry) kept first. */
export function sortByLabelPinned<T>(items: readonly T[], label: (item: T) => string, pinned: (item: T) => boolean): T[] {
  return [...items.filter(pinned), ...sortByLabel(items.filter((i) => !pinned(i)), label)]
}
