import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";
import { startConfetti } from "./confetti";
import { startConstellation } from "./constellation";

// The drawings run against a 2D context that records what it is asked, and frames that run when the test says.

type Calls = Record<string, number>;

function stubContext(): Calls {
  const calls: Calls = {};
  const context = new Proxy({} as Record<string, unknown>, {
    get: (target, key: string) =>
      key in target
        ? target[key]
        : (...args: unknown[]) => {
            calls[key] = (calls[key] ?? 0) + 1;
            return args;
          },
    set: (target, key: string, value) => {
      target[key] = value;
      return true;
    },
  });
  vi.spyOn(HTMLCanvasElement.prototype, "getContext").mockReturnValue(context as unknown as CanvasRenderingContext2D);
  return calls;
}

let frames: Map<number, FrameRequestCallback>;
let nextFrame: number;

function runFrame(now: number) {
  const pending = [...frames.entries()];
  frames.clear();
  for (const [, callback] of pending) callback(now);
}

function stillness(asked: boolean) {
  vi.stubGlobal("matchMedia", (query: string) => ({ matches: asked && query.includes("reduce"), media: query }));
}

beforeEach(() => {
  frames = new Map();
  nextFrame = 1;
  vi.stubGlobal("requestAnimationFrame", (callback: FrameRequestCallback) => {
    frames.set(nextFrame, callback);
    return nextFrame++;
  });
  vi.stubGlobal("cancelAnimationFrame", (id: number) => frames.delete(id));
  stillness(false);
});

afterEach(() => {
  vi.unstubAllGlobals();
  vi.restoreAllMocks();
});

// The test DOM has no PointerEvent; a mouse event of the same type carries what the confetti reads.
const pointer = (init: MouseEventInit) => new MouseEvent("pointerdown", init);

function hostedCanvas(width: number, height: number) {
  const host = document.createElement("main");
  host.dataset.backdropHost = "";
  Object.defineProperty(host, "clientWidth", { value: width });
  Object.defineProperty(host, "clientHeight", { value: height });
  const canvas = document.createElement("canvas");
  host.appendChild(canvas);
  document.body.appendChild(host);
  return { host, canvas };
}

describe("the constellation", () => {
  it("fills its column with points, joins the near ones, and keeps moving until stopped", async () => {
    const calls = stubContext();
    const { host, canvas } = hostedCanvas(1200, 900);
    const stop = startConstellation(canvas);
    expect(canvas.style.width).toBe("1200px");
    expect(calls.arc).toBeGreaterThan(50);
    expect(calls.stroke).toBeGreaterThan(0);

    const before = calls.arc ?? 0;
    runFrame(1000);
    runFrame(1016);
    runFrame(3000);
    expect(calls.arc).toBeGreaterThan(before);
    // The column reports its size once observed, which seeds the points again without a second animation.
    await Promise.resolve();
    expect(frames.size).toBe(1);

    stop();
    expect(frames.size).toBe(0);
    host.remove();
  });

  it("draws one frame and stands still for whoever asked for less motion, and nothing in a hidden tab", async () => {
    const calls = stubContext();
    stillness(true);
    const { host, canvas } = hostedCanvas(400, 300);
    const stop = startConstellation(canvas);
    expect(calls.clearRect).toBe(1);
    expect(frames.size).toBe(0);
    await Promise.resolve();
    expect(calls.clearRect).toBe(2);
    document.dispatchEvent(new Event("visibilitychange"));
    expect(frames.size).toBe(0);
    stop();
    host.remove();

    stillness(false);
    const again = hostedCanvas(400, 300);
    const stopAgain = startConstellation(again.canvas);
    Object.defineProperty(document, "hidden", { value: true, configurable: true });
    runFrame(10);
    expect(frames.size).toBe(0);
    Object.defineProperty(document, "hidden", { value: false, configurable: true });
    document.dispatchEvent(new Event("visibilitychange"));
    expect(frames.size).toBe(1);
    stopAgain();
    again.host.remove();
  });

  it("does nothing without a canvas to draw on", () => {
    vi.spyOn(HTMLCanvasElement.prototype, "getContext").mockReturnValue(null);
    expect(() => startConstellation(document.createElement("canvas"))()).not.toThrow();
  });
});

describe("the confetti", () => {
  it("throws a burst where the page is clicked, draws it until it has fallen, and stops listening", () => {
    const calls = stubContext();
    const canvas = document.createElement("canvas");
    const stop = startConfetti(canvas);
    window.dispatchEvent(new Event("resize"));

    window.dispatchEvent(pointer({ button: 2, clientX: 5, clientY: 5 }));
    expect(frames.size).toBe(0);
    window.dispatchEvent(pointer({ button: 0, clientX: 100, clientY: 80 }));
    expect(frames.size).toBe(1);
    const now = performance.now();
    runFrame(now + 16);
    runFrame(now + 32);
    expect(calls.fillRect).toBeGreaterThan(0);
    expect(calls.fillText).toBeGreaterThan(0);
    runFrame(now + 5000);
    expect(frames.size).toBe(0);

    window.dispatchEvent(pointer({ button: 0, clientX: 1, clientY: 1 }));
    stop();
    expect(frames.size).toBe(0);
    window.dispatchEvent(pointer({ button: 0, clientX: 1, clientY: 1 }));
    expect(frames.size).toBe(0);
  });

  it("throws nothing for whoever asked for less motion", () => {
    stubContext();
    stillness(true);
    const stop = startConfetti(document.createElement("canvas"));
    window.dispatchEvent(pointer({ button: 0, clientX: 1, clientY: 1 }));
    expect(frames.size).toBe(0);
    stop();
  });
});
