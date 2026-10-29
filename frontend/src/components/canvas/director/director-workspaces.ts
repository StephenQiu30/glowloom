import {
  readDirector,
  emptyDirectorConfig,
  directorFingerprint,
  type Director,
  type DirectorObject,
  type DirectorTransform,
  type DirectorFrozenNode,
} from "./director-model";
import {
  sampleDirectorTransform,
  sampleDirectorCamera,
} from "./director-animation";
import { sampleDirectorBone } from "./director-motion";
export type DirectorMode = "static" | "dynamic";
export type DirectorSceneData = Omit<
  Director,
  "id" | "title" | "version" | "mode" | "workspaces"
>;
export type DirectorDocument = {
  id: string;
  title: string;
  version: 2;
  mode: DirectorMode;
  workspaces: { static: DirectorSceneData; dynamic: DirectorSceneData };
};
export type DirectorNodePoses = Record<
  string,
  Record<string, DirectorFrozenNode>
>;
export type DirectorPoses = Record<string, Record<string, DirectorTransform>>;
function sceneData(value: Director): DirectorSceneData {
  const { id, title, version, mode, workspaces, ...scene } = value;
  void id;
  void title;
  void version;
  void mode;
  void workspaces;
  return structuredClone(scene);
}
function sceneFor(document: DirectorDocument, mode: DirectorMode): Director {
  return readDirector({
    ...document.workspaces[mode],
    id: document.id,
    title: document.title,
    version: 1,
    mode,
  });
}
/** Validates both workspaces without upgrading a caller's CAS baseline. */
export function validateDirectorDocument(
  value: API.DirectorConfig | undefined,
): API.DirectorConfig {
  if (value?.version === 1) {
    if (value.workspaces != null)
      throw new Error("旧导演台不能包含双工作区数据");
    return readDirector(value);
  }
  if (
    !value ||
    value.version !== 2 ||
    !["static", "dynamic"].includes(value.mode ?? "") ||
    !value.workspaces?.static ||
    !value.workspaces.dynamic
  )
    throw new Error("导演台工作区数据无效");
  if (
    Object.keys(value).some(
      (key) => !["id", "title", "version", "mode", "workspaces"].includes(key),
    )
  )
    throw new Error("双工作区不能混入旧场景字段");
  if (
    Object.keys(value.workspaces).some(
      (key) => !["static", "dynamic"].includes(key),
    )
  )
    throw new Error("工作区类型无效");
  const document = value as DirectorDocument;
  for (const mode of ["static", "dynamic"] as const) sceneFor(document, mode);
  if (new TextEncoder().encode(JSON.stringify(document)).length > 512 * 1024)
    throw new Error("导演台场景过大");
  return structuredClone(document);
}
const hasMotion = (object: DirectorObject) =>
  !!(
    object.keyframes.length ||
    object.motion_clips?.length ||
    object.bone_tracks?.some((track) => track.keyframes?.length)
  );
function stripAnimation(scene: Director) {
  for (const object of scene.objects) {
    object.keyframes = [];
    delete object.motion_clips;
    delete object.active_motion_clip_id;
    delete object.bone_tracks;
  }
  for (const camera of scene.cameras) camera.keyframes = [];
}
function initialWorkspace(id: string, mode: DirectorMode): DirectorSceneData {
  const scene = emptyDirectorConfig();
  // Stable IDs make repeated legacy reads and CAS/history comparisons identical.
  const derive = (salt: bigint) =>
    id.slice(0, 24) +
    (BigInt(`0x${id.slice(24)}`) ^ salt).toString(16).padStart(12, "0");
  scene.cameras[0].id = derive(
    mode === "static" ? BigInt("0x646972000001") : BigInt("0x646972000002"),
  );
  scene.shots[0].id = derive(
    mode === "static" ? BigInt("0x646972000003") : BigInt("0x646972000004"),
  );
  scene.shots[0].camera_id = scene.cameras[0].id;
  scene.active_shot_id = scene.shots[0].id;
  return sceneData(scene);
}
export function migrateDirectorDocument(
  value: API.DirectorConfig | undefined,
): DirectorDocument {
  const checked = validateDirectorDocument(value);
  if (checked.version === 2) return checked as DirectorDocument;
  const legacy = readDirector(checked),
    mode = legacy.mode === "dynamic" ? "dynamic" : "static";
  const document: DirectorDocument = {
    id: legacy.id,
    title: legacy.title,
    version: 2,
    mode,
    workspaces: {
      static: initialWorkspace(legacy.id, "static"),
      dynamic: initialWorkspace(legacy.id, "dynamic"),
    },
  };
  document.workspaces[mode] = sceneData(legacy);
  if (
    mode === "static" &&
    (legacy.objects.some(hasMotion) ||
      legacy.cameras.some((camera) => camera.keyframes.length))
  ) {
    document.workspaces.dynamic = sceneData(legacy);
    // Legacy static mode did not evaluate animation: preserve its visible pose,
    // while retaining every hidden animation in the dynamic workspace.
    stripAnimation(legacy);
    document.workspaces.static = sceneData(legacy);
  }
  return document;
}
export function activeDirectorScene(document: DirectorDocument): Director {
  return sceneFor(document, document.mode);
}
export function replaceDirectorScene(
  document: DirectorDocument,
  scene: Director,
): DirectorDocument {
  const next = structuredClone(document);
  if (scene.id !== document.id || scene.mode !== document.mode)
    throw new Error("场景或工作区已切换");
  next.workspaces[document.mode] = sceneData(readDirector(scene));
  return validateDirectorDocument(next) as DirectorDocument;
}
export function freezeDirectorScene(
  scene: Director,
  time: number,
  poses?: DirectorPoses,
  nodePoses?: DirectorNodePoses,
): Director {
  if (!Number.isFinite(time) || time < 0) throw new Error("冻结时间无效");
  const frozen = readDirector(scene);
  for (const object of frozen.objects) {
    object.transform = sampleDirectorTransform(
      object.transform,
      object.keyframes,
      time,
    );
    if (object.motion_clips?.length && !nodePoses?.[object.id])
      throw new Error("请等待角色动作加载后再冻结当前帧");
    object.frozen_bones = poses?.[object.id] ?? object.frozen_bones;
    object.frozen_nodes = nodePoses?.[object.id] ?? object.frozen_nodes;
    object.bone_overrides = { ...object.bone_overrides };
    for (const track of object.bone_tracks ?? []) {
      const pose = sampleDirectorBone(track.keyframes ?? [], time);
      if (pose && track.bone)
        object.bone_overrides[track.bone] = pose.toArray();
    }
  }
  for (const camera of frozen.cameras)
    Object.assign(camera, sampleDirectorCamera(camera, time));
  stripAnimation(frozen);
  frozen.mode = "static";
  return frozen;
}
export function switchDirectorWorkspace(
  document: DirectorDocument,
  mode: DirectorMode,
  synchronize: boolean,
  time: number,
  poses?: DirectorPoses,
  nodePoses?: DirectorNodePoses,
): DirectorDocument {
  const next = structuredClone(document);
  if (mode === document.mode) return next;
  if (synchronize && mode === "static")
    next.workspaces.static = sceneData(
      freezeDirectorScene(
        sceneFor(document, "dynamic"),
        time,
        poses,
        nodePoses,
      ),
    );
  if (synchronize && mode === "dynamic") {
    const source = sceneFor(document, "static"),
      dynamic = sceneFor(document, "dynamic");
    const objects = new Map(
      source.objects.map((object) => [object.id, object]),
    );
    if (
      dynamic.objects.some(
        (object) => hasMotion(object) && !objects.has(object.id),
      )
    )
      throw new Error("时间轴仍引用静态场景中缺失的对象，不能覆盖");
    const cameras = new Set(source.cameras.map((camera) => camera.id));
    if (
      dynamic.cameras.some(
        (camera) => camera.keyframes.length && !cameras.has(camera.id),
      )
    )
      throw new Error("时间轴仍引用静态场景中缺失的机位，不能覆盖");
    for (const previous of dynamic.objects) {
      const object = objects.get(previous.id);
      if (
        object &&
        hasMotion(previous) &&
        (object.asset_id !== previous.asset_id ||
          object.builtin_actor !== previous.builtin_actor ||
          object.builtin_model !== previous.builtin_model ||
          JSON.stringify(object.rig?.bone_map ?? {}) !==
            JSON.stringify(previous.rig?.bone_map ?? {}))
      )
        throw new Error("角色模型或骨架已改变，不能覆盖仍被时间轴引用的角色");
    }
    // The first sync starts from the empty workspace's default camera. Once
    // animated, its shot and camera references must survive subsequent syncs.
    const pristine = emptyDirectorConfig();
    pristine.cameras[0].id = dynamic.cameras[0].id;
    pristine.shots[0].id = dynamic.shots[0].id;
    pristine.shots[0].camera_id = pristine.cameras[0].id;
    pristine.active_shot_id = pristine.shots[0].id;
    const empty =
      directorFingerprint(sceneData(dynamic)) ===
      directorFingerprint(sceneData(pristine));
    if (!empty && dynamic.shots.some((shot) => !cameras.has(shot.camera_id)))
      throw new Error("时间轴仍引用静态场景中缺失的机位，不能覆盖");
    for (const object of source.objects) {
      const previous = dynamic.objects.find(
        (candidate) => candidate.id === object.id,
      );
      object.pose = "neutral";
      delete object.bone_overrides;
      delete object.frozen_bones;
      delete object.frozen_nodes;
      object.keyframes = previous?.keyframes ?? [];
      object.motion_clips = previous?.motion_clips;
      const preservedSources = previous?.motion_library ?? [];
      object.motion_library = [
        ...preservedSources,
        ...(object.motion_library ?? []).filter(
          (source) => !preservedSources.some((old) => old.id === source.id),
        ),
      ];
      object.bone_tracks = previous?.bone_tracks;
      object.active_motion_clip_id = previous?.active_motion_clip_id;
    }
    for (const camera of source.cameras)
      camera.keyframes =
        dynamic.cameras.find((candidate) => candidate.id === camera.id)
          ?.keyframes ?? [];
    if (!empty) {
      source.shots = dynamic.shots;
      source.active_shot_id = dynamic.active_shot_id;
      source.cover = dynamic.cover;
    }
    next.workspaces.dynamic = sceneData(source);
  }
  next.mode = mode;
  return validateDirectorDocument(next) as DirectorDocument;
}
