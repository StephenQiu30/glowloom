import { expect, it } from "vitest";
import { directorTransform } from "@/components/canvas/director/director-model";
import {
  editDirectorChannel,
  directorOwnsChannel,
} from "@/components/canvas/director/director-timeline";
it("moves, removes and curves a single channel without disturbing sibling keys", () => {
  const id = crypto.randomUUID(),
    frames = [
      { id, time: 0, easing: "linear", transform: directorTransform() },
    ];
  const moved = editDirectorChannel(frames, id, "position", { time: 2 });
  expect(moved.find((f) => f.id === id)?.time).toBe(0);
  expect(moved.find((f) => directorOwnsChannel(f, "position"))?.time).toBe(2);
  const curved = editDirectorChannel(frames, id, "rotation", {
    easing: "step",
  });
  expect(curved.find((f) => directorOwnsChannel(f, "position"))?.easing).toBe(
    "linear",
  );
  expect(curved.find((f) => directorOwnsChannel(f, "rotation"))?.easing).toBe(
    "step",
  );
  const removed = editDirectorChannel(frames, id, "scale", { remove: true });
  expect(removed).toHaveLength(1);
  expect(directorOwnsChannel(removed[0], "position")).toBe(true);
  expect(directorOwnsChannel(removed[0], "scale")).toBe(false);
});

it("rejects a collision behind an unrelated same-time key", () => {
  const base = {
    easing: "linear",
    transform: directorTransform(),
    position_keyed: false,
    rotation_keyed: false,
    scale_keyed: false,
  };
  const id = crypto.randomUUID();
  const frames = [
    { ...base, id, time: 0, position_keyed: true },
    { ...base, id: crypto.randomUUID(), time: 2, rotation_keyed: true },
    { ...base, id: crypto.randomUUID(), time: 2, position_keyed: true },
  ];
  expect(() =>
    editDirectorChannel(frames, id, "position", { time: 2 }),
  ).toThrow("已有关键帧");
});
it("moving a key preserves another channel's easing at its destination", () => {
  const base = {
    transform: directorTransform(),
    position_keyed: false,
    rotation_keyed: false,
    scale_keyed: false,
  };
  const id = crypto.randomUUID();
  const frames = [
    { ...base, id, time: 0, easing: "linear", position_keyed: true },
    {
      ...base,
      id: crypto.randomUUID(),
      time: 2,
      easing: "step",
      rotation_keyed: true,
    },
  ];
  const result = editDirectorChannel(frames, id, "position", {
    time: 2,
    easing: "smooth",
  });
  expect(result.find((f) => directorOwnsChannel(f, "rotation"))?.easing).toBe(
    "step",
  );
  expect(result.find((f) => directorOwnsChannel(f, "position"))?.easing).toBe(
    "smooth",
  );
});
