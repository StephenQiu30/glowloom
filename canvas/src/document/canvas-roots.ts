import * as Y from "yjs";

const prefix = "__tiptap_position_migrations:item:";
const number = (value: unknown): value is number =>
  Number.isSafeInteger(value) && (value as number) >= 0;
const invalid = (code = "INVALID_EDITOR_METADATA"): never => {
  throw Object.assign(new Error(code), { code, definitive: true });
};
const shape = (value: any, required: string[], optional: string[] = []): boolean =>
  value !== null &&
  typeof value === "object" &&
  Object.getPrototypeOf(value) === Object.prototype &&
  required.every((key) => Object.hasOwn(value, key)) &&
  Object.keys(value).every(
    (key) => required.includes(key) || optional.includes(key),
  );
const boundary = (value: unknown) =>
  value === undefined || value === "start" || value === "end";

/** The pinned y-tiptap plugin stores position transfers outside the business root. */
export function validateCanvasRoots(doc: Y.Doc) {
  const roots = [...doc.share.keys()].filter((name) => name !== "canvas");
  if (!roots.length) return;
  const clocks = Y.decodeStateVector(Y.encodeStateVector(doc));
  let budget = 20000;
  const spend = (amount = 1) => {
    budget -= amount;
    if (budget < 0) invalid();
  };
  const integrated = (id: any, length = 0) =>
    shape(id, ["client", "clock"], ["length", "boundary"]) &&
    number(id.client) &&
    number(id.clock) &&
    id.clock + length <= (clocks.get(id.client) ?? -1);
  for (const name of roots) {
    const match =
      /^__tiptap_position_migrations:item:(0|[1-9]\d*):(0|[1-9]\d*)$/.exec(
        name,
      );
    if (!match) return invalid("INVALID_ROOT");
    const client = Number(match[1]),
      clock = Number(match[2]);
    if (!number(client) || !number(clock) || !integrated({ client, clock }, 1))
      invalid();
    spend();
  }
  // A sequence under a metadata name must not become invisible when read as a map.
  for (const item of Y.decodeUpdate(Y.encodeStateAsUpdate(doc)).structs) {
    if (!(item instanceof Y.Item)) continue;
    // Decoded updates can identify a root by name before Yjs resolves the parent.
    const parent: unknown = item.parent;
    if (
      typeof parent === "string" &&
      parent.startsWith(prefix) &&
      item.parentSub === null
    )
      invalid();
  }
  for (const name of roots) {
    let store: Y.Map<any> = null as any;
    try {
      store = doc.getMap(name) as Y.Map<any>;
    } catch {
      invalid();
    }
    for (const [key, entry] of store) {
      spend();
      if (
        !shape(entry, ["version", "client", "clock", "seen", "transfers"]) ||
        entry.version !== 1 ||
        !integrated({ client: entry.client, clock: entry.clock }, 1) ||
        key !== `${entry.client}:${entry.clock}` ||
        !Array.isArray(entry.seen) ||
        !Array.isArray(entry.transfers) ||
        !entry.transfers.length
      )
        invalid();
      spend(entry.seen.length + entry.transfers.length);
      const seen = new Set();
      for (const pair of entry.seen) {
        if (
          !Array.isArray(pair) ||
          pair.length !== 2 ||
          !number(pair[0]) ||
          !number(pair[1]) ||
          seen.has(pair[0]) ||
          pair[1] > (clocks.get(pair[0]) ?? -1)
        )
          invalid();
        seen.add(pair[0]);
      }
      for (const transfer of entry.transfers) {
        if (!shape(transfer, ["source", "target"])) invalid();
        const { source, target } = transfer;
        if (
          !shape(source, ["client", "clock", "length"], ["boundary"]) ||
          !number(source.length) ||
          !source.length ||
          !integrated(source, source.length) ||
          !boundary(source.boundary)
        )
          invalid();
        if (target === null) continue;
        if (
          !shape(
            target,
            ["client", "clock", "type"],
            ["tname", "assoc", "boundary"],
          ) ||
          !integrated({ client: target.client, clock: target.clock }) ||
          !boundary(target.boundary) ||
          (target.assoc !== undefined &&
            target.assoc !== -1 &&
            target.assoc !== 0) ||
          (target.tname !== undefined &&
            (typeof target.tname !== "string" || target.tname.length > 256)) ||
          (target.type !== null &&
            (!shape(target.type, ["client", "clock"]) ||
              !integrated(target.type, 1)))
        )
          invalid();
      }
    }
  }
}
