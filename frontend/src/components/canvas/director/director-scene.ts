import {
  directorViewLayout,
  directorViewPoint,
  type DirectorViewRect,
} from "./director-view-layout";
import { directorGroundPosition } from "./director-ground";
import {
  directorNameLabel,
  directorPathHelpers,
  directorHeightReference,
} from "./director-view-helpers";
import {
  encodeDirectorVideo,
  type DirectorVideoOptions,
} from "./director-video-output";
import * as THREE from "three";
import { OrbitControls } from "three/addons/controls/OrbitControls.js";
import { TransformControls } from "three/addons/controls/TransformControls.js";
import { GLTFLoader } from "three/addons/loaders/GLTFLoader.js";
import {
  DirectorModelMotion,
  directorModelInfo,
  type DirectorModelInfo,
} from "./director-motion";
import {
  sampleDirectorTransform,
  sampleDirectorCamera,
} from "./director-animation";
import {
  directorEntity,
  directorKey,
  type Director,
  type DirectorObject,
  type DirectorSelection,
  type DirectorTransform,
} from "./director-model";
import {
  createDirectorTransaction,
  installDirectorTerminalListeners,
  type DirectorTransaction,
} from "./director-gesture";
export type DirectorTool = "select" | "translate" | "rotate" | "scale";
const tuple = (v: number[]) => v as [number, number, number];
export function applyDirectorTransform(
  object: THREE.Object3D,
  transform: DirectorTransform,
) {
  object.position.fromArray(transform.position);
  object.rotation.set(...tuple(transform.rotation));
  object.scale.fromArray(transform.scale);
}
export function readDirectorTransform(
  object: THREE.Object3D,
): DirectorTransform {
  return {
    position: object.position.toArray(),
    rotation: [object.rotation.x, object.rotation.y, object.rotation.z],
    scale: object.scale.toArray(),
  };
}
export function disposeDirectorObject(object: THREE.Object3D) {
  const skeletons = new Set<THREE.Skeleton>();
  const geometries = new Set<THREE.BufferGeometry>();
  const materialsSeen = new Set<THREE.Material>();
  const textures = new Set<THREE.Texture>();
  object.traverse((node) => {
    if (node instanceof THREE.SkinnedMesh && !skeletons.has(node.skeleton)) {
      skeletons.add(node.skeleton);
      node.skeleton.dispose();
    }
    if (
      node instanceof THREE.DirectionalLight ||
      node instanceof THREE.SpotLight ||
      node instanceof THREE.PointLight
    )
      node.shadow.dispose();
    const mesh = node as THREE.Mesh;
    if (mesh.geometry && !geometries.has(mesh.geometry)) {
      geometries.add(mesh.geometry);
      mesh.geometry.dispose();
    }
    const materials = Array.isArray(mesh.material)
      ? mesh.material
      : mesh.material
        ? [mesh.material]
        : [];
    for (const material of materials) {
      if (materialsSeen.has(material)) continue;
      materialsSeen.add(material);
      for (const value of Object.values(material))
        if (value instanceof THREE.Texture && !textures.has(value)) {
          textures.add(value);
          value.dispose();
        }
      material.dispose();
    }
  });
}
export function createDirectorPrimitive(config: DirectorObject): THREE.Group {
  const group = new THREE.Group();
  const material = new THREE.MeshStandardMaterial({
    color: config.color,
    roughness: 0.65,
    opacity: config.opacity ?? 1,
    transparent: (config.opacity ?? 1) < 1,
    side: THREE.DoubleSide,
  });
  const add = (
    geometry: THREE.BufferGeometry,
    position: [number, number, number],
  ) => {
    const mesh = new THREE.Mesh(geometry, material);
    mesh.position.set(...position);
    mesh.castShadow = config.cast_shadow;
    mesh.receiveShadow = config.receive_shadow;
    group.add(mesh);
  };
  if (
    config.primitive === "character" ||
    config.builtin_actor === "mannequin"
  ) {
    // Original geometric blocking mannequin; no third-party character assets.
    add(new THREE.SphereGeometry(0.15, 16, 12), [0, 1.5, 0]);
    add(new THREE.BoxGeometry(0.42, 0.55, 0.22), [0, 1.03, 0]);
    for (const side of [-1, 1]) {
      add(new THREE.CapsuleGeometry(0.075, 0.55, 4, 8), [side * 0.13, 0.37, 0]);
      add(new THREE.CapsuleGeometry(0.055, 0.5, 4, 8), [side * 0.31, 0.97, 0]);
    }
  } else {
    const geometries: Record<string, () => THREE.BufferGeometry> = {
      box: () => new THREE.BoxGeometry(1, 1, 1),
      cuboid: () => new THREE.BoxGeometry(1.5, 1, 0.75),
      pyramid: () =>
        new THREE.ConeGeometry(Math.SQRT1_2, 1, 4).rotateY(Math.PI / 4),
      half_arc: () => new THREE.TorusGeometry(0.5, 0.15, 16, 48, Math.PI),
      half_cylinder: () => {
        const shape = new THREE.Shape();
        shape.moveTo(-0.5, 0);
        shape.absarc(0, 0, 0.5, Math.PI, 0, true);
        shape.closePath();
        return new THREE.ExtrudeGeometry(shape, {
          depth: 1,
          bevelEnabled: false,
          curveSegments: 24,
        })
          .rotateX(-Math.PI / 2)
          .translate(0, -0.5, 0);
      },
      sphere: () => new THREE.SphereGeometry(0.5, 32, 16),
      cylinder: () => new THREE.CylinderGeometry(0.5, 0.5, 1, 24),
      plane: () => new THREE.PlaneGeometry(2, 2),
      cone: () => new THREE.ConeGeometry(0.5, 1, 32),
      torus: () => new THREE.TorusGeometry(0.5, 0.15, 16, 48),
      capsule: () => new THREE.CapsuleGeometry(0.25, 0.5, 8, 16),
      circle: () => new THREE.CircleGeometry(0.5, 32),
      ring: () => new THREE.RingGeometry(0.3, 0.5, 32),
      tetrahedron: () => new THREE.TetrahedronGeometry(0.6),
      octahedron: () => new THREE.OctahedronGeometry(0.6),
      dodecahedron: () => new THREE.DodecahedronGeometry(0.6),
      icosahedron: () => new THREE.IcosahedronGeometry(0.6),
    };
    const geometry = (
      geometries[config.primitive ?? "box"] ?? geometries.box
    )();
    add(geometry, [0, 0, 0]);
    if (config.primitive === "plane")
      group.children[0].rotation.x = -Math.PI / 2;
  }
  return group;
}
export async function loadDirectorGLB(
  url: string,
  signal: AbortSignal,
): Promise<THREE.Group> {
  // eslint-disable-next-line no-restricted-syntax -- Read authorized object-store bytes, not a business API.
  const response = await fetch(url, { signal });
  if (!response.ok) throw new Error("模型文件读取失败");
  const bytes = await response.arrayBuffer();
  signal.throwIfAborted();
  if (!bytes.byteLength || bytes.byteLength > 64 * 1024 * 1024)
    throw new Error("模型文件为空或超过 64 MiB");
  const binary =
    bytes.byteLength >= 12 &&
    new DataView(bytes).getUint32(0, true) === 0x46546c67;
  const source = binary ? bytes : new TextDecoder().decode(bytes);
  const manager = new THREE.LoadingManager();
  manager.setURLModifier((resource) => {
    if (!resource.startsWith("blob:") && !resource.startsWith("data:"))
      throw new Error("模型包含外部资源，不能加载");
    return resource;
  });
  const model = await new GLTFLoader(manager).parseAsync(source, "");
  if (signal.aborted) {
    disposeDirectorObject(model.scene);
    signal.throwIfAborted();
  }
  model.scene.animations = model.animations;
  return model.scene;
}
export type DirectorSceneCallbacks = {
  onSelect: (selection: DirectorSelection | null) => void;
  onPlace?: (position: number[]) => void;
  onTargetSelect?: (cameraID: string) => void;
  onTargetTransform?: (cameraID: string, target: number[]) => void;
  onContextState?: (lost: boolean) => void;
  onOrientation?: (rotation: THREE.Quaternion) => void;
  onGestureStart: () => void;
  onGestureCancel?: () => void;
  onTransform: (selection: DirectorSelection, value: DirectorTransform) => void;
  onError: (message: string) => void;
  onModelInfo?: (info: DirectorModelInfo) => void;
  loadAsset: (
    object: DirectorObject,
    signal: AbortSignal,
  ) => Promise<THREE.Object3D>;
};
export class DirectorScene {
  readonly renderer: THREE.WebGLRenderer;
  readonly scene = new THREE.Scene();
  readonly directorCamera = new THREE.PerspectiveCamera(50, 1, 0.1, 2000);
  readonly orbit: OrbitControls;
  readonly gizmo: TransformControls;
  private content = new THREE.Group();
  private helpers = new THREE.Group();
  private stage = new THREE.Group();
  private targets = new Map<string, THREE.Object3D>();
  private cameras = new Map<string, THREE.PerspectiveCamera>();
  private model: Director | null = null;
  private selection: DirectorSelection | null = null;
  private observer: ResizeObserver;
  private frame = 0;
  private abort = new AbortController();
  private width = 1;
  private height = 1;
  private disposed = false;
  private motions = new Map<string, DirectorModelMotion>();
  private currentTime = 0;
  private previewObjectID: string | null = null;
  setPreviewObject(id: string | null) {
    this.previewObjectID = id;
  }
  private mainView = "director";
  private split = false;
  private inputSurface: HTMLDivElement;
  private contextUnavailable = false;
  private targetCameraID: string | null = null;
  private cameraTargets = new Map<string, THREE.Object3D>();
  private targetHelpers = new THREE.Group();
  private targetLines = new Map<string, THREE.Line>();
  private lastTool: DirectorTool = "select";
  private writable = false;
  private viewHelpers = { names: true, paths: true, height: false };
  private nameHelpers: THREE.Object3D[] = [];
  private pathHelper = new THREE.Group();
  private heightHelper = new THREE.Group();
  private previewView = "active";
  private previewVisible = true;
  private previewWidth = 320;
  private loadingAssets = 0;
  private assetErrors = false;
  private down: [number, number] | null = null;
  private placement: string | null = null;
  private gesture!: DirectorTransaction<{
    object: THREE.Object3D;
    selection: DirectorSelection;
    transform: DirectorTransform;
    targetCameraID: string | null;
  }>;
  private removeGestureListeners: (() => void) | null = null;
  private keys = new Set<string>();
  private lastDraw = 0;
  private box: THREE.BoxHelper | null = null;
  private boxTarget: THREE.Object3D | null = null;
  private grid: THREE.GridHelper | null = null;
  private helpersForTargets: {
    helper: THREE.Object3D;
    target: THREE.Object3D;
  }[] = [];
  constructor(
    private host: HTMLElement,
    private callbacks: DirectorSceneCallbacks,
    private compact = false,
  ) {
    this.renderer = new THREE.WebGLRenderer({ antialias: true, alpha: false });
    this.renderer.setPixelRatio(Math.min(window.devicePixelRatio || 1, 2));
    this.renderer.shadowMap.enabled = true;
    this.renderer.shadowMap.type = THREE.PCFSoftShadowMap;
    this.renderer.toneMapping = THREE.ACESFilmicToneMapping;
    this.renderer.domElement.setAttribute("aria-label", "导演台三维场景");
    this.renderer.domElement.setAttribute("role", "img");
    host.appendChild(this.renderer.domElement);
    this.inputSurface = document.createElement("div");
    this.inputSurface.setAttribute("aria-label", "导演视角交互区域");
    this.inputSurface.tabIndex = 0;
    Object.assign(this.inputSurface.style, {
      position: "absolute",
      inset: "0",
      touchAction: "none",
      zIndex: "1",
    });
    host.appendChild(this.inputSurface);
    this.directorCamera.position.set(7, 5, 9);
    this.orbit = new OrbitControls(this.directorCamera, this.inputSurface);
    this.orbit.target.set(0, 1, 0);
    this.orbit.update();
    this.gizmo = new TransformControls(this.directorCamera, this.inputSurface);
    this.gizmo.setSize(0.8);
    this.scene.add(this.gizmo.getHelper());
    this.gizmo.addEventListener("dragging-changed", (event) => {
      this.orbit.enabled =
        !event.value &&
        this.mainView === "director" &&
        !this.contextUnavailable;
    });
    this.gesture = createDirectorTransaction({
      read: () =>
        this.gizmo.object && this.selection
          ? {
              object: this.gizmo.object,
              selection: this.selection,
              transform: readDirectorTransform(this.gizmo.object),
              targetCameraID: this.targetCameraID,
            }
          : null,
      restore: (snapshot) => {
        applyDirectorTransform(snapshot.object, snapshot.transform);
        this.callbacks.onGestureCancel?.();
      },
      commit: (from, to) => {
        if (from.targetCameraID)
          this.callbacks.onTargetTransform?.(
            from.targetCameraID,
            to.transform.position,
          );
        else this.callbacks.onTransform(from.selection, to.transform);
      },
      setActive: (active) => {
        this.orbit.enabled =
          !active && this.mainView === "director" && !this.contextUnavailable;
        if (active) this.callbacks.onGestureStart();
      },
      terminateDrag: () => this.gizmo.pointerUp(null),
    });
    this.removeGestureListeners = installDirectorTerminalListeners(
      this.gesture,
      { window, document, isHidden: () => document.hidden },
    );
    this.gizmo.addEventListener("mouseDown", () => this.gesture.begin());
    this.gizmo.addEventListener("mouseUp", () => this.gesture.end("commit"));
    this.renderer.domElement.tabIndex = 0;
    this.inputSurface.addEventListener("keydown", this.keyDown);
    window.addEventListener("keyup", this.keyUp);
    window.addEventListener("blur", this.clearKeys);
    this.inputSurface.addEventListener("pointerdown", this.pointerDown);
    this.inputSurface.addEventListener("pointerup", this.pointerUp);
    this.inputSurface.addEventListener("pointermove", this.pointerMove);
    this.renderer.domElement.addEventListener(
      "webglcontextlost",
      this.contextLost,
    );
    this.renderer.domElement.addEventListener(
      "webglcontextrestored",
      this.contextRestored,
    );
    this.observer = new ResizeObserver(() => this.resize());
    this.observer.observe(host);
    this.resize();
    const draw = (now: number) => {
      if (this.disposed) return;
      this.navigate(Math.min((now - this.lastDraw) / 1000, 0.05));
      this.lastDraw = now;
      this.render();
      this.frame = requestAnimationFrame(draw);
    };
    this.frame = requestAnimationFrame(draw);
  }
  private keyDown = (event: KeyboardEvent) => {
    if (event.isComposing || event.ctrlKey || event.metaKey || event.altKey)
      return;
    const key = event.key.toLowerCase();
    if ("qweas dijkl".replaceAll(" ", "").includes(key) || key === "shift") {
      this.keys.add(key);
      event.preventDefault();
    }
  };
  private keyUp = (event: KeyboardEvent) => {
    this.keys.delete(event.key.toLowerCase());
  };
  private clearKeys = () => {
    this.keys.clear();
  };
  private navigate(delta: number) {
    if (
      !this.keys.size ||
      this.gesture.active() ||
      document.activeElement !== this.inputSurface ||
      this.mainView !== "director"
    )
      return;
    const speed = (this.keys.has("shift") ? 12 : 3) * delta;
    const direction = new THREE.Vector3();
    this.directorCamera.getWorldDirection(direction);
    const right = new THREE.Vector3()
      .crossVectors(direction, this.directorCamera.up)
      .normalize();
    const movement = new THREE.Vector3()
      .addScaledVector(
        direction,
        Number(this.keys.has("w")) - Number(this.keys.has("s")),
      )
      .addScaledVector(
        right,
        Number(this.keys.has("d")) - Number(this.keys.has("a")),
      );
    movement.y += Number(this.keys.has("e")) - Number(this.keys.has("q"));
    movement.multiplyScalar(speed);
    this.directorCamera.position.add(movement);
    this.orbit.target.add(movement);
    const yaw =
      (Number(this.keys.has("j")) - Number(this.keys.has("l"))) * delta;
    const pitch =
      (Number(this.keys.has("i")) - Number(this.keys.has("k"))) * delta;
    const offset = this.orbit.target.clone().sub(this.directorCamera.position);
    offset
      .applyAxisAngle(new THREE.Vector3(0, 1, 0), yaw)
      .applyAxisAngle(right, pitch);
    this.orbit.target.copy(this.directorCamera.position).add(offset);
    this.orbit.update();
  }
  private resize() {
    this.width = Math.max(1, this.host.clientWidth);
    this.height = Math.max(1, this.host.clientHeight);
    this.renderer.setSize(this.width, this.height);
    const editor = this.layout().editor;
    this.inputSurface.style.width = `${editor.width}px`;
    this.inputSurface.style.pointerEvents =
      this.compact || this.mainView !== "director" ? "none" : "auto";
    const preview = this.layout().preview;
    this.inputSurface.style.clipPath =
      preview && !this.split && !this.compact
        ? `polygon(0 0, ${preview.x}px 0, ${preview.x}px ${preview.y + preview.height}px, 100% ${preview.y + preview.height}px, 100% 100%, 0 100%)`
        : "none";
    this.directorCamera.aspect = editor.width / editor.height;
    this.directorCamera.updateProjectionMatrix();
  }
  private contextLost = (event: Event) => {
    event.preventDefault();
    this.contextUnavailable = true;
    this.gesture.end("cancel");
    this.clearKeys();
    this.orbit.enabled = false;
    this.abort.abort();
    this.callbacks.onContextState?.(true);
  };
  private contextRestored = () => {
    if (this.disposed) return;
    this.contextUnavailable = false;
    const config = this.model;
    if (config) {
      this.rebuild(config);
      this.update(
        config,
        this.currentTime,
        this.selection,
        this.lastTool,
        this.writable,
      );
    }
    this.resize();
    this.orbit.enabled = this.mainView === "director";
    this.callbacks.onContextState?.(false);
  };
  restoreContext() {
    this.renderer
      .getContext()
      .getExtension("WEBGL_lose_context")
      ?.restoreContext();
  }
  private layout() {
    const ratio = this.model?.aspect_ratio?.split(":").map(Number);
    const aspect =
      ratio?.length === 2 && ratio[0] > 0 && ratio[1] > 0
        ? ratio[0] / ratio[1]
        : 16 / 9;
    return directorViewLayout(this.width, this.height, aspect, {
      split: this.split,
      compact: this.compact,
      cameraMain: this.cameras.has(this.mainView),
      previewVisible: this.previewVisible,
      previewWidth: this.previewWidth,
    });
  }
  setTargetCamera(id: string | null) {
    this.targetCameraID = id;
  }
  standardView(
    view:
      "front" | "back" | "left" | "right" | "top" | "bottom" | "perspective",
  ) {
    const vectors = {
      front: [0, 0, 1],
      back: [0, 0, -1],
      left: [-1, 0, 0],
      right: [1, 0, 0],
      top: [0, 1, 0.0001],
      bottom: [0, -1, 0.0001],
      perspective: [5, 3, 6],
    };
    const distance = Math.max(
      1,
      this.directorCamera.position.distanceTo(this.orbit.target),
    );
    this.directorCamera.position
      .copy(this.orbit.target)
      .add(
        new THREE.Vector3()
          .fromArray(vectors[view])
          .normalize()
          .multiplyScalar(distance),
      );
    this.orbit.update();
  }
  groundSelection(selection: DirectorSelection): DirectorTransform | null {
    const target = this.targets.get(directorKey(selection));
    if (!target || this.loadingAssets || this.assetErrors) return null;
    return {
      ...readDirectorTransform(target),
      position: directorGroundPosition(target, this.model?.ground?.height ?? 0),
    };
  }
  private placePosition(point: number[]): number[] {
    const target = this.targets.get(`object:${this.placement}`);
    if (!target) return point;
    target.position.fromArray(point);
    const placed = directorGroundPosition(
      target,
      this.model?.ground?.height ?? 0,
    );
    target.position.fromArray(placed);
    return placed;
  }
  setPlacement(id: string | null) {
    this.placement = id;
  }
  private pointerRay(event: PointerEvent) {
    const bounds = this.renderer.domElement.getBoundingClientRect();
    const x = event.clientX - bounds.left,
      y = event.clientY - bounds.top;
    const layout = this.layout();
    if (layout.preview && directorViewPoint(layout.preview, x, y)) return null;
    const point = directorViewPoint(layout.main, x, y);
    if (!point) return null;
    const ray = new THREE.Raycaster();
    ray.setFromCamera(
      new THREE.Vector2(...point),
      this.cameras.get(this.mainView) ?? this.directorCamera,
    );
    return ray;
  }
  private groundPosition(event: PointerEvent) {
    const ray = this.pointerRay(event);
    if (!ray) return null;
    const point = ray.ray.intersectPlane(
      new THREE.Plane(
        new THREE.Vector3(0, 1, 0),
        -(this.model?.ground?.height ?? 0),
      ),
      new THREE.Vector3(),
    );
    return point ? this.stage.worldToLocal(point).toArray() : null;
  }
  private pointerMove = (event: PointerEvent) => {
    if (!this.placement) return;
    const point = this.groundPosition(event),
      target = this.targets.get(`object:${this.placement}`);
    if (point && target) this.placePosition(point);
  };
  private pointerDown = (event: PointerEvent) => {
    this.inputSurface.focus({ preventScroll: true });
    this.down = [event.clientX, event.clientY];
  };
  private pointerUp = (event: PointerEvent) => {
    if (event.button !== 0) {
      this.down = null;
      return;
    }
    if (
      this.placement &&
      event.button === 0 &&
      this.down &&
      Math.hypot(event.clientX - this.down[0], event.clientY - this.down[1]) < 4
    ) {
      const point = this.groundPosition(event);
      this.down = null;
      if (point) this.callbacks.onPlace?.(this.placePosition(point));
      return;
    }
    if (
      !this.down ||
      Math.hypot(event.clientX - this.down[0], event.clientY - this.down[1]) >
        4 ||
      this.gizmo.axis ||
      this.compact
    ) {
      this.down = null;
      return;
    }
    this.down = null;
    const ray = this.pointerRay(event);
    if (!ray) return;
    const hit = ray
      .intersectObjects(
        [
          ...[...this.targets.values()].filter((object) => object.visible),
          ...[...this.cameraTargets.values()].filter(
            (object) => object.visible,
          ),
          ...this.helpersForTargets
            .filter(
              (item) =>
                item.helper.visible && item.helper.userData.directorSelection,
            )
            .map((item) => item.helper),
        ],
        true,
      )
      .find((item) => item.object.visible);
    let object = hit?.object;
    while (object && !object.userData.directorSelection)
      object = object.parent ?? undefined;
    if (object?.userData.targetCameraID)
      this.callbacks.onTargetSelect?.(object.userData.targetCameraID);
    else this.callbacks.onSelect(object?.userData.directorSelection ?? null);
  };
  update(
    config: Director,
    time: number,
    selection: DirectorSelection | null,
    tool: DirectorTool,
    writable: boolean,
  ) {
    this.lastTool = tool;
    this.writable = writable;
    if (this.contextUnavailable) {
      this.model = config;
      this.selection = selection;
      this.currentTime = time;
      return;
    }
    if (this.model !== config) {
      this.model = config;
      this.rebuild(config);
      this.resize();
    }
    this.selection = selection;
    this.currentTime = time;
    for (const [id, motion] of this.motions)
      motion.sample(
        time,
        config.mode === "dynamic" || id === this.previewObjectID,
      );
    for (const object of config.objects) {
      const target = this.targets.get(`object:${object.id}`);
      if (!target || (this.gizmo.dragging && this.gizmo.object === target))
        continue;
      applyDirectorTransform(
        target,
        config.mode === "dynamic"
          ? sampleDirectorTransform(object.transform, object.keyframes, time)
          : object.transform,
      );
      target.visible = object.visible;
    }
    for (const camera of config.cameras) {
      const target = this.cameras.get(camera.id)!;
      if (this.gizmo.dragging && this.gizmo.object === target) continue;
      const optical =
        config.mode === "dynamic"
          ? sampleDirectorCamera(camera, time)
          : { ...camera, roll: camera.roll ?? 0 };
      const lookTarget = this.cameraTargets.get(camera.id);
      if (
        lookTarget &&
        !(this.gizmo.dragging && this.gizmo.object === lookTarget)
      )
        lookTarget.position.fromArray(optical.target);
      if (lookTarget)
        lookTarget.visible =
          camera.look_at_mode !== "rotation" && !camera.look_at_object_id;
      applyDirectorTransform(target, optical.transform);
      target.fov = optical.fov;
      target.updateProjectionMatrix();
      if (camera.follow_object_id) {
        const followed = this.targets.get(`object:${camera.follow_object_id}`);
        if (followed)
          target.position
            .add(followed.position)
            .add(
              new THREE.Vector3(...tuple(camera.follow_anchor ?? [0, 0, 0])),
            );
      }
      if (camera.look_at_mode !== "rotation") {
        const look = camera.look_at_object_id
          ? this.targets
              .get(`object:${camera.look_at_object_id}`)
              ?.getWorldPosition(new THREE.Vector3())
          : null;
        target.lookAt(
          look ??
            this.stage.localToWorld(
              new THREE.Vector3(...tuple(optical.target)),
            ),
        );
      }
      target.userData.directorRoll = optical.roll;
      target.rotateZ(optical.roll);
    }
    const entity = directorEntity(config, selection),
      target = this.targetCameraID
        ? this.cameraTargets.get(this.targetCameraID)
        : selection
          ? this.targets.get(directorKey(selection))
          : undefined;
    if (
      target &&
      target.visible &&
      entity &&
      !entity.locked &&
      writable &&
      tool !== "select" &&
      !this.compact
    ) {
      this.gizmo.setMode(this.targetCameraID ? "translate" : tool);
      if (this.gizmo.object !== target) this.gizmo.attach(target);
    } else this.gizmo.detach();
    this.gizmo.setTranslationSnap(config.grid_snap ? 0.25 : null);
    this.gizmo.setRotationSnap(config.grid_snap ? Math.PI / 12 : null);
    const desiredBox = target && selection?.kind === "object" ? target : null;
    if (desiredBox !== this.boxTarget) {
      if (this.box) {
        this.helpers.remove(this.box);
        this.box.dispose();
      }
      this.boxTarget = desiredBox;
      this.box = desiredBox ? new THREE.BoxHelper(desiredBox, 0x75b7ff) : null;
      if (this.box) this.helpers.add(this.box);
    }
  }
  private rebuild(config: Director) {
    this.gesture.end("cancel");
    this.abort.abort();
    this.abort = new AbortController();
    for (const motion of this.motions.values()) motion.dispose();
    this.motions.clear();
    this.loadingAssets = 0;
    this.assetErrors = false;
    this.gizmo.detach();
    this.scene.remove(this.content, this.helpers);
    disposeDirectorObject(this.content);
    disposeDirectorObject(this.helpers);
    this.content = new THREE.Group();
    this.helpers = new THREE.Group();
    this.stage = new THREE.Group();
    this.content.add(this.stage);
    this.scene.add(this.content, this.helpers);
    this.targets.clear();
    this.cameras.clear();
    this.cameraTargets.clear();
    this.targetLines.clear();
    this.targetHelpers = new THREE.Group();
    this.targetHelpers.matrixAutoUpdate = false;
    this.helpers.add(this.targetHelpers);
    this.helpersForTargets = [];
    this.nameHelpers = [];
    this.box = null;
    this.boxTarget = null;
    if (this.scene.background instanceof THREE.Texture)
      this.scene.background.dispose();
    this.scene.background = new THREE.Color(config.background);
    this.renderer.toneMappingExposure = config.exposure ?? 1.05;
    if (config.stage_transform) {
      const s = config.stage_transform;
      this.stage.position.fromArray(s.position!);
      this.stage.rotation.set(...tuple(s.rotation!));
      this.stage.scale.setScalar(s.scale!);
    }
    this.content.add(
      new THREE.HemisphereLight(
        0xffffff,
        0x303040,
        config.environment_intensity,
      ),
    );
    this.grid = new THREE.GridHelper(100, 100, 0x555555, 0x333333);
    this.grid.visible = config.grid_visible;
    this.helpers.add(this.grid);
    if (config.ground?.visible) {
      const ground = new THREE.Mesh(
        new THREE.PlaneGeometry(200, 200),
        new THREE.MeshStandardMaterial({
          color: config.ground.color ?? "#242424",
          transparent: true,
          opacity: config.ground.opacity ?? 1,
          roughness: 0.85,
          depthWrite: (config.ground.opacity ?? 1) === 1,
        }),
      );
      ground.rotation.x = -Math.PI / 2;
      ground.position.y = (config.ground.height ?? 0) - 0.005;
      ground.receiveShadow = true;
      this.content.add(ground);
    }
    if (config.panorama?.asset_id) {
      const signal = this.abort.signal;
      this.loadingAssets++;
      const background: DirectorObject = {
        id: config.panorama.asset_id,
        name: config.panorama.name ?? "全景背景",
        kind: "billboard",
        asset_id: config.panorama.asset_id,
        transform: {
          position: [0, 0, 0],
          rotation: [0, 0, 0],
          scale: [1, 1, 1],
        },
        color: "#ffffff",
        visible: true,
        cast_shadow: false,
        receive_shadow: false,
        keyframes: [],
      };
      void this.callbacks
        .loadAsset(background, signal)
        .then((loaded) => {
          if (signal.aborted || this.disposed) {
            disposeDirectorObject(loaded);
            return;
          }
          let texture: THREE.Texture | null = null;
          loaded.traverse((node) => {
            if (
              node instanceof THREE.Mesh &&
              node.material instanceof THREE.MeshStandardMaterial &&
              node.material.map
            )
              texture = node.material.map.clone();
          });
          disposeDirectorObject(loaded);
          if (!texture) throw new Error("全景背景无法读取");
          const panorama = texture as THREE.Texture;
          panorama.mapping = THREE.EquirectangularReflectionMapping;
          panorama.colorSpace = THREE.SRGBColorSpace;
          panorama.needsUpdate = true;
          this.scene.background = panorama;
          this.scene.backgroundRotation.set(
            0,
            config.panorama?.rotation ?? 0,
            0,
          );
        })
        .catch((error) => {
          if (!signal.aborted) {
            this.assetErrors = true;
            this.callbacks.onError(
              error instanceof Error ? error.message : "背景加载失败",
            );
          }
        })
        .finally(() => {
          if (!signal.aborted) this.loadingAssets--;
        });
    }
    for (const object of config.objects) {
      const group = new THREE.Group();
      group.name = object.name;
      group.userData.directorSelection = { kind: "object", id: object.id };
      this.stage.add(group);
      this.targets.set(`object:${object.id}`, group);
      const label = directorNameLabel(object.name);
      this.helpers.add(label);
      this.nameHelpers.push(label);
      this.helpersForTargets.push({ helper: label, target: group });
      if (
        object.kind === "primitive" ||
        (object.kind === "actor" &&
          object.builtin_actor === "mannequin" &&
          !object.asset_id)
      )
        group.add(createDirectorPrimitive(object));
      else {
        const signal = this.abort.signal;
        this.loadingAssets++;
        void this.callbacks
          .loadAsset(object, signal)
          .then((loaded) => {
            if (signal.aborted || this.disposed) {
              disposeDirectorObject(loaded);
              return;
            }
            loaded.traverse((child) => {
              if (child instanceof THREE.Mesh) {
                child.castShadow = object.cast_shadow;
                child.receiveShadow = object.receive_shadow;
              }
            });
            group.add(loaded);
            try {
              const motion = new DirectorModelMotion(loaded, object);
              this.motions.set(object.id, motion);
              this.callbacks.onModelInfo?.(directorModelInfo(loaded, object));
              motion.sample(
                this.currentTime,
                this.model?.mode === "dynamic" ||
                  object.id === this.previewObjectID,
              );
            } catch (cause) {
              this.assetErrors = true;
              throw cause;
            }
          })
          .catch((error) => {
            if (!signal.aborted) {
              this.assetErrors = true;
              this.callbacks.onError(
                `${object.name}：${error instanceof Error ? error.message : "素材加载失败"}`,
              );
            }
          })
          .finally(() => {
            if (!signal.aborted) this.loadingAssets--;
          });
      }
    }
    for (const camera of config.cameras) {
      const item = new THREE.PerspectiveCamera(
        camera.fov,
        16 / 9,
        camera.near,
        camera.far,
      );
      item.userData.directorSelection = { kind: "camera", id: camera.id };
      this.stage.add(item);
      this.targets.set(`camera:${camera.id}`, item);
      this.cameras.set(camera.id, item);
      const lookTarget = new THREE.Mesh(
        new THREE.OctahedronGeometry(0.16),
        new THREE.MeshBasicMaterial({ color: 0x75b7ff, depthTest: false }),
      );
      lookTarget.position.fromArray(camera.target);
      lookTarget.userData.directorSelection = { kind: "camera", id: camera.id };
      lookTarget.userData.targetCameraID = camera.id;
      this.targetHelpers.add(lookTarget);
      this.cameraTargets.set(camera.id, lookTarget);
      const lookLine = new THREE.Line(
        new THREE.BufferGeometry().setFromPoints([
          new THREE.Vector3(),
          new THREE.Vector3(),
        ]),
        new THREE.LineBasicMaterial({
          color: 0x75b7ff,
          transparent: true,
          opacity: 0.5,
        }),
      );
      this.targetHelpers.add(lookLine);
      this.targetLines.set(camera.id, lookLine);
      const helper = new THREE.CameraHelper(item);
      helper.userData.directorSelection = { kind: "camera", id: camera.id };
      this.helpers.add(helper);
      this.helpersForTargets.push({ helper, target: item });
    }
    this.pathHelper = directorPathHelpers(config);
    this.pathHelper.matrixAutoUpdate = false;
    this.heightHelper = directorHeightReference();
    this.helpers.add(this.pathHelper, this.heightHelper);
    for (const configLight of config.lights) {
      const light =
        configLight.type === "ambient"
          ? new THREE.AmbientLight(configLight.color, configLight.intensity)
          : configLight.type === "point"
            ? new THREE.PointLight(
                configLight.color,
                configLight.intensity,
                0,
                2,
              )
            : configLight.type === "spot"
              ? new THREE.SpotLight(
                  configLight.color,
                  configLight.intensity,
                  0,
                  configLight.angle ?? Math.PI / 6,
                  configLight.penumbra ?? 0.3,
                )
              : new THREE.DirectionalLight(
                  configLight.color,
                  configLight.intensity,
                );
      applyDirectorTransform(light, configLight.transform);
      light.userData.directorSelection = { kind: "light", id: configLight.id };
      if ("castShadow" in light) light.castShadow = configLight.cast_shadow;
      this.stage.add(light);
      this.targets.set(`light:${configLight.id}`, light);
      if (
        light instanceof THREE.DirectionalLight ||
        light instanceof THREE.SpotLight
      ) {
        light.target.position
          .copy(light.position)
          .add(new THREE.Vector3(0, 0, -1).applyEuler(light.rotation));
        this.stage.add(light.target);
      }
      const proxy = new THREE.Mesh(
        new THREE.SphereGeometry(0.12, 8, 6),
        new THREE.MeshBasicMaterial({ color: configLight.color }),
      );
      proxy.userData.directorSelection = { kind: "light", id: configLight.id };
      this.helpers.add(proxy);
      this.helpersForTargets.push({ helper: proxy, target: light });
    }
  }
  setViews(
    main: string,
    preview: string,
    visible: boolean,
    width: number,
    helpers?: {
      names?: boolean;
      paths?: boolean;
      height?: boolean;
      split?: boolean;
    },
  ) {
    this.viewHelpers = {
      names: helpers?.names ?? true,
      paths: helpers?.paths ?? true,
      height: helpers?.height ?? false,
    };
    this.mainView = main;
    this.split = helpers?.split ?? false;
    this.previewView = preview;
    this.previewVisible = visible;
    this.previewWidth = width;
    this.orbit.enabled = main === "director";
    this.gizmo.enabled = main === "director";
    this.resize();
  }
  restoreDirectorView(view: {
    position: number[];
    target: number[];
    fov: number;
  }) {
    this.directorCamera.position.fromArray(view.position);
    this.directorCamera.fov = view.fov;
    this.directorCamera.updateProjectionMatrix();
    this.orbit.target.fromArray(view.target);
    this.orbit.update();
  }
  directorView() {
    return {
      position: this.directorCamera.position.toArray(),
      target: this.orbit.target.toArray(),
      fov: this.directorCamera.fov,
    };
  }
  focus(selection: DirectorSelection | null) {
    const target = this.targetCameraID
      ? this.cameraTargets.get(this.targetCameraID)
      : selection
        ? this.targets.get(directorKey(selection))
        : null;
    const bounds = target ? new THREE.Box3().setFromObject(target) : null;
    const center = target
      ? bounds && !bounds.isEmpty()
        ? bounds.getCenter(new THREE.Vector3())
        : target.getWorldPosition(new THREE.Vector3())
      : new THREE.Vector3(0, 1, 0);
    if (!Number.isFinite(center.x)) center.set(0, 1, 0);
    this.orbit.target.copy(center);
    this.directorCamera.position.copy(center).add(new THREE.Vector3(5, 3, 6));
    this.orbit.update();
  }
  private render() {
    if (!this.model || this.contextUnavailable) return;
    for (const { helper, target } of this.helpersForTargets) {
      if (helper instanceof THREE.CameraHelper) helper.update();
      else helper.position.copy(target.getWorldPosition(new THREE.Vector3()));
      helper.visible = target.visible;
      if (
        target instanceof THREE.DirectionalLight ||
        target instanceof THREE.SpotLight
      )
        target.target.position
          .copy(target.position)
          .add(new THREE.Vector3(0, 0, -1).applyEuler(target.rotation));
    }
    this.box?.update();
    this.stage.updateMatrixWorld();
    this.pathHelper.matrix.copy(this.stage.matrixWorld);
    this.targetHelpers.matrix.copy(this.stage.matrixWorld);
    this.targetHelpers.updateMatrixWorld(true);
    for (const config of this.model.cameras) {
      const line = this.targetLines.get(config.id),
        point = this.cameraTargets.get(config.id),
        position = this.cameras.get(config.id)?.position;
      if (line && point && position) {
        line.visible = point.visible;
        const vertices = line.geometry.getAttribute("position");
        vertices.setXYZ(0, position.x, position.y, position.z);
        vertices.setXYZ(
          1,
          point.position.x,
          point.position.y,
          point.position.z,
        );
        vertices.needsUpdate = true;
        line.geometry.computeBoundingSphere();
      }
      if (config.look_at_mode === "rotation") continue;
      const camera = this.cameras.get(config.id),
        target = config.look_at_object_id
          ? this.targets.get(`object:${config.look_at_object_id}`)
          : this.cameraTargets.get(config.id);
      if (
        camera &&
        target &&
        !(this.gizmo.dragging && this.gizmo.object === camera)
      ) {
        camera.lookAt(target.getWorldPosition(new THREE.Vector3()));
        camera.rotateZ(camera.userData.directorRoll ?? 0);
      }
    }
    this.pathHelper.visible =
      this.viewHelpers.paths && this.model.mode === "dynamic";
    this.heightHelper.visible = this.viewHelpers.height;
    for (const label of this.nameHelpers)
      label.visible &&=
        this.viewHelpers.names && this.model.labels_visible !== false;
    this.helpers.visible = true;
    this.gizmo.getHelper().visible = !!this.gizmo.object && !this.compact;
    const shot = this.model.shots.find(
        (s) => s.id === this.model!.active_shot_id,
      ),
      camera =
        this.previewView === "active"
          ? shot
            ? this.cameras.get(shot.camera_id)
            : undefined
          : this.cameras.get(this.previewView);
    const layout = this.layout();
    this.renderer.setScissorTest(false);
    this.renderer.setViewport(0, 0, this.width, this.height);
    this.renderer.clear();
    const draw = (
      view: THREE.PerspectiveCamera,
      rect: DirectorViewRect,
      editing: boolean,
    ) => {
      view.aspect = rect.width / rect.height;
      view.updateProjectionMatrix();
      this.helpers.visible = editing;
      this.gizmo.getHelper().visible = editing && !!this.gizmo.object;
      this.renderer.setScissorTest(true);
      this.renderer.setScissor(
        rect.x,
        this.height - rect.y - rect.height,
        rect.width,
        rect.height,
      );
      this.renderer.setViewport(
        rect.x,
        this.height - rect.y - rect.height,
        rect.width,
        rect.height,
      );
      this.renderer.render(this.scene, view);
    };
    const main = this.cameras.get(this.mainView);
    if (!this.compact) draw(main ?? this.directorCamera, layout.main, !main);
    if (camera && layout.preview) draw(camera, layout.preview, false);
    this.renderer.setScissorTest(false);
    this.helpers.visible = !this.compact && !main;
    this.gizmo.getHelper().visible =
      !this.compact && !main && !!this.gizmo.object;
    this.callbacks.onOrientation?.(this.directorCamera.quaternion);
  }

  snapshotPoses(): Record<string, Record<string, DirectorTransform>> {
    if (
      this.loadingAssets ||
      this.assetErrors ||
      this.disposed ||
      this.contextUnavailable
    )
      throw new Error("请等待场景资源加载完成后再冻结");
    return Object.fromEntries(
      [...this.motions].map(([id, motion]) => [id, motion.snapshot()]),
    );
  }
  snapshotNodePoses() {
    if (
      this.loadingAssets ||
      this.assetErrors ||
      this.disposed ||
      this.contextUnavailable
    )
      throw new Error("请等待场景资源加载完成后再冻结");
    return Object.fromEntries(
      [...this.motions].map(([id, motion]) => [id, motion.snapshotNodes()]),
    );
  }
  renderOutputFrame(
    cameraID: string,
    width: number,
    height: number,
    clay = false,
  ): HTMLCanvasElement {
    if (this.loadingAssets) throw new Error("场景素材仍在加载，请稍后拍摄");
    if (this.assetErrors || this.renderer.getContext().isContextLost())
      throw new Error("场景素材或图形上下文不可用，请重新加载后拍摄");
    const config = this.model;
    const camera = this.cameras.get(cameraID);
    if (!config || !camera || this.disposed) throw new Error("机位尚未就绪");
    const ratio = width / height;
    const target = new THREE.WebGLRenderTarget(width, height);
    target.texture.colorSpace = THREE.SRGBColorSpace;
    const previous = this.renderer.getRenderTarget(),
      oldAspect = camera.aspect,
      helpersVisible = this.helpers.visible,
      gizmoVisible = this.gizmo.getHelper().visible;
    const viewport = this.renderer.getViewport(new THREE.Vector4()),
      scissor = this.renderer.getScissor(new THREE.Vector4()),
      scissorTest = this.renderer.getScissorTest();
    const oldMaterial = this.scene.overrideMaterial;
    const clayMaterial = clay
      ? new THREE.MeshStandardMaterial({ color: 0xdddddd, roughness: 1 })
      : null;
    const pixels = new Uint8Array(width * height * 4);
    try {
      this.helpers.visible = false;
      this.gizmo.getHelper().visible = false;
      if (clayMaterial) this.scene.overrideMaterial = clayMaterial;
      camera.aspect = ratio;
      camera.updateProjectionMatrix();
      this.renderer.setScissorTest(false);
      this.renderer.setRenderTarget(target);
      this.renderer.clear();
      this.renderer.render(this.scene, camera);
      this.renderer.readRenderTargetPixels(target, 0, 0, width, height, pixels);
    } finally {
      this.renderer.setRenderTarget(previous);
      camera.aspect = oldAspect;
      camera.updateProjectionMatrix();
      this.helpers.visible = helpersVisible;
      this.gizmo.getHelper().visible = gizmoVisible;
      this.renderer.setViewport(viewport);
      this.renderer.setScissor(scissor);
      this.renderer.setScissorTest(scissorTest);
      this.scene.overrideMaterial = oldMaterial;
      clayMaterial?.dispose();
      target.dispose();
    }
    const canvas = document.createElement("canvas");
    canvas.width = width;
    canvas.height = height;
    const context = canvas.getContext("2d");
    if (!context) throw new Error("浏览器无法编码机位图片");
    const data = context.createImageData(width, height);
    for (let row = 0; row < height; row++)
      data.data.set(
        pixels.subarray(
          (height - 1 - row) * width * 4,
          (height - row) * width * 4,
        ),
        row * width * 4,
      );
    context.putImageData(data, 0, 0);
    return canvas;
  }
  async capture(): Promise<Blob> {
    if (this.previewObjectID) throw new Error("请先结束动作预览再拍摄");
    const config = this.model,
      shot = config?.shots.find((item) => item.id === config.active_shot_id);
    if (!config || !shot) throw new Error("机位尚未就绪");
    const [x, y] = config.aspect_ratio?.split(":").map(Number) ?? [];
    const ratio = x && y ? x / y : this.width / this.height;
    const canvas = this.renderOutputFrame(
      shot.camera_id,
      Math.round(ratio >= 1 ? 1920 : 1920 * ratio),
      Math.round(ratio >= 1 ? 1920 / ratio : 1920),
    );
    return new Promise((resolve, reject) =>
      canvas.toBlob(
        (blob) =>
          blob ? resolve(blob) : reject(new Error("机位图片编码失败")),
        "image/png",
      ),
    );
  }
  async exportVideo(
    options: DirectorVideoOptions,
    signal: AbortSignal,
    progress: (value: number) => void,
  ): Promise<Blob> {
    if (this.previewObjectID) throw new Error("请先结束动作预览再导出");
    const config = this.model,
      time = this.currentTime,
      selection = this.selection,
      tool = this.lastTool,
      writable = this.writable;
    if (!config || config.mode !== "dynamic")
      throw new Error("请先进入动态预演");
    if (this.loadingAssets || this.assetErrors)
      throw new Error("请等待全部场景资源加载完成");
    try {
      return await encodeDirectorVideo(
        config,
        options,
        signal,
        progress,
        (frameTime, camera, width, height) => {
          if (this.disposed || this.model !== config)
            throw new Error("导出期间场景已变化，请重试");
          this.update(config, frameTime, null, "select", false);
          return this.renderOutputFrame(camera, width, height, options.clay);
        },
      );
    } finally {
      if (!this.disposed && this.model === config)
        this.update(config, time, selection, tool, writable);
    }
  }
  dispose() {
    this.gesture.end("cancel");
    this.removeGestureListeners?.();
    this.inputSurface.removeEventListener("keydown", this.keyDown);
    window.removeEventListener("keyup", this.keyUp);
    window.removeEventListener("blur", this.clearKeys);
    this.keys.clear();
    this.disposed = true;
    for (const motion of this.motions.values()) motion.dispose();
    this.motions.clear();
    this.abort.abort();
    cancelAnimationFrame(this.frame);
    this.observer.disconnect();
    this.inputSurface.removeEventListener("pointerdown", this.pointerDown);
    this.inputSurface.removeEventListener("pointerup", this.pointerUp);
    this.inputSurface.removeEventListener("pointermove", this.pointerMove);
    this.renderer.domElement.removeEventListener(
      "webglcontextlost",
      this.contextLost,
    );
    this.renderer.domElement.removeEventListener(
      "webglcontextrestored",
      this.contextRestored,
    );
    this.inputSurface.remove();
    this.gizmo.dispose();
    this.orbit.dispose();
    disposeDirectorObject(this.content);
    disposeDirectorObject(this.helpers);
    if (this.scene.background instanceof THREE.Texture)
      this.scene.background.dispose();
    this.renderer.dispose();
    this.renderer.forceContextLoss();
    this.renderer.domElement.remove();
  }
}
