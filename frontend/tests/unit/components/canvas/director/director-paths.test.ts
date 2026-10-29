import { expect, it } from "vitest";
import { createDirectorActor } from "@/components/canvas/director/director-assets";
import {
  actorPathFrames,
  customCameraPathFrames,
  cameraMotionFrames,
  mergeDirectorPath,
} from "@/components/canvas/director/director-paths";
import { emptyDirectorConfig } from "@/components/canvas/director/director-model";
it("starts a drawn path at the actor and distributes time by traveled distance", () => {
  const actor = createDirectorActor("male_standard");
  actor.transform.position = [3, 0, 5];
  const frames = actorPathFrames(
    actor.transform,
    [
      { x: 0, z: 0 },
      { x: 1, z: 0 },
      { x: 4, z: 0 },
    ],
    2,
    4,
    true,
  );
  expect(frames[0].time).toBe(2);
  expect(frames.at(-1)!.time).toBe(6);
  expect(frames[0].transform.position).toEqual([3, 0, 5]);
  expect(frames.at(-1)!.transform.position).toEqual([7, 0, 5]);
  expect(
    frames.every((frame) => frame.transform.rotation[1] === Math.PI / 2),
  ).toBe(true);
});
it("path collision requires explicit replacement and preserves other channels", () => {
  const actor = createDirectorActor("female_child");
  const path = actorPathFrames(
    actor.transform,
    [
      { x: 0, z: 0 },
      { x: 0, z: 3 },
    ],
    0,
    3,
    false,
  );
  const optical = {
    ...path[0],
    id: crypto.randomUUID(),
    position_keyed: false,
    rotation_keyed: true,
    fov: 60,
  };
  expect(() => mergeDirectorPath(path, path, false)).toThrow();
  expect(mergeDirectorPath([optical], path, false)[0].fov).toBe(60);
  expect(mergeDirectorPath(path, path, true)).toHaveLength(path.length);
});
it("camera orbit closes at its start, and dolly zoom changes FOV", () => {
  const camera = emptyDirectorConfig().cameras[0];
  const orbit = cameraMotionFrames(camera, "orbit", 0, 4);
  expect(orbit[0].transform.position).toEqual(orbit.at(-1)!.transform.position);
  const zoom = cameraMotionFrames(camera, "dolly_zoom", 0, 4);
  expect(zoom[0].fov).not.toBe(zoom.at(-1)!.fov);
});

it("retains the original curve when a path shares time with another channel", () => {
  const base = emptyDirectorConfig().cameras[0];
  const path = actorPathFrames(
    base.transform,
    [
      { x: 0, z: 0 },
      { x: 3, z: 0 },
    ],
    0,
    3,
    false,
  );
  const rotation = {
    ...path[0],
    id: crypto.randomUUID(),
    position_keyed: false,
    rotation_keyed: true,
    easing: "step",
  };
  const merged = mergeDirectorPath([rotation], path, false);
  expect(merged.find((f) => f.id === rotation.id)?.easing).toBe("step");
  expect(merged.find((f) => f.position_keyed)?.easing).toBe("linear");
});
it("free camera paths own position and target while preserving FOV and roll", () => {
  const camera = emptyDirectorConfig().cameras[0];
  const points = [
    { x: 0, z: 0 },
    { x: 2, z: 0 },
  ];
  const frames = customCameraPathFrames(camera, "free", points, points, 1, 3);
  expect(frames[0].transform.position).toEqual(camera.transform.position);
  expect(frames.at(-1)?.target?.[0]).toBe(camera.target[0] + 2);
  expect(frames.every((f) => f.fov === undefined && f.roll === undefined)).toBe(
    true,
  );
  expect(
    customCameraPathFrames(camera, "fixed-camera", [], points, 0, 1).every(
      (f) => f.position_keyed === false,
    ),
  ).toBe(true);
});

it("position camera presets preserve independent FOV and roll channels", () => {
  const camera = emptyDirectorConfig().cameras[0];
  expect(
    cameraMotionFrames(camera, "push_in", 0, 3).every(
      (frame) => frame.fov === undefined && frame.roll === undefined,
    ),
  ).toBe(true);
});
