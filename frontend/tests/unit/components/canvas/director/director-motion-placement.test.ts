import { describe, expect, it } from "vitest";
import { placeDirectorMotionClip } from "@/components/canvas/director/director-motion-model";
const clip = (
  id: string,
  start: number,
  duration = 2,
): API.DirectorMotionClip => ({
  id,
  start,
  duration,
  name: "走路",
  source_animation: "walk",
  playback_rate: 1,
  loop: false,
});
describe("director motion placement", () => {
  it("allows adjacency and rejects overlap without changing the input", () => {
    const a = clip("a", 0),
      b = clip("b", 2),
      source = [a];
    expect(placeDirectorMotionClip(source, b, 20)).toEqual([a, b]);
    expect(() => placeDirectorMotionClip(source, clip("b", 1), 20)).toThrow(
      "重叠",
    );
    expect(source).toEqual([a]);
  });
  it("moves only the edited clip and bounds it to the scene", () => {
    const a = clip("a", 0),
      b = clip("b", 4);
    expect(
      placeDirectorMotionClip([a, b], { ...b, start: 2 }, 20)[1].start,
    ).toBe(2);
    expect(() => placeDirectorMotionClip([a], clip("b", 19), 20)).toThrow(
      "范围",
    );
    expect(() => placeDirectorMotionClip([a], clip("b", -1), 20)).toThrow(
      "范围",
    );
  });
});
