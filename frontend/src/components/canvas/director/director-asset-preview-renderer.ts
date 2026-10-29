import {
  Box3,
  DirectionalLight,
  HemisphereLight,
  OrthographicCamera,
  Scene,
  Vector3,
  WebGLRenderer,
} from "three";
import { disposeDirectorObject, loadDirectorGLB } from "./director-scene";

// One short-lived renderer serializes thumbnails, avoiding one WebGL context per card.
let queue: Promise<unknown> = Promise.resolve();
let renderer: WebGLRenderer | undefined;
let release: ReturnType<typeof setTimeout> | undefined;
export function renderDirectorAssetThumbnail(
  url: string,
  signal: AbortSignal,
): Promise<string> {
  const task = queue
    .catch(() => {})
    .then(async () => {
      signal.throwIfAborted();
      const model = await loadDirectorGLB(url, signal);
      try {
        signal.throwIfAborted();
        clearTimeout(release);
        renderer ??= new WebGLRenderer({
          antialias: true,
          preserveDrawingBuffer: true,
        });
        renderer.setSize(192, 192);
        renderer.setClearColor("#202020");
        const bounds = new Box3().setFromObject(model, true);
        const center = bounds.getCenter(new Vector3()),
          size = bounds.getSize(new Vector3());
        const radius = Math.max(size.length() * 0.6, 0.01);
        if (!Number.isFinite(radius) || bounds.isEmpty())
          throw new Error("模型没有可预览的几何体");
        const scene = new Scene();
        const light = new DirectionalLight(0xffffff, 3);
        light.position.set(3, 5, 4);
        scene.add(model, light, new HemisphereLight(0xffffff, 0x444455, 2));
        const camera = new OrthographicCamera(
          -radius,
          radius,
          radius,
          -radius,
          0.001,
          radius * 20,
        );
        camera.position
          .copy(center)
          .add(new Vector3(1, 0.7, 1).normalize().multiplyScalar(radius * 4));
        camera.lookAt(center);
        renderer.render(scene, camera);
        signal.throwIfAborted();
        return renderer.domElement.toDataURL("image/png");
      } finally {
        disposeDirectorObject(model);
        release = setTimeout(() => {
          renderer?.dispose();
          renderer?.forceContextLoss();
          renderer = undefined;
        }, 1500);
      }
    });
  queue = task;
  return task;
}
