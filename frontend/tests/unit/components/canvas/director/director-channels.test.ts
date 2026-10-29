import { expect, it } from "vitest";
import {
  sampleDirectorTransform,
  sampleDirectorCamera,
  keyDirectorCamera,
} from "@/components/canvas/director/director-animation";
import {
  directorTransform,
  emptyDirectorConfig,
  editDirector,
} from "@/components/canvas/director/director-model";
it("does not let rotation keys pause or overwrite a position track", () => {
  const base = directorTransform();
  const frames = [
    {
      id: crypto.randomUUID(),
      time: 0,
      easing: "linear",
      transform: base,
      position_keyed: true,
      rotation_keyed: false,
      scale_keyed: false,
    },
    {
      id: crypto.randomUUID(),
      time: 5,
      easing: "linear",
      transform: { ...base, position: [99, 99, 99], rotation: [0, 1, 0] },
      position_keyed: false,
      rotation_keyed: true,
      scale_keyed: false,
    },
    {
      id: crypto.randomUUID(),
      time: 10,
      easing: "linear",
      transform: { ...base, position: [10, 0, 0] },
      position_keyed: true,
      rotation_keyed: false,
      scale_keyed: false,
    },
  ];
  expect(sampleDirectorTransform(base, frames, 5)).toEqual({
    position: [5, 0, 0],
    rotation: [0, 1, 0],
    scale: [1, 1, 1],
  });
});
it("samples target, FOV and roll independently with bounded endpoints", () => {
  const camera = emptyDirectorConfig().cameras[0];
  camera.keyframes = [0, 10].map((time) => ({
    id: crypto.randomUUID(),
    time,
    easing: "linear",
    transform: camera.transform,
    position_keyed: false,
    rotation_keyed: false,
    scale_keyed: false,
    target: [time, 1, 0],
    fov: 40 + time * 2,
    roll: time / 10,
  }));
  expect(sampleDirectorCamera(camera, 5)).toMatchObject({
    target: [5, 1, 0],
    fov: 50,
    roll: 0.5,
  });
  expect(sampleDirectorCamera(camera, 20).fov).toBe(60);
});

it("record-all updates split transform owners once and keeps optics easing", () => {
  const scene = emptyDirectorConfig(),
    camera = scene.cameras[0];
  const make = (easing: string) => ({
    id: crypto.randomUUID(),
    time: 2,
    easing,
    transform: directorTransform(),
    position_keyed: false,
    rotation_keyed: false,
    scale_keyed: false,
  });
  camera.keyframes = [
    { ...make("step"), fov: 40 },
    { ...make("linear"), position_keyed: true },
    { ...make("smooth"), rotation_keyed: true },
  ];
  const transform = {
    position: [4, 5, 6],
    rotation: [0, 1, 0],
    scale: [2, 2, 2],
  };
  const result = editDirector(scene, {
    type: "keyframe",
    selection: { kind: "camera", id: camera.id },
    time: 2,
    transform,
  });
  const frames = result.cameras[0].keyframes;
  for (const channel of ["position", "rotation", "scale"] as const)
    expect(frames.filter((f) => f[`${channel}_keyed`] !== false)).toHaveLength(
      1,
    );
  expect(sampleDirectorTransform(camera.transform, frames, 2)).toEqual(
    transform,
  );
  expect(frames.find((f) => f.fov !== undefined)?.easing).toBe("step");
  expect(frames.find((f) => f.rotation_keyed !== false)?.easing).toBe("smooth");
  expect(scene.cameras[0].keyframes).toHaveLength(3);
});
it("recording one transform with new easing leaves sibling easing untouched", () => {
  const scene = emptyDirectorConfig(),
    camera = scene.cameras[0];
  camera.keyframes = [
    {
      id: crypto.randomUUID(),
      time: 0,
      easing: "step",
      transform: directorTransform(),
      fov: 42,
    },
  ];
  const result = editDirector(scene, {
    type: "keyframe",
    selection: { kind: "camera", id: camera.id },
    time: 0,
    channel: "position",
    transform: directorTransform([8, 0, 0]),
    easing: "linear",
  });
  const frames = result.cameras[0].keyframes;
  expect(frames.find((f) => f.rotation_keyed !== false)?.easing).toBe("step");
  expect(frames.find((f) => f.fov !== undefined)?.easing).toBe("step");
  expect(frames.find((f) => f.position_keyed !== false)?.easing).toBe("linear");
});
it("optical patches update separate same-time owners without duplicates", () => {
  const camera = emptyDirectorConfig().cameras[0];
  const make = (easing: string) => ({
    id: crypto.randomUUID(),
    time: 3,
    easing,
    transform: directorTransform(),
    position_keyed: false,
    rotation_keyed: false,
    scale_keyed: false,
  });
  camera.keyframes = [
    { ...make("smooth"), target: [0, 1, 0] },
    { ...make("step"), fov: 40 },
    { ...make("linear"), roll: 0 },
  ];
  const result = keyDirectorCamera(camera, 3, {
    target: [2, 1, 0],
    fov: 70,
    roll: 1,
  });
  for (const channel of ["target", "fov", "roll"] as const)
    expect(
      result.keyframes.filter((f) => f[channel] !== undefined),
    ).toHaveLength(1);
  expect(result.keyframes.find((f) => f.fov !== undefined)?.easing).toBe(
    "step",
  );
  expect(sampleDirectorCamera(result, 3)).toMatchObject({
    target: [2, 1, 0],
    fov: 70,
    roll: 1,
  });
  expect(camera.keyframes[1].fov).toBe(40);
});
