// EngagementTracker fires once per page view, after 15 seconds on the page or
// once the visitor has scrolled past half of it, whichever comes first.
export class EngagementTracker {
  private timer?: ReturnType<typeof setTimeout>;
  private scrollHandler?: () => void;
  private fired = false;

  constructor(private readonly onEngaged: (url: string | undefined, pageViewId: string | undefined) => void) {}

  start(url: string | undefined, pageViewId: string | undefined): void {
    if (typeof window === "undefined") return;

    this.fired = false;

    const fire = () => {
      if (this.fired) return;
      this.fired = true;
      this.cleanup();
      this.onEngaged(url, pageViewId);
    };

    // 15-second wall-clock timer using 1s ticks.
    let ticks = 0;
    const tick = () => {
      if (this.fired) return;
      ticks++;
      if (ticks >= 15) {
        fire();
      } else {
        this.timer = setTimeout(tick, 1000);
      }
    };
    this.timer = setTimeout(tick, 1000);

    // Passive scroll listener: fires when scrolled past 50%.
    const scrollHandler = () => {
      const scrollable =
        document.documentElement.scrollHeight -
        document.documentElement.clientHeight;
      if (scrollable <= 0) return;
      if (window.scrollY / scrollable >= 0.5) {
        fire();
      }
    };
    this.scrollHandler = scrollHandler;
    window.addEventListener("scroll", scrollHandler, { passive: true });
  }

  cleanup(): void {
    if (this.timer !== undefined) {
      clearTimeout(this.timer);
      this.timer = undefined;
    }
    if (this.scrollHandler !== undefined && typeof window !== "undefined") {
      window.removeEventListener("scroll", this.scrollHandler);
      this.scrollHandler = undefined;
    }
  }
}
