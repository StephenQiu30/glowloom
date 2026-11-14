export type Cue = {
  startMs: number;
  endMs: number;
  blockId: string;
  textStart: number;
  textEnd: number;
};
export type Segment = {
  id: string;
  src: string;
  sha256: string;
  durationMs: number;
  cues: Cue[];
};
export type NarrationManifest = {
  documentId: string;
  revision: string;
  scope: string;
  configDigest: string;
  precision: "sentence" | "paragraph" | "segment" | "none";
  segments: Segment[];
};
export function rangeForChapter(
  manifest: NarrationManifest,
  blocks: string[],
): {
  fromSegment: number;
  fromMs: number;
  toSegment: number;
  toMs: number;
} | null {
  const ids = new Set(blocks);
  let range: null | {
    fromSegment: number;
    fromMs: number;
    toSegment: number;
    toMs: number;
  } = null;
  manifest.segments.forEach((segment, index) => {
    for (const cue of segment.cues) {
      if (!ids.has(cue.blockId)) continue;
      if (!range)
        range = {
          fromSegment: index,
          fromMs: cue.startMs,
          toSegment: index,
          toMs: cue.endMs,
        };
      else {
        range.toSegment = index;
        range.toMs = cue.endMs;
      }
    }
  });
  return range;
}
export function validateManifest(
  m: NarrationManifest,
  doc: { id: string; revision: string; blocks: { id: string; text: string }[] },
  expectedScope?: string,
) {
  if (m.documentId !== doc.id || m.revision !== doc.revision)
    throw new Error("Narration source stale 过期");
  if (
    !m.scope ||
    !m.configDigest ||
    !Array.isArray(m.segments) ||
    !m.segments.length ||
    !["sentence", "paragraph", "segment", "none"].includes(m.precision)
  )
    throw new Error("Invalid manifest");
  if (expectedScope && m.scope !== expectedScope)
    throw new Error("Narration scope mismatch");
  const blocks = new Map(doc.blocks.map((b) => [b.id, b.text]));
  const ids = new Set<string>();
  for (const segment of m.segments) {
    if (
      ids.has(segment.id) ||
      !segment.id ||
      !/^[\w/-]+\.(wav|mp3|m4a|ogg|opus)$/.test(segment.src) ||
      segment.src.includes("..") ||
      segment.src.startsWith("/") ||
      !Number.isFinite(segment.durationMs) ||
      segment.durationMs <= 0 ||
      !/^[a-f0-9]{64}$/.test(segment.sha256)
    )
      throw new Error("Invalid audio segment");
    ids.add(segment.id);
    let end = 0;
    for (const cue of segment.cues) {
      const text = blocks.get(cue.blockId);
      if (
        text === undefined ||
        !Number.isFinite(cue.startMs) ||
        !Number.isFinite(cue.endMs) ||
        cue.startMs < end ||
        cue.endMs <= cue.startMs ||
        cue.endMs > segment.durationMs ||
        cue.textStart < 0 ||
        cue.textEnd <= cue.textStart ||
        cue.textEnd > text.length
      )
        throw new Error("Invalid cue mapping");
      end = cue.endMs;
    }
  }
}
export function cueAt(segment: Segment, seconds: number): Cue | null {
  const ms = seconds * 1000;
  return segment.cues.find((c) => c.startMs <= ms && ms < c.endMs) ?? null;
}
export function planSegments(
  texts: { id: string; textDigest: string }[],
  config: string,
  cache: {
    id: string;
    textDigest: string;
    configDigest: string;
    status: string;
  }[],
): { id: string; action: string }[] {
  return texts.map((t) => {
    const existing = cache.find(
      (c) =>
        c.id === t.id &&
        c.textDigest === t.textDigest &&
        c.configDigest === config,
    );
    return {
      id: t.id,
      action:
        existing?.status === "ready"
          ? "reuse"
          : cache.some(
                (c) =>
                  c.id === t.id && ["unknown", "running"].includes(c.status),
              )
            ? "reconcile"
            : "generate",
    };
  });
}
