/** Scene content colors are persisted/rendered media values, independent of UI theme tokens. */
export const DIRECTOR_SCENE_COLORS = {
  object: "#a3a3a3",
  light: "#ffffff",
  background: "#e5e5e5",
  gridMajor: "#888888",
  gridMinor: "#bbbbbb",
  ground: "#d4d4d4",
  environmentGround: "#777777",
  selectionHelper: "#404040",
} as const;

export type DirectorEntityKind = "object" | "camera" | "light";
export type DirectorSelection = { kind: DirectorEntityKind; id: string };

export function directorTransform(position = [0, 0, 0]): API.DirectorTransform {
  return { position: [...position], rotation: [0, 0, 0], scale: [1, 1, 1] };
}

export function createDirectorCamera(index = 1): API.DirectorCamera {
  return {
    id: crypto.randomUUID(),
    name: `摄像机 ${index}`,
    transform: directorTransform([5, 3, 7]),
    target: [0, 1, 0],
    focal_length: 35,
    fov: 50,
    aperture: 2.8,
    focus_distance: 8,
    near: 0.01,
    far: 1000,
    keyframes: [],
    look_at_mode: "coordinates",
  };
}
export function createDirectorLight(
  type = "directional",
  index = 1,
): API.DirectorLight {
  return {
    id: crypto.randomUUID(),
    name: `灯光 ${index}`,
    type,
    transform: {
      ...directorTransform([3, 5, 4]),
      rotation: [-Math.PI / 3, Math.PI / 5, 0],
    },
    color: DIRECTOR_SCENE_COLORS.light,
    intensity: type === "ambient" ? 0.5 : 3,
    angle: Math.PI / 4,
    penumbra: 0.3,
    cast_shadow: true,
  };
}
export function createDirectorShot(
  cameraID: string,
  index = 1,
): API.DirectorShot {
  return {
    id: crypto.randomUUID(),
    name: `镜头 ${index}`,
    camera_id: cameraID,
    duration: 5,
    fps: 24,
    shot_size: "full",
    camera_move: "static",
    prompt: "",
  };
}
export function createDirectorConfig(): API.DirectorConfig {
  const camera = createDirectorCamera();
  const shot = createDirectorShot(camera.id!);
  return {
    id: crypto.randomUUID(),
    version: 1,
    title: "导演台",
    background: DIRECTOR_SCENE_COLORS.background,
    environment_intensity: 0.5,
    grid_visible: true,
    grid_snap: false,
    ground: { visible: true, height: 0, opacity: 1 },
    aspect_ratio: "16:9",
    objects: [],
    cameras: [camera],
    lights: [
      createDirectorLight(),
      { ...createDirectorLight("ambient", 2), intensity: 0.5 },
    ],
    shots: [shot],
    active_shot_id: shot.id,
  };
}
export function createDirectorObject(primitive: string): API.DirectorObject {
  const actor = primitive === "character";
  return {
    id: crypto.randomUUID(),
    name:
      (
        {
          box: "立方体",
          sphere: "球体",
          cylinder: "圆柱",
          plane: "平面",
          character: "人偶",
        } as Record<string, string>
      )[primitive] ?? "对象",
    kind: actor ? "actor" : "primitive",
    primitive: actor ? undefined : primitive,
    builtin_actor: actor ? "mannequin" : undefined,
    pose: actor ? "stand" : undefined,
    transform: directorTransform(actor ? [0, 0, 0] : [0, 0.5, 0]),
    color: DIRECTOR_SCENE_COLORS.object,
    visible: true,
    cast_shadow: true,
    receive_shadow: true,
    keyframes: [],
  };
}

export function directorTransformAt(
  base: API.DirectorTransform | undefined,
  frames: API.DirectorKeyframe[] | undefined,
  time: number,
): API.DirectorTransform {
  const sorted = (frames ?? [])
    .filter((f) => f.transform && Number.isFinite(f.time))
    .toSorted((a, b) => (a.time ?? 0) - (b.time ?? 0));
  if (!sorted.length) return structuredClone(base ?? directorTransform());
  if (time <= sorted[0].time!) return structuredClone(sorted[0].transform!);
  const last = sorted.at(-1)!;
  if (time >= last.time!) return structuredClone(last.transform!);
  const right = sorted.findIndex((frame) => frame.time! > time);
  const a = sorted[right - 1],
    b = sorted[right];
  let t = (time - a.time!) / (b.time! - a.time!);
  if (a.easing === "step") t = 0;
  if (a.easing === "smooth") t = t * t * (3 - 2 * t);
  const result: API.DirectorTransform = {};
  for (const key of ["position", "rotation", "scale"] as const) {
    const start =
      a.transform?.[key] ??
      base?.[key] ??
      (key === "scale" ? [1, 1, 1] : [0, 0, 0]);
    const end = b.transform?.[key] ?? start;
    result[key] = start.map((v, i) => v + (end[i] - v) * t);
  }
  return result;
}
export function putDirectorKeyframe(
  frames: API.DirectorKeyframe[] | undefined,
  time: number,
  transform: API.DirectorTransform,
  easing: string,
): API.DirectorKeyframe[] {
  const existing = frames?.find((f) => Math.abs((f.time ?? 0) - time) < 0.0001);
  const next = (frames ?? []).filter((f) => f.id !== existing?.id);
  next.push({
    id: existing?.id ?? crypto.randomUUID(),
    time,
    transform: structuredClone(transform),
    easing,
  });
  return next.toSorted((a, b) => a.time! - b.time!);
}
export function directorShotAt(shots: API.DirectorShot[], time: number) {
  let offset = 0;
  for (let index = 0; index < shots.length; index++) {
    const shot = shots[index],
      duration = shot.duration ?? 5;
    if (time < offset + duration || index === shots.length - 1)
      return {
        shot,
        localTime: Math.max(0, Math.min(duration, time - offset)),
        index,
      };
    offset += duration;
  }
  return undefined;
}
export function removeDirectorEntity(
  config: API.DirectorConfig,
  kind: DirectorEntityKind,
  id: string,
): API.DirectorConfig {
  if (kind === "object")
    return {
      ...config,
      objects: config.objects?.filter((o) => o.id !== id),
      cameras: config.cameras?.map((c) => ({
        ...c,
        follow_object_id:
          c.follow_object_id === id ? undefined : c.follow_object_id,
        look_at_object_id:
          c.look_at_object_id === id ? undefined : c.look_at_object_id,
        look_at_mode:
          c.look_at_object_id === id ? "coordinates" : c.look_at_mode,
      })),
    };
  if (kind === "light")
    return { ...config, lights: config.lights?.filter((l) => l.id !== id) };
  if ((config.cameras?.length ?? 0) <= 1) return config;
  const cameras = config.cameras!.filter((c) => c.id !== id);
  return {
    ...config,
    cameras,
    shots: config.shots?.map((s) =>
      s.camera_id === id ? { ...s, camera_id: cameras[0].id } : s,
    ),
  };
}

export function directorBoneAt(
  frames: API.DirectorBoneFrame[] | undefined,
  time: number,
  fallback: number[],
): number[] {
  const sorted = (frames ?? [])
    .filter((f) => f.rotation?.length === 4 && Number.isFinite(f.time))
    .toSorted((a, b) => a.time! - b.time!);
  if (!sorted.length) return [...fallback];
  const right = sorted.findIndex((frame) => frame.time! > time);
  const a =
    right === 0 ? sorted[0] : right < 0 ? sorted.at(-1)! : sorted[right - 1];
  const b = right <= 0 ? a : sorted[right];
  if (a === b || a.easing === "step") return [...a.rotation!];
  let t = Math.max(0, Math.min(1, (time - a.time!) / (b.time! - a.time!)));
  if (a.easing === "smooth") t = t * t * (3 - 2 * t);
  const start = a.rotation!,
    end = [...b.rotation!];
  let dot = start.reduce((sum, v, i) => sum + v * end[i], 0);
  if (dot < 0) {
    dot = -dot;
    for (let i = 0; i < 4; i++) end[i] = -end[i];
  }
  let result: number[];
  if (dot > 0.9995) result = start.map((v, i) => v + (end[i] - v) * t);
  else {
    const theta = Math.acos(Math.max(-1, Math.min(1, dot)));
    const sin = Math.sin(theta);
    result = start.map(
      (v, i) =>
        (v * Math.sin((1 - t) * theta) + end[i] * Math.sin(t * theta)) / sin,
    );
  }
  const length = Math.hypot(...result);
  return result.map((v) => v / length);
}

/** Euler rotations in radians for the actual articulated mannequin joints. */
export function directorPose(
  pose: string,
  time: number,
): { height: number; joints: Record<string, [number, number, number]> } {
  const result: ReturnType<typeof directorPose> = { height: 0.95, joints: {} };
  const half = Math.PI / 2;
  function angle(bone: string, x = 0, y = 0, z = 0) {
    result.joints[bone] = [x, y, z];
  }
  if (pose === "t_pose") {
    angle("leftUpperArm", 0, 0, -half);
    angle("rightUpperArm", 0, 0, half);
  } else if (pose === "sit" || pose === "squat") {
    result.height = pose === "sit" ? 0.55 : 0.4;
    angle("leftUpperLeg", -half);
    angle("rightUpperLeg", -half);
    angle("leftLowerLeg", half);
    angle("rightLowerLeg", half);
  } else if (pose === "kneel_single" || pose === "kneel_double") {
    result.height = 0.52;
    angle("leftUpperLeg", -half);
    angle("leftLowerLeg", half);
    angle("rightLowerLeg", half);
    if (pose === "kneel_double") angle("leftUpperLeg", 0);
  } else if (pose === "wave") {
    angle("rightUpperArm", 0, 0, 2.3);
    angle("rightLowerArm", 0, 0, -0.8);
  } else if (pose === "walk" || pose === "run") {
    const swing =
      Math.sin(time * (pose === "run" ? 10 : 6)) *
      (pose === "run" ? 0.9 : 0.55);
    angle("leftUpperLeg", swing);
    angle("rightUpperLeg", -swing);
    angle("leftUpperArm", -swing);
    angle("rightUpperArm", swing);
    angle("leftLowerLeg", Math.max(0, -swing));
    angle("rightLowerLeg", Math.max(0, swing));
  } else if (pose === "hands_hips") {
    angle("leftUpperArm", 0, 0, -0.6);
    angle("rightUpperArm", 0, 0, 0.6);
    angle("leftLowerArm", 0, 0, 1.1);
    angle("rightLowerArm", 0, 0, -1.1);
  } else if (pose === "lean") {
    angle("hips", 0, 0, 0.2);
    angle("spine", 0, 0, 0.15);
  } else if (pose === "bow") {
    angle("spine", 0.6);
    angle("chest", 0.3);
    angle("neck", 0.2);
  } else if (pose === "think") {
    angle("rightUpperArm", -0.7, 0, -0.2);
    angle("rightLowerArm", -1.9);
    angle("head", 0.2, -0.15);
  } else if (pose === "fight") {
    angle("leftUpperArm", -1, 0, -0.3);
    angle("rightUpperArm", -1, 0, 0.3);
    angle("leftLowerArm", -1.3);
    angle("rightLowerArm", -1.3);
    angle("hips", 0, 0.4);
  } else if (pose === "kick") {
    angle("rightUpperLeg", -1.4);
    angle("rightLowerLeg", 0.2);
    angle("leftUpperArm", 0, 0, -0.6);
    angle("rightUpperArm", 0, 0, 0.6);
  } else if (pose === "throw") {
    angle("rightUpperArm", -2.3, 0, 0.8);
    angle("rightLowerArm", -1.2);
    angle("chest", 0, -0.4);
    angle("leftUpperArm", -0.8);
  } else if (pose === "push" || pose === "reach") {
    angle("rightUpperArm", -half);
    angle("leftUpperArm", -half);
    if (pose === "push") angle("spine", 0.2);
    else {
      angle("rightUpperArm", -2.5);
      angle("leftUpperArm", -2.5);
    }
  } else if (pose === "arms_crossed") {
    angle("leftUpperArm", -0.7, 0, 0.3);
    angle("rightUpperArm", -0.7, 0, -0.3);
    angle("leftLowerArm", -0.4, 0, 1.8);
    angle("rightLowerArm", -0.4, 0, -1.8);
  } else if (pose === "phone") {
    angle("rightUpperArm", -0.7, 0, 0.2);
    angle("rightLowerArm", -2.1);
    angle("head", 0, 0, -0.12);
  }
  return result;
}

export function moveDirectorShot(
  config: API.DirectorConfig,
  id: string,
  offset: number,
): API.DirectorConfig {
  const shots = [...(config.shots ?? [])],
    index = shots.findIndex((shot) => shot.id === id),
    target = index + offset;
  if (index < 0 || target < 0 || target >= shots.length) return config;
  [shots[index], shots[target]] = [shots[target], shots[index]];
  return { ...config, shots };
}
