/**
 * FunnelBarn browser + Node.js SDK.
 *
 * Tracks page views and custom events. Batches events and flushes every
 * 5 seconds or on beforeunload. Generates a session ID in localStorage
 * with a 30-minute idle timeout (browser only).
 */

export interface FunnelBarnOptions {
  apiKey: string;
  endpoint: string;
  /** Optional project name sent as x-funnelbarn-project header */
  projectName?: string;
  /** Flush interval in ms (default: 5000) */
  flushInterval?: number;
  /** Session idle timeout in ms (default: 30 minutes) */
  sessionTimeout?: number;
}

export interface EventProperties {
  [key: string]: unknown;
}

interface EventPayload {
  name: string;
  url?: string;
  referrer?: string;
  utm_source?: string;
  utm_medium?: string;
  utm_campaign?: string;
  utm_term?: string;
  utm_content?: string;
  properties?: EventProperties;
  session_id?: string;
  user_id?: string;
  timestamp: string;
}

const SESSION_KEY = "funnelbarn_sid";
const SESSION_EXPIRY_KEY = "funnelbarn_sid_exp";
const SESSION_TIMEOUT_DEFAULT = 30 * 60 * 1000; // 30 min

/**
 * FunnelBarnClient is the main analytics client.
 */
export class FunnelBarnClient {
  private readonly apiKey: string;
  private readonly endpoint: string;
  private readonly projectName?: string;
  private readonly flushInterval: number;
  private readonly sessionTimeout: number;

  private queue: EventPayload[] = [];
  private flushTimer?: ReturnType<typeof setInterval>;
  private userId?: string;

  constructor(options: FunnelBarnOptions) {
    this.apiKey = options.apiKey;
    this.endpoint = options.endpoint.replace(/\/$/, "");
    this.projectName = options.projectName;
    this.flushInterval = options.flushInterval ?? 5000;
    this.sessionTimeout = options.sessionTimeout ?? SESSION_TIMEOUT_DEFAULT;

    this.startFlushTimer();

    // Auto-flush on page unload (browser only).
    if (typeof window !== "undefined") {
      window.addEventListener("beforeunload", () => this.flush());
    }
  }

  /**
   * Track a page view. Auto-detects URL and referrer from window.location
   * when running in a browser.
   */
  page(properties?: EventProperties): void {
    const url = this.detectURL();
    const referrer = this.detectReferrer();
    const utms = this.extractUTMs(url);

    this.queue.push({
      name: "page_view",
      url,
      referrer,
      ...utms,
      properties,
      session_id: this.getOrCreateSessionID(),
      user_id: this.userId,
      timestamp: new Date().toISOString(),
    });
  }

  /**
   * Track a custom event.
   */
  track(name: string, properties?: EventProperties): void {
    if (!name) return;
    const url = this.detectURL();
    const utms = this.extractUTMs(url);

    this.queue.push({
      name,
      url,
      ...utms,
      properties,
      session_id: this.getOrCreateSessionID(),
      user_id: this.userId,
      timestamp: new Date().toISOString(),
    });
  }

  /**
   * Associate a user ID with subsequent events.
   */
  identify(userId: string): void {
    this.userId = userId || undefined;
  }

  /**
   * Flush queued events to the server immediately.
   */
  async flush(): Promise<void> {
    if (this.queue.length === 0) return;

    const batch = this.queue.splice(0, this.queue.length);

    for (const event of batch) {
      try {
        await this.sendEvent(event);
      } catch {
        // Best-effort: silently discard on error.
      }
    }
  }

  // --------------------------------------------------------------------------

  private async sendEvent(event: EventPayload): Promise<void> {
    const url = `${this.endpoint}/api/v1/events`;
    const headers: Record<string, string> = {
      "Content-Type": "application/json",
      "x-funnelbarn-api-key": this.apiKey,
    };
    if (this.projectName) {
      headers["x-funnelbarn-project"] = this.projectName;
    }

    const body = JSON.stringify(event);

    if (typeof fetch !== "undefined") {
      // Use fetch (browser + Node 18+).
      const response = await fetch(url, {
        method: "POST",
        headers,
        body,
        keepalive: true,
      });
      if (!response.ok) {
        throw new Error(`funnelbarn: server returned ${response.status}`);
      }
    } else {
      // Node < 18: use http/https module.
      await sendWithHttp(url, headers, body);
    }
  }

  private startFlushTimer(): void {
    if (typeof setInterval !== "undefined") {
      this.flushTimer = setInterval(() => {
        this.flush().catch(() => {});
      }, this.flushInterval);

      // Allow Node.js process to exit even if timer is active.
      if (this.flushTimer && typeof this.flushTimer === "object" && "unref" in this.flushTimer) {
        (this.flushTimer as NodeJS.Timeout).unref();
      }
    }
  }

  private getOrCreateSessionID(): string {
    if (typeof localStorage === "undefined") {
      return "";
    }

    const now = Date.now();
    const expiry = parseInt(localStorage.getItem(SESSION_EXPIRY_KEY) ?? "0", 10);

    if (now < expiry) {
      // Extend session expiry on activity.
      localStorage.setItem(SESSION_EXPIRY_KEY, String(now + this.sessionTimeout));
      return localStorage.getItem(SESSION_KEY) ?? this.generateSessionID();
    }

    // New session.
    const id = this.generateSessionID();
    localStorage.setItem(SESSION_KEY, id);
    localStorage.setItem(SESSION_EXPIRY_KEY, String(now + this.sessionTimeout));
    return id;
  }

  private generateSessionID(): string {
    const bytes = new Uint8Array(16);
    if (typeof crypto !== "undefined" && crypto.getRandomValues) {
      crypto.getRandomValues(bytes);
    } else {
      for (let i = 0; i < bytes.length; i++) {
        bytes[i] = Math.floor(Math.random() * 256);
      }
    }
    return Array.from(bytes)
      .map((b) => b.toString(16).padStart(2, "0"))
      .join("");
  }

  private detectURL(): string | undefined {
    if (typeof window !== "undefined" && window.location) {
      return window.location.href;
    }
    return undefined;
  }

  private detectReferrer(): string | undefined {
    if (typeof document !== "undefined" && document.referrer) {
      return document.referrer;
    }
    return undefined;
  }

  private extractUTMs(url?: string): Partial<EventPayload> {
    if (!url) return {};
    try {
      const u = new URL(url);
      const result: Partial<EventPayload> = {};
      if (u.searchParams.get("utm_source")) result.utm_source = u.searchParams.get("utm_source")!;
      if (u.searchParams.get("utm_medium")) result.utm_medium = u.searchParams.get("utm_medium")!;
      if (u.searchParams.get("utm_campaign")) result.utm_campaign = u.searchParams.get("utm_campaign")!;
      if (u.searchParams.get("utm_term")) result.utm_term = u.searchParams.get("utm_term")!;
      if (u.searchParams.get("utm_content")) result.utm_content = u.searchParams.get("utm_content")!;
      return result;
    } catch {
      return {};
    }
  }
}

// --------------------------------------------------------------------------
// Node < 18 HTTP fallback
// --------------------------------------------------------------------------

function sendWithHttp(
  url: string,
  headers: Record<string, string>,
  body: string
): Promise<void> {
  return new Promise((resolve, reject) => {
    try {
      // eslint-disable-next-line @typescript-eslint/no-var-requires
      const protocol = url.startsWith("https") ? require("https") : require("http");
      const parsed = new URL(url);
      const req = protocol.request(
        {
          hostname: parsed.hostname,
          port: parsed.port,
          path: parsed.pathname,
          method: "POST",
          headers,
        },
        (res: { statusCode: number }) => {
          if (res.statusCode && res.statusCode >= 400) {
            reject(new Error(`funnelbarn: server returned ${res.statusCode}`));
          } else {
            resolve();
          }
        }
      );
      req.on("error", reject);
      req.write(body);
      req.end();
    } catch (err) {
      reject(err);
    }
  });
}
