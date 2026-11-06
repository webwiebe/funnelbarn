// rrweb v2's raw Replayer sizes its iframe to the recorded viewport and does
// not scale, so the player scales the wrapper itself.

export interface RecordedSize {
  width: number
  height: number
  // fallback is true when the recording's Meta event gave no usable size.
  fallback: boolean
}

export const FALLBACK_RECORDED_SIZE = { width: 1280, height: 720 }

function positive(n: unknown): n is number {
  return typeof n === 'number' && Number.isFinite(n) && n > 0
}

// recordedSize reads the viewport from the first Meta event (type 4). A
// missing event, or a width or height that is 0, negative or not a number,
// gives the fallback size, since dividing by it would make the scale infinite
// and the browser would drop the transform.
export function recordedSize(events: unknown[]): RecordedSize {
  // eslint-disable-next-line @typescript-eslint/no-explicit-any
  const meta = events.find((e: any) => e?.type === 4) as any
  const width = meta?.data?.width
  const height = meta?.data?.height
  if (positive(width) && positive(height)) return { width, height, fallback: false }
  return { ...FALLBACK_RECORDED_SIZE, fallback: true }
}

// replayScale fits the recorded viewport inside the container. A container
// with no size yet gives 1; the ResizeObserver applies the real scale once
// the container is laid out.
export function replayScale(containerWidth: number, containerHeight: number, size: RecordedSize): number {
  const scale = Math.min(containerWidth / size.width, containerHeight / size.height)
  return positive(scale) ? scale : 1
}
