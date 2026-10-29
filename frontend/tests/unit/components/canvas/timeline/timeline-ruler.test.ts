import { expect, it } from "vitest";
import { timelineRulerIntervals } from "@/components/canvas/timeline/timeline-ruler";
it.each([
  [14, 5, 1],
  [21.4, 5, 1],
  [23.999, 5, 1],
  [24, 2, 0.5],
  [47.999, 2, 0.5],
  [48, 1, 0.5],
  [50, 1, 0.5],
  [59.999, 1, 0.5],
  [60, 1, 0.2],
  [100, 1, 0.2],
  [119.999, 1, 0.2],
  [120, 1, 0.1],
  [180, 1, 0.1],
])(
  "chooses documented fullscreen tick spacing at %s px/s",
  (zoom, major, minor) => {
    expect(timelineRulerIntervals(zoom)).toEqual({ major, minor });
  },
);
it("accounts for the compact node's 1.5 density factor", () => {
  expect(timelineRulerIntervals(21.4, true)).toEqual({ major: 5, minor: 1 });
  expect(timelineRulerIntervals(50, true)).toEqual({ major: 2, minor: 0.5 });
  expect(timelineRulerIntervals(180, true)).toEqual({ major: 1, minor: 0.1 });
});
