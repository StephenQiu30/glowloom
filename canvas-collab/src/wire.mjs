import { base64, unbase64, sha256 } from "./binary.mjs";

const CHUNK_BYTES = 384 * 1024;
const MAX_BYTES = 16 * 1024 * 1024;
const MAX_DURATION = 30000;
const fail = (message) => new Error(message);

/** The logical message budget is independent of the existing 1MiB Netty frame budget. */
export async function wireFrames(message, maxBytes = MAX_BYTES) {
  const bytes = new TextEncoder().encode(JSON.stringify(message));
  if (!bytes.length || bytes.length > maxBytes) throw fail("WIRE_BUDGET");
  const id = crypto.randomUUID(),
    hash = await sha256(bytes);
  const count = Math.ceil(bytes.length / CHUNK_BYTES);
  return Array.from({ length: count }, (_, index) => ({
    type: "YJS_CHUNK",
    id,
    index,
    count,
    total: bytes.length,
    hash,
    data: base64(
      bytes.subarray(index * CHUNK_BYTES, (index + 1) * CHUNK_BYTES),
    ),
  }));
}

/** Ordered, bounded transfer; callers serialize accept() and clear on connection change. */
export class WireAssembler {
  constructor({ maxBytes = MAX_BYTES, now = Date.now } = {}) {
    this.maxBytes = maxBytes;
    this.now = now;
    this.transfer = null;
  }

  get pendingBytes() {
    return this.transfer?.total ?? 0;
  }
  clear() {
    this.transfer = null;
  }

  async accept(frame) {
    if (this.transfer && this.now() - this.transfer.started > MAX_DURATION)
      this.clear();
    try {
      if (
        !frame ||
        Object.keys(frame).sort().join(",") !==
          "count,data,hash,id,index,total,type" ||
        frame.type !== "YJS_CHUNK" ||
        !/^[a-f0-9-]{36}$/.test(frame.id) ||
        !Number.isSafeInteger(frame.index) ||
        !Number.isSafeInteger(frame.count) ||
        !Number.isSafeInteger(frame.total) ||
        !/^[a-f0-9]{64}$/.test(frame.hash) ||
        typeof frame.data !== "string"
      )
        throw fail("WIRE_FORMAT");
      if (
        frame.total < 1 ||
        frame.total > this.maxBytes ||
        frame.data.length > (CHUNK_BYTES * 4) / 3 ||
        frame.count !== Math.ceil(frame.total / CHUNK_BYTES)
      )
        throw fail("WIRE_BUDGET");
      if (!this.transfer) {
        if (frame.index !== 0) throw fail("WIRE_ORDER");
        this.transfer = {
          id: frame.id,
          total: frame.total,
          hash: frame.hash,
          count: frame.count,
          parts: [],
          started: this.now(),
        };
      }
      const transfer = this.transfer;
      if (
        frame.id !== transfer.id ||
        frame.total !== transfer.total ||
        frame.hash !== transfer.hash ||
        frame.count !== transfer.count ||
        frame.index !== transfer.parts.length
      )
        throw fail("WIRE_ORDER");
      const bytes = unbase64(frame.data);
      const expected = Math.min(
        CHUNK_BYTES,
        frame.total - frame.index * CHUNK_BYTES,
      );
      if (bytes.length !== expected) throw fail("WIRE_BUDGET");
      transfer.parts.push(bytes);
      if (transfer.parts.length < transfer.count) return null;
      const complete = new Uint8Array(transfer.total);
      transfer.parts.forEach((part, index) =>
        complete.set(part, index * CHUNK_BYTES),
      );
      this.clear();
      if ((await sha256(complete)) !== transfer.hash) throw fail("WIRE_HASH");
      return JSON.parse(
        new TextDecoder("utf8", { fatal: true }).decode(complete),
      );
    } catch (error) {
      this.clear();
      throw error;
    }
  }
}
