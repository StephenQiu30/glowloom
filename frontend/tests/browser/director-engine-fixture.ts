import { Mesh, MeshStandardMaterial } from "three";
import {
  DirectorScene,
  loadDirectorGLB,
} from "../../src/components/canvas/director/director-scene";
import { emptyDirectorConfig } from "@/components/canvas-editor/director/director-model.ts";
import {
  createDirectorActor,
  DIRECTOR_ACTORS,
} from "@/components/canvas-editor/director/director-assets.ts";
import { reshapeDirectorQuaterniusActor } from "@/components/canvas-editor/director/director-actor-body.ts";
import { actorPathFrames } from "@/components/canvas-editor/director/director-paths.ts";
import { freezeDirectorScene } from "@/components/canvas-editor/director/director-workspaces.ts";
import type { DirectorVideoOptions } from "@/components/canvas-editor/director/director-video-output.ts";
const host = document.createElement("div");
host.style.cssText = "position:fixed;inset:0;background:#171717";
document.body.append(host);
const scene = emptyDirectorConfig();
scene.mode = "dynamic";
const actor = createDirectorActor("female_standard");
actor.keyframes = actorPathFrames(
  actor.transform,
  [
    { x: 0, z: 0 },
    { x: 1, z: 0 },
  ],
  0,
  1,
  true,
);
scene.objects = [actor];
const errors: string[] = [];
let ready = false;
const runtime = new DirectorScene(host, {
  onSelect: () => {},
  onGestureStart: () => {},
  onTransform: () => {},
  onError: (error) => errors.push(error),
  onModelInfo: () => {
    ready = true;
  },
  loadAsset: async (object, signal) => {
    const preset = DIRECTOR_ACTORS.find(
      (item) => item.id === object.builtin_actor,
    )!;
    const model = await loadDirectorGLB(
      `/__director_validation/models/quaternius-standard-${preset.sex}.glb`,
      signal,
    );
    reshapeDirectorQuaterniusActor(model, preset.shape);
    model.traverse((node) => {
      if (node instanceof Mesh)
        node.material = new MeshStandardMaterial({
          color: object.color,
          roughness: 0.7,
        });
    });
    return model;
  },
});
runtime.update(scene, 0, null, "select", true);
const api = {
  get ready() {
    return ready;
  },
  errors,
  freeze: () => freezeDirectorScene(scene, 0.5, runtime.snapshotPoses()),
  capture: () => runtime.capture(),
  video: (options: DirectorVideoOptions) =>
    runtime.exportVideo(options, new AbortController().signal, () => {}),
  scene,
  dispose: () => runtime.dispose(),
};
Object.assign(window, { directorValidation: api });
