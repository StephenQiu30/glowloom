import { expect, it } from "vitest";
import { updateConnectionGesture } from "@/components/canvas/connections/connection-gesture";

it("keeps a one-pixel move a click but starts dragging strictly beyond it", () => {
  const start = { x: 10, y: 20, dragged: false };
  expect(updateConnectionGesture(start, { x: 11, y: 20 }).dragged).toBe(false);
  expect(updateConnectionGesture(start, { x: 11.01, y: 20 }).dragged).toBe(
    true,
  );
  expect(updateConnectionGesture(start, { x: 10.8, y: 20.8 }).dragged).toBe(
    true,
  );
});
it("does not turn a drag back into a click when the pointer returns to the origin", () => {
  const start = { x: 10, y: 20, dragged: false };
  const moved = updateConnectionGesture(start, { x: 12, y: 20 });
  expect(updateConnectionGesture(moved, { x: 10, y: 20 }).dragged).toBe(true);
  expect(start.dragged).toBe(false);
});
