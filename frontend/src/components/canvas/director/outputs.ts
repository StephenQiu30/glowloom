// Screenshot grouping and stale cover semantics adapted from BeefTV 1ae25027
// web/src/components/canvas/director/director-camera-screenshot-tabs.tsx and
// web/src/lib/canvas/director/director-cover-write.ts (MIT).
import {
  directorSchema,
  type DirectorScene,
  type DirectorScreenshot,
} from "./model";

export type DirectorCaptureContext = {
  sceneId: string;
  shotId: string;
  renderKey: string;
};
// Compare only bounded, closed scene content. Labels and editor grids do not
// invalidate a rendered camera image; gallery edits do not invalidate one another.
function renderKey(scene: DirectorScene): string {
  const {
    cover: _cover,
    title: _title,
    gridSnap: _snap,
    gridVisible: _grid,
    labelsVisible: _labels,
    ...content
  } = scene;
  void _cover;
  void _title;
  void _snap;
  void _grid;
  void _labels;
  const value = {
    ...content,
    shots: scene.shots.map(({ screenshots: _screenshots, ...shot }) => {
      void _screenshots;
      return shot;
    }),
  };
  return JSON.stringify(value, (key, item: unknown) => {
    // Go's closed DTO omits these empty optional collections on a save/read.
    if (
      ["motionClips", "boneTracks", "boneOverrides"].includes(key) &&
      item &&
      typeof item === "object" &&
      Object.keys(item).length === 0
    )
      return undefined;
    return item && typeof item === "object" && !Array.isArray(item)
      ? Object.fromEntries(
          Object.entries(item).sort(([a], [b]) => a.localeCompare(b)),
        )
      : item;
  });
}
export function createDirectorCaptureContext(
  scene: DirectorScene,
): DirectorCaptureContext {
  return {
    sceneId: scene.id,
    shotId: scene.activeShotId,
    renderKey: renderKey(scene),
  };
}
export function reconcileDirectorCover(
  before: DirectorScene,
  after: DirectorScene,
): DirectorScene {
  if (
    !after.cover ||
    (renderKey(before) === renderKey(after) &&
      after.shots.some((shot) => shot.id === after.cover!.shotId))
  )
    return after;
  const { cover: _cover, ...rest } = after;
  void _cover;
  return rest;
}
export function appendDirectorScreenshot(
  scene: DirectorScene,
  context: DirectorCaptureContext,
  screenshot: DirectorScreenshot,
): DirectorScene {
  const shot = scene.shots.find((item) => item.id === context.shotId);
  if (
    !shot ||
    scene.id !== context.sceneId ||
    renderKey(scene) !== context.renderKey
  )
    throw new Error("场景已改变。截图素材仍已保存，请重新打开当前场景后捕获。");
  if (shot.screenshots?.some((item) => item.assetId === screenshot.assetId)) {
    if (
      scene.cover?.assetId === screenshot.assetId &&
      scene.cover.shotId === shot.id
    )
      return scene;
    return directorSchema.parse({
      ...scene,
      cover: { assetId: screenshot.assetId, shotId: shot.id },
    });
  }
  return directorSchema.parse({
    ...scene,
    shots: scene.shots.map((item) =>
      item.id === shot.id
        ? { ...item, screenshots: [...(item.screenshots ?? []), screenshot] }
        : item,
    ),
    cover: { assetId: screenshot.assetId, shotId: shot.id },
  });
}
export function removeDirectorScreenshot(
  scene: DirectorScene,
  shotId: string,
  screenshotId: string,
): DirectorScene {
  const removed = scene.shots
    .find((shot) => shot.id === shotId)
    ?.screenshots?.find((entry) => entry.id === screenshotId);
  if (!removed) return scene;
  const next = {
    ...scene,
    shots: scene.shots.map((shot) =>
      shot.id === shotId
        ? {
            ...shot,
            screenshots: shot.screenshots?.filter(
              (entry) => entry.id !== screenshotId,
            ),
          }
        : shot,
    ),
  };
  if (next.cover?.shotId === shotId && next.cover.assetId === removed.assetId)
    delete next.cover;
  return next;
}
export function groupDirectorScreenshots(
  scene: Pick<DirectorScene, "cameras" | "shots">,
) {
  return scene.cameras
    .map((camera) => ({
      cameraId: camera.id,
      cameraName: camera.name,
      screenshots: scene.shots
        .filter((shot) => shot.cameraId === camera.id)
        .flatMap((shot) =>
          (shot.screenshots ?? []).map((screenshot) => ({
            ...screenshot,
            shotId: shot.id,
          })),
        ),
    }))
    .filter((group) => group.screenshots.length > 0);
}
