// @vitest-environment node
import { afterEach, expect, it, vi } from "vitest";
import { acquireTimelineWaveform } from "@/components/canvas/timeline/timeline-waveform";
afterEach(() => vi.unstubAllGlobals());
it("uses the installed demuxer and PCM decoder against an HTTP range WAV source", async () => {
  const sampleRate = 8000,
    frames = sampleRate * 2;
  const bytes = new Uint8Array(44 + frames * 2),
    view = new DataView(bytes.buffer);
  const label = (offset: number, text: string) =>
    bytes.set(new TextEncoder().encode(text), offset);
  label(0, "RIFF");
  view.setUint32(4, bytes.length - 8, true);
  label(8, "WAVE");
  label(12, "fmt ");
  view.setUint32(16, 16, true);
  view.setUint16(20, 1, true);
  view.setUint16(22, 1, true);
  view.setUint32(24, sampleRate, true);
  view.setUint32(28, sampleRate * 2, true);
  view.setUint16(32, 2, true);
  view.setUint16(34, 16, true);
  label(36, "data");
  view.setUint32(40, frames * 2, true);
  for (let frame = 0; frame < frames; frame++)
    view.setInt16(44 + frame * 2, frame < sampleRate ? 8192 : 4096, true);
  const fetcher = vi.fn(async (_url: RequestInfo | URL, init?: RequestInit) => {
    const range = new Headers(init?.headers).get("range");
    const match = /bytes=(\d+)-(\d*)/.exec(range ?? "");
    const start = match ? Number(match[1]) : 0,
      end = match?.[2]
        ? Math.min(bytes.length - 1, Number(match[2]))
        : bytes.length - 1;
    return new Response(bytes.slice(start, end + 1), {
      status: match ? 206 : 200,
      headers: {
        "Content-Type": "audio/wav",
        "Content-Length": String(end - start + 1),
        "Accept-Ranges": "bytes",
        ...(match
          ? { "Content-Range": `bytes ${start}-${end}/${bytes.length}` }
          : {}),
      },
    });
  });
  vi.stubGlobal("fetch", fetcher);
  const lease = acquireTimelineWaveform(
    "https://media.example/pcm-waveform.wav",
    2000,
  );
  try {
    const peaks = await lease.promise;
    expect(peaks).toHaveLength(128);
    expect(peaks[0]).toBe(1);
    expect(peaks[63]).toBe(1);
    expect(peaks[64]).toBe(0.5);
    expect(peaks[127]).toBe(0.5);
    expect(fetcher).toHaveBeenCalled();
  } finally {
    lease.release();
  }
});
