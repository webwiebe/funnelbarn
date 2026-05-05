/**
 * IIFE entry point for script-tag usage.
 *
 * Exposes a `window.funnelbarn` global with `init`, `track`, `page`, and
 * `identify` convenience functions that delegate to a shared FunnelBarnClient
 * instance. Usage:
 *
 *   <script src="/sdk/funnelbarn.js"></script>
 *   <script>
 *     funnelbarn.init({ apiKey: '...', endpoint: 'https://analytics.example.com' });
 *     funnelbarn.page();
 *     funnelbarn.track('signup', { plan: 'pro' });
 *     funnelbarn.identify('user-123');
 *   </script>
 */

import { FunnelBarnClient, FunnelBarnOptions } from "./index.js";

let _client: FunnelBarnClient | null = null;

function init(options: FunnelBarnOptions): void {
  _client = new FunnelBarnClient(options);
}

function track(name: string, properties?: Record<string, unknown>): void {
  _client?.track(name, properties);
}

function page(properties?: Record<string, unknown>): void {
  _client?.page(properties);
}

function identify(userId: string): void {
  _client?.identify(userId);
}

export { init, track, page, identify };

// Auto-init from script tag data attributes:
//   <script src="/sdk.js" data-api-key="fb_xxx"></script>
if (typeof document !== "undefined") {
  const script = document.currentScript as HTMLScriptElement | null;
  if (script) {
    const apiKey = script.getAttribute("data-api-key");
    if (apiKey) {
      const endpoint = script.getAttribute("data-endpoint") || script.src.replace(/\/sdk\.js.*$/, "");
      init({ apiKey, endpoint });
      page();
    }
  }
}
