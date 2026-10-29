import { afterEach, beforeEach, expect, it, vi } from "vitest";
import { acquireTimelineWaveform } from "@/components/canvas/timeline/timeline-waveform";
const api = vi.hoisted(() => ({
  source: vi.fn(),
  input: vi.fn(),
  dispose: vi.fn(),
  canDecode: vi.fn(),
  samples: vi.fn(),
  duration: vi.fn(),
  track: vi.fn(),
}));
vi.mock("mediabunny", () => ({
  ALL_FORMATS: ["formats"],
  UrlSource: class {
    constructor(url: string, options: unknown) {
      api.source(url, options);
    }
  },
  Input: class {
    constructor(options: unknown) {
      api.input(options);
    }
    getPrimaryAudioTrack = api.track;
    dispose = api.dispose;
  },
  AudioSampleSink: class {
    samples = api.samples;
  },
}));
const leases: Array<{ release(): void }> = [];
function acquire(url = "https://media.example/audio", durationMS = 4000) {
  const lease = acquireTimelineWaveform(url, durationMS);
  leases.push(lease);
  return lease;
}
function sample(timestamp: number, amplitude: number, frames = 8193) {
  return {
    timestamp,
    sampleRate: 48000,
    numberOfFrames: frames,
    numberOfChannels: 2,
    close: vi.fn(),
    copyTo: vi.fn(
      (
        destination: Float32Array,
        options: { frameCount: number; planeIndex: number },
      ) => {
        expect(destination.length).toBeLessThanOrEqual(4096);
        destination.fill(
          options.planeIndex === 0 ? amplitude / 2 : amplitude,
          0,
          options.frameCount,
        );
      },
    ),
  };
}
beforeEach(() => {
  vi.clearAllMocks();
  api.track.mockResolvedValue({
    canDecode: api.canDecode,
    computeDuration: api.duration,
  });
  api.canDecode.mockResolvedValue(true);
  api.duration.mockResolvedValue(4);
  api.samples.mockImplementation(async function* () {
    yield sample(0, 0.5);
    yield sample(2, 0.25);
  });
});
afterEach(() => {
  leases.splice(0).forEach((lease) => lease.release());
  vi.restoreAllMocks();
});
it("streams long sources with bounded network cache and scratch buffers, closing samples before advancing", async () => {
  const long = [sample(0, 0.5), sample(18000, 0.25), sample(35999, 0.125)];
  api.samples.mockImplementation(async function* () {
    for (let i = 0; i < long.length; i++) {
      if (i) expect(long[i - 1].close).toHaveBeenCalledOnce();
      yield long[i];
    }
  });
  const lease = acquire(undefined, 36000000);
  const peaks = await lease.promise;
  expect(peaks).toHaveLength(128);
  expect(peaks[0]).toBe(1);
  expect(peaks[64]).toBe(0.5);
  expect(peaks[127]).toBe(0.25);
  expect(api.source).toHaveBeenCalledWith(
    expect.any(String),
    expect.objectContaining({ maxCacheSize: 2 * 1024 * 1024, parallelism: 1 }),
  );
  expect(long.every((chunk) => chunk.close.mock.calls.length === 1)).toBe(true);
  expect(api.dispose).toHaveBeenCalledOnce();
});
it("shares work for card and clips, retains 128 peaks and normalizes silence safely", async () => {
  api.samples.mockImplementation(async function* () {
    yield sample(0, 0);
  });
  const a = acquire(),
    b = acquire();
  const [first, second] = await Promise.all([a.promise, b.promise]);
  expect(first).toBe(second);
  expect(first.every((value) => value === 0)).toBe(true);
  expect(api.input).toHaveBeenCalledOnce();
  a.release();
  const c = acquire();
  expect(await c.promise).toBe(first);
  expect(api.input).toHaveBeenCalledOnce();
});
it("disposes an active input on final release and ignores late results", async () => {
  let resume!: () => void;
  const chunk = sample(0, 0.5);
  api.samples.mockImplementation(async function* () {
    await new Promise<void>((resolve) => {
      resume = resolve;
    });
    yield chunk;
  });
  const a = acquire(),
    b = acquire();
  const results = Promise.allSettled([a.promise, b.promise]);
  await vi.waitFor(() => expect(resume).toBeTypeOf("function"));
  a.release();
  expect(api.dispose).not.toHaveBeenCalled();
  b.release();
  expect(api.dispose).toHaveBeenCalledOnce();
  resume();
  expect((await results).every((result) => result.status === "rejected")).toBe(
    true,
  );
  expect(chunk.close).toHaveBeenCalledOnce();
});
it("limits active sources to two and skips cancelled queued jobs", async () => {
  const pending: Array<() => void> = [];
  api.samples.mockImplementation(async function* () {
    await new Promise<void>((resolve) => pending.push(resolve));
    yield sample(0, 0.5);
  });
  const a = acquire("a"),
    b = acquire("b"),
    c = acquire("c");
  const results = Promise.allSettled([a.promise, b.promise, c.promise]);
  await vi.waitFor(() => expect(pending).toHaveLength(2));
  expect(api.input).toHaveBeenCalledTimes(2);
  c.release();
  pending.forEach((resolve) => resolve());
  await results;
  expect(api.input).toHaveBeenCalledTimes(2);
});
it("releases failed sources and allows an explicit retry without a whole-file fallback", async () => {
  api.canDecode.mockResolvedValueOnce(false);
  const failed = acquire();
  await expect(failed.promise).rejects.toThrow("unsupported");
  failed.release();
  expect(api.dispose).toHaveBeenCalledOnce();
  await expect(acquire().promise).resolves.toHaveLength(128);
  expect(api.input).toHaveBeenCalledTimes(2);
});
it("closes the current sample and input if copying PCM fails", async () => {
  const chunk = sample(0, 0.5);
  chunk.copyTo.mockImplementation(() => {
    throw new Error("copy failed");
  });
  api.samples.mockImplementation(async function* () {
    yield chunk;
  });
  await expect(acquire().promise).rejects.toThrow("copy failed");
  expect(chunk.close).toHaveBeenCalledOnce();
  expect(api.dispose).toHaveBeenCalledOnce();
});
it("frees a decoder slot immediately on cancellation even if its read never settles", async () => {
  let calls = 0;
  api.samples.mockImplementation(async function* () {
    if (++calls === 1) await new Promise<void>(() => {});
    yield sample(0, 0.5);
  });
  const a = acquire("hang"),
    b = acquire("normal"),
    c = acquire("queued");
  const rejected = expect(a.promise).rejects.toBeDefined();
  await vi.waitFor(() => expect(calls).toBeGreaterThanOrEqual(2));
  a.release();
  await rejected;
  await expect(b.promise).resolves.toHaveLength(128);
  await expect(c.promise).resolves.toHaveLength(128);
});
it("times out stalled reads without imposing a duration limit on active long audio", async () => {
  vi.useFakeTimers();
  api.track.mockImplementationOnce(() => new Promise(() => {}));
  const stalled = acquire("stalled");
  const rejected = expect(stalled.promise).rejects.toThrow("timed out");
  try {
    await vi.advanceTimersByTimeAsync(30001);
    await rejected;
    expect(api.dispose).toHaveBeenCalledOnce();
    await expect(acquire("next", 36000000).promise).resolves.toHaveLength(128);
  } finally {
    vi.useRealTimers();
  }
});
