/** Measured Jimeng connection contract, canvas-rendering.3800953894 (2026-10-29). */
export const CONNECTION_FLOW_PERIOD_MS = 2600;
export const CONNECTION_FLOW_BASE = "#008EE5";
export const CONNECTION_FLOW_STOPS: ReadonlyArray<readonly [number, string]> = [
  [0, "#008EE5"],
  [0.13, "#018FE5"],
  [0.27, "#14A7EC"],
  [0.41, "#1CB1E5"],
  [0.52, "#75CCB9"],
  [0.6, "#D6B20A"],
  [0.68, "#E29618"],
  [0.71, "#E48120"],
  [0.75, "#E29618"],
  [0.8, "#D6B20A"],
  [0.85, "#75CCB9"],
  [0.95, "#18ACE9"],
  [1, "#008EE5"],
];

export function connectionFlowWindow(length: number, time: number) {
  const sweep = Math.min(312, Math.max(120, length * 0.55));
  const progress =
    (((time % CONNECTION_FLOW_PERIOD_MS) + CONNECTION_FLOW_PERIOD_MS) %
      CONNECTION_FLOW_PERIOD_MS) /
    CONNECTION_FLOW_PERIOD_MS;
  const head = progress * (length + sweep + 72);
  return { progress, sweep, head, tail: head - sweep };
}

export function connectionFlowTexture(stops = CONNECTION_FLOW_STOPS) {
  const pixels = new Uint8Array(256 * 4);
  for (let index = 0; index < 256; index++) {
    const position = index / 255;
    let next = 1;
    while (next < stops.length - 1 && position > stops[next][0]) next++;
    const [start, from] = stops[next - 1];
    const [end, to] = stops[next];
    const ratio = (position - start) / (end - start);
    for (let channel = 0; channel < 3; channel++) {
      const offset = 1 + channel * 2;
      const a = parseInt(from.slice(offset, offset + 2), 16);
      const b = parseInt(to.slice(offset, offset + 2), 16);
      pixels[index * 4 + channel] = Math.round(a + (b - a) * ratio);
    }
    pixels[index * 4 + 3] = 255;
  }
  return pixels;
}

/** World-space arc length uses the same 64/16 segment sampling as the reference. */
export function connectionFlowMesh(path: string, fanout = false) {
  const values = path.replaceAll(/[MC,]/g, " ").trim().split(/\s+/).map(Number);
  if (values.length !== 8 || values.some((value) => !Number.isFinite(value)))
    return new Float32Array();
  const [x0, y0, x1, y1, x2, y2, x3, y3] = values;
  const count = fanout ? 16 : 64;
  const points = new Float32Array((count + 1) * 3);
  points[0] = x0;
  points[1] = y0;
  let length = 0,
    previousX = x0,
    previousY = y0;
  for (let index = 1; index <= count; index++) {
    const t = index / count,
      u = 1 - t;
    const x =
      u ** 3 * x0 + 3 * u ** 2 * t * x1 + 3 * u * t ** 2 * x2 + t ** 3 * x3;
    const y =
      u ** 3 * y0 + 3 * u ** 2 * t * y1 + 3 * u * t ** 2 * y2 + t ** 3 * y3;
    length += Math.hypot(x - previousX, y - previousY);
    points.set([x, y, length], index * 3);
    previousX = x;
    previousY = y;
  }
  if (!length) return new Float32Array();
  const vertices = new Float32Array(count * 54);
  let offset = 0;
  for (let index = 0; index < count; index++) {
    for (const [along, side] of [
      [0, -1],
      [0, 1],
      [1, -1],
      [1, -1],
      [0, 1],
      [1, 1],
    ]) {
      vertices.set(
        [
          points[index * 3],
          points[index * 3 + 1],
          points[index * 3 + 3],
          points[index * 3 + 4],
          points[index * 3 + 2],
          points[index * 3 + 5],
          length,
          along,
          side,
        ],
        offset,
      );
      offset += 9;
    }
  }
  return vertices;
}
