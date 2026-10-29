import { describe, expect, it } from "vitest";
import {
  directorFitView,
  directorViewLayout,
  directorViewPoint,
} from "@/components/canvas/director/director-view-layout";
import {
  directorLightAngles,
  directorLightRotation,
} from "@/components/canvas/director/director-light-angles";
const options = {
  split: false,
  compact: false,
  cameraMain: false,
  previewVisible: true,
  previewWidth: 320,
};
describe("director viewport isolation", () => {
  it("splits into nonoverlapping editable and camera regions even with floating preview hidden", () => {
    const view = directorViewLayout(1000, 600, 16 / 9, {
      ...options,
      split: true,
      previewVisible: false,
    });
    expect(view.editor).toEqual({ x: 0, y: 0, width: 500, height: 600 });
    expect(directorViewPoint(view.main, 250, 300)).toEqual([0, 0]);
    expect(directorViewPoint(view.main, 750, 300)).toBeNull();
    expect(directorViewPoint(view.preview!, 750, 300)).toEqual([0, 0]);
    expect(view.preview!.width / view.preview!.height).toBeCloseTo(16 / 9);
  });
  it("letterboxes camera selection rays and keeps portrait capture aspect independent", () => {
    const view = directorViewLayout(1000, 600, 9 / 16, {
      ...options,
      cameraMain: true,
      previewVisible: false,
    });
    expect(directorViewPoint(view.main, 500, 300)).toEqual([0, 0]);
    expect(directorViewPoint(view.main, 10, 300)).toBeNull();
    expect(view.main.width / view.main.height).toBeCloseTo(9 / 16);
    expect(view.preview).toBeNull();
  });
  it("compact playback uses all available space, independent of editor split", () => {
    const view = directorViewLayout(1000, 600, 16 / 9, {
      ...options,
      compact: true,
      split: true,
    });
    expect(view.preview!.width).toBe(1000);
    expect(directorViewPoint(view.preview!, 500, 300)).toEqual([0, 0]);
  });
  it("keeps a portrait floating preview anchored to the right edge", () => {
    const view = directorViewLayout(1000, 600, 9 / 16, options);
    expect(view.preview!.x + view.preview!.width).toBe(988);
    expect(view.preview!.height).toBe(300);
    expect(view.preview!.y).toBe(48);
  });
  it("rejects points in letterbox bars and sanitizes invalid ratios", () => {
    const rect = directorFitView(
      { x: 10, y: 20, width: 300, height: 300 },
      NaN,
    );
    expect(Number.isFinite(rect.height)).toBe(true);
    expect(directorViewPoint(rect, 100, 20)).toBeNull();
  });
});
describe("light direction angle controls", () => {
  it.each([
    [0, 45],
    [90, 30],
    [-135, -15],
    [179, 70],
  ])(
    "round trips horizontal %s and elevation %s without Euler-axis assumptions",
    (horizontal, elevation) => {
      const actual = directorLightAngles(
        directorLightRotation(horizontal, elevation),
      );
      expect(actual.horizontal).toBeCloseTo(horizontal);
      expect(actual.elevation).toBeCloseTo(elevation);
    },
  );
  it("clamps elevation to a valid pole", () =>
    expect(
      directorLightAngles(directorLightRotation(0, 140)).elevation,
    ).toBeCloseTo(90));
});
