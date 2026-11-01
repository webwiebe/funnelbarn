// A browser driven by Playwright, Selenium or similar reports navigator.webdriver.
// Its events are test traffic whatever the page configured, so an E2E run against
// a production build does not count as production. A crawler that sets the flag
// files as test too.
export function isAutomatedBrowser(): boolean {
  return typeof navigator !== "undefined" && navigator.webdriver === true;
}

// Observe a performance entry type; browsers that do not support it throw.
export function observePerformance(type: string, onEntries: PerformanceObserverCallback): void {
  try {
    new PerformanceObserver(onEntries).observe({ type, buffered: true });
  } catch {
    // Not supported in this environment.
  }
}
