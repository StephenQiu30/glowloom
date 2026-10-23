import assert from "node:assert/strict";
import test from "node:test";
import { WireAssembler, wireFrames } from "../src/wire.mjs";

test("binary bootstrap travels within the existing frame budget and is exposed only when complete", async () => {
  const message = { type: "SYNC_STATE", update: "原文😀".repeat(140000) };
  const frames = await wireFrames(message);
  assert.ok(frames.length > 1);
  const assembler = new WireAssembler();
  for (const frame of frames.slice(0, -1)) {
    assert.ok(Buffer.byteLength(JSON.stringify(frame)) < 1024 * 1024);
    assert.equal(await assembler.accept(frame), null);
  }
  assert.deepEqual(await assembler.accept(frames.at(-1)), message);
  assert.equal(assembler.pendingBytes, 0);
});

test("corrupt, reordered and oversized transfers never expose partial content", async () => {
  const frames = await wireFrames({ update: "a".repeat(500000) });
  const assembler = new WireAssembler();
  await assert.rejects(assembler.accept(frames[1]), /WIRE_ORDER/);
  assert.equal(assembler.pendingBytes, 0);
  await assembler.accept(frames[0]);
  await assert.rejects(
    assembler.accept({ ...frames[1], data: "b" + frames[1].data.slice(1) }),
    /WIRE_HASH/,
  );
  assert.equal(assembler.pendingBytes, 0);
  await assert.rejects(
    new WireAssembler({ maxBytes: 100 }).accept(frames[0]),
    /WIRE_BUDGET/,
  );
});

test("expired transfers release reservations and a new transfer can begin", async () => {
  let now = 0;
  const frames = await wireFrames({ update: "a".repeat(500000) });
  const assembler = new WireAssembler({ now: () => now });
  await assembler.accept(frames[0]);
  now = 31000;
  await assert.rejects(assembler.accept(frames[1]), /WIRE_ORDER/);
  assert.equal(assembler.pendingBytes, 0);
  await assembler.accept(frames[0]);
  assert.deepEqual(await assembler.accept(frames[1]), {
    update: "a".repeat(500000),
  });
});
