/**
 * Assemble rrweb chunks into a single ordered, validated event stream and build
 * a self-contained replay HTML page.
 */

export interface RRWebEvent {
  type: number;
  timestamp: number;
  data?: unknown;
}

export interface AssembleResult {
  events: RRWebEvent[];
  hasSnapshot: boolean;
  startTimestamp: number;
  endTimestamp: number;
  durationMs: number;
}

/**
 * Order events by timestamp and report whether the stream is replayable (rrweb
 * needs a full-snapshot event, type 2). Chunks can arrive out of order, so we
 * always sort; a stable sort keeps same-timestamp events in arrival order.
 */
export function assembleEvents(raw: unknown[]): AssembleResult {
  const events = (raw as RRWebEvent[])
    .filter((e) => e && typeof e.type === "number" && typeof e.timestamp === "number")
    .slice()
    .sort((a, b) => a.timestamp - b.timestamp);

  const hasSnapshot = events.some((e) => e.type === 2);
  const startTimestamp = events.length ? events[0].timestamp : 0;
  const endTimestamp = events.length ? events[events.length - 1].timestamp : 0;

  return {
    events,
    hasSnapshot,
    startTimestamp,
    endTimestamp,
    durationMs: endTimestamp - startTimestamp,
  };
}

export interface BuildHTMLOptions {
  events: RRWebEvent[];
  /** Milliseconds from recording start to auto-seek to (the trace moment). */
  seekMs: number;
  /** rrweb-player UMD JS source to inline (keeps the page offline). */
  playerJs: string;
  /** rrweb-player CSS to inline. */
  playerCss: string;
  /** Optional banner text (trace id, url) shown above the player. */
  banner?: string;
}

/**
 * Build a fully self-contained HTML page that renders the recording with
 * rrweb-player and auto-seeks to the trace moment. Everything is inlined so the
 * page works offline inside a headless browser.
 */
export function buildReplayHTML(opts: BuildHTMLOptions): string {
  const eventsJson = JSON.stringify(opts.events);
  const seek = Math.max(0, Math.floor(opts.seekMs));
  const banner = opts.banner ? escapeHtml(opts.banner) : "";
  return `<!doctype html>
<html>
<head>
<meta charset="utf-8" />
<title>FunnelBarn replay</title>
<style>${opts.playerCss}</style>
<style>
  body { margin: 0; background: #1a1a1a; color: #eee; font-family: ui-monospace, monospace; }
  #banner { padding: 8px 12px; background: #111; font-size: 13px; border-bottom: 1px solid #333; }
  #marker { color: #ffb454; }
  #player { display: flex; justify-content: center; padding: 16px; }
</style>
</head>
<body>
<div id="banner">${banner} <span id="marker">▶ seeking to +${seek}ms</span></div>
<div id="player"></div>
<script>${opts.playerJs}</script>
<script>
  (function () {
    var events = ${eventsJson};
    var seekMs = ${seek};
    if (!events.length) {
      document.getElementById('marker').textContent = 'no events to replay';
      return;
    }
    // rrweb-player UMD exposes the constructor as window.rrwebPlayer.
    var PlayerCtor = window.rrwebPlayer || (window.rrweb && window.rrweb.Player);
    var player = new PlayerCtor({
      target: document.getElementById('player'),
      props: { events: events, autoPlay: true, showController: true, width: 1024, height: 640 },
    });
    // Jump straight to the trace moment so the developer lands where it happened.
    try { player.goto(seekMs); } catch (e) { /* older player API */ }
    window.__fbReplayReady = true;
  })();
</script>
</body>
</html>`;
}

function escapeHtml(s: string): string {
  return s
    .replace(/&/g, "&amp;")
    .replace(/</g, "&lt;")
    .replace(/>/g, "&gt;")
    .replace(/"/g, "&quot;");
}
