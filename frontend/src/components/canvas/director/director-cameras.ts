import { newDirectorCamera, type DirectorCamera } from "./director-model";
export const DIRECTOR_CAMERA_PRESETS = [
  ["current", "当前视角"],
  ["medium", "正面中景"],
  ["close", "正面特写"],
  ["wide", "正面全景"],
  ["side_follow", "侧面跟拍"],
  ["side_close", "侧面近景"],
  ["back", "背面"],
  ["top", "俯拍"],
  ["high45", "45° 俯拍"],
  ["low", "低角度"],
  ["shoulder", "过肩"],
  ["bird", "鸟瞰"],
  ["dutch", "荷兰角"],
] as const;
export function directorCameraPreset(
  kind: string,
  target?: { id: string; position: number[] },
  view?: { position: number[]; target: number[]; fov: number },
): DirectorCamera {
  const preset = DIRECTOR_CAMERA_PRESETS.find(([id]) => id === kind);
  if (!preset) throw new Error("机位预设不存在");
  if (
    ["close", "side_follow", "side_close", "shoulder"].includes(kind) &&
    !target
  )
    throw new Error("请先选择一个目标对象");
  const camera = newDirectorCamera(preset[1]);
  if (kind === "current") {
    if (!view) throw new Error("导演视角尚未就绪");
    camera.transform.position = [...view.position];
    camera.target = [...view.target];
    camera.fov = view.fov;
    return camera;
  }
  const point = target
    ? [target.position[0], target.position[1] + 1, target.position[2]]
    : [0, 1, 0];
  const offsets: Record<string, number[]> = {
    medium: [0, 0.2, 4],
    close: [0, 0.2, 1.4],
    wide: [0, 1, 8],
    side_follow: [4, 0.2, 0],
    side_close: [1.8, 0.1, 0],
    back: [0, 0.2, -4],
    top: [0, 5, 0.01],
    high45: [0, 4, 4],
    low: [0, -0.8, 3],
    shoulder: [1, 0.6, -2],
    bird: [0, 10, 0.01],
    dutch: [0, 0.2, 4],
  };
  camera.target = point;
  camera.transform.position = offsets[kind].map((v, i) => v + point[i]);
  if (kind === "dutch") camera.roll = Math.PI / 8;
  if (target && kind === "side_follow") {
    camera.follow_object_id = target.id;
    camera.transform.position = camera.transform.position.map(
      (v, i) => v - target.position[i],
    );
    camera.look_at_mode = "object";
    camera.look_at_object_id = target.id;
  }
  return camera;
}
