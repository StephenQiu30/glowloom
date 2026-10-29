type WaveformEntry = {
  consumers: number;
  abort: AbortController;
  promise: Promise<number[]>;
};
let decoderLibrary: Promise<typeof import("mediabunny")> | undefined;
const waveforms = new Map<string, WaveformEntry>();
const queue: Array<() => void> = [];
let decoding = 0;
function drain() {
  while (decoding < 2 && queue.length) {
    decoding++;
    queue.shift()!();
  }
}

async function streamWaveform(
  url: string,
  durationMS: number | undefined,
  signal: AbortSignal,
) {
  const { Input, UrlSource, ALL_FORMATS, AudioSampleSink } =
    await (decoderLibrary ??= import("mediabunny"));
  signal.throwIfAborted();
  const input = new Input({
    formats: ALL_FORMATS,
    source: new UrlSource(url, {
      maxCacheSize: 2 * 1024 * 1024,
      parallelism: 1,
      handleUnhandledError: () => dispose(),
      getRetryDelay: (attempts) => (attempts < 2 ? 0.5 : null),
    }),
  });
  let disposed = false;
  const dispose = () => {
    if (disposed) return;
    disposed = true;
    input.dispose();
  };
  signal.addEventListener("abort", dispose, { once: true });
  function read<T>(pending: Promise<T>, late?: (value: T) => void): Promise<T> {
    return new Promise<T>((resolve, reject) => {
      let finished = false;
      const cleanup = () => {
        clearTimeout(timer);
        signal.removeEventListener("abort", cancelled);
      };
      const fail = (error: unknown) => {
        if (finished) return;
        finished = true;
        cleanup();
        dispose();
        reject(error);
      };
      const cancelled = () => fail(signal.reason ?? new Error("cancelled"));
      const timer = setTimeout(
        () => fail(new Error("Audio waveform read timed out")),
        30000,
      );
      signal.addEventListener("abort", cancelled, { once: true });
      void pending.then((value) => {
        if (finished) {
          late?.(value);
          return;
        }
        finished = true;
        cleanup();
        resolve(value);
      }, fail);
      if (signal.aborted) cancelled();
    });
  }
  try {
    const track = await read(input.getPrimaryAudioTrack());
    signal.throwIfAborted();
    if (!track || !(await read(track.canDecode())))
      throw new Error("Audio waveform codec unsupported");
    const duration =
      durationMS && Number.isFinite(durationMS) && durationMS > 0
        ? durationMS / 1000
        : await read(track.computeDuration());
    if (!Number.isFinite(duration) || duration <= 0)
      throw new Error("Audio duration unavailable");
    signal.throwIfAborted();
    const values = new Float32Array(128);
    const scratch = new Float32Array(4096);
    let blocks = 0;
    // The sink applies decoder backpressure. Never retain a decoded sample after
    // advancing, and copy in fixed-size pieces even for large PCM packets.
    const samples = new AudioSampleSink(track).samples();
    try {
      while (true) {
        const next = await read(samples.next(), (result) => {
          if (!result.done) result.value.close();
        });
        if (next.done) break;
        const sample = next.value;
        try {
          signal.throwIfAborted();
          for (let channel = 0; channel < sample.numberOfChannels; channel++) {
            for (
              let offset = 0;
              offset < sample.numberOfFrames;
              offset += scratch.length
            ) {
              if (++blocks % 64 === 0) {
                await new Promise<void>((resolve) => setTimeout(resolve, 0));
                signal.throwIfAborted();
              }
              const count = Math.min(
                scratch.length,
                sample.numberOfFrames - offset,
              );
              sample.copyTo(scratch, {
                planeIndex: channel,
                format: "f32-planar",
                frameOffset: offset,
                frameCount: count,
              });
              for (let index = 0; index < count; index++) {
                const time =
                  sample.timestamp + (offset + index) / sample.sampleRate;
                if (time < 0) continue;
                const bucket = Math.min(
                  127,
                  Math.floor((time / duration) * 128),
                );
                const amplitude = Math.abs(scratch[index]);
                if (Number.isFinite(amplitude))
                  values[bucket] = Math.max(values[bucket], amplitude);
              }
            }
          }
        } finally {
          sample.close();
        }
      }
    } finally {
      // Dispose interrupts a pending read; do not await a stalled iterator's return.
      void samples.return().catch(() => {});
    }
    signal.throwIfAborted();
    const maximum = Math.max(...values);
    return Array.from(values, (value) => (maximum > 0 ? value / maximum : 0));
  } finally {
    signal.removeEventListener("abort", dispose);
    dispose();
  }
}

/** Share only normalized source-wide peaks, never decoded PCM or persistent URLs. */
export function acquireTimelineWaveform(url: string, durationMS?: number) {
  const key = JSON.stringify([url, durationMS]);
  let entry = waveforms.get(key);
  if (!entry) {
    const owned: WaveformEntry = {
      consumers: 0,
      abort: new AbortController(),
      promise: Promise.resolve([]),
    };
    owned.promise = (async () => {
      await new Promise<void>((resolve) => {
        queue.push(resolve);
        drain();
      });
      try {
        owned.abort.signal.throwIfAborted();
        return await streamWaveform(url, durationMS, owned.abort.signal);
      } finally {
        decoding--;
        drain();
      }
    })().catch((error: unknown) => {
      if (waveforms.get(key) === owned) waveforms.delete(key);
      throw error;
    });
    entry = owned;
    waveforms.set(key, entry);
  }
  entry.consumers++;
  const owned = entry;
  let released = false;
  return {
    promise: owned.promise,
    release() {
      if (released) return;
      released = true;
      if (--owned.consumers) return;
      owned.abort.abort();
      if (waveforms.get(key) === owned) waveforms.delete(key);
    },
  };
}
