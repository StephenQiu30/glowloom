"use client";
import type { DirectorNodePoses } from "./director-workspaces";
import { useEffect, useRef, useState } from "react";
import { Button } from "@/components/ui/button";
import * as THREE from "three";
import { getMediaPreview } from "@/api/media";
import { requirePreview } from "@/components/media/library";
import {
  DirectorScene,
  loadDirectorGLB,
  disposeDirectorObject,
  type DirectorTool,
} from "./director-scene";
import type { DirectorModelInfo } from "./director-motion";
import type {
  Director,
  DirectorSelection,
  DirectorTransform,
} from "./director-model";
import { actorModelURL, DIRECTOR_ACTORS } from "./director-assets";
import { attachBuiltinDirectorMotions } from "./director-builtin-motion";
import { reshapeDirectorQuaterniusActor } from "./director-actor-body";
import { directorBuiltinModel } from "./director-model-catalog";
import { DirectorResources } from "./director-resources";
import type { DirectorVideoExporter } from "./director-video-output";
type Props = {
  scene: Director;
  placement?: string | null;
  targetCameraID?: string | null;
  previewObjectID?: string | null;
  onTargetSelect?: (cameraID: string) => void;
  onTargetTransform?: (cameraID: string, target: number[]) => void;
  views?: {
    main: string;
    preview: string;
    visible: boolean;
    width: number;
    names?: boolean;
    paths?: boolean;
    height?: boolean;
    split?: boolean;
  };
  onPlace?: (position: number[]) => void;
  onGroundReady?: (
    ground: ((selection: DirectorSelection) => DirectorTransform | null) | null,
  ) => void;
  time: number;
  selected: DirectorSelection | null;
  tool: DirectorTool;
  writable: boolean;
  projectID: string;
  compact: boolean;
  focus: number;
  onSelect: (v: DirectorSelection | null) => void;
  onGestureStart: () => void;
  onGestureCancel?: () => void;
  onTransform: (s: DirectorSelection, t: DirectorTransform) => void;
  onError: (message: string) => void;
  onModelInfo?: (info: DirectorModelInfo) => void;
  onViewReady?: (
    view: (() => { position: number[]; target: number[]; fov: number }) | null,
  ) => void;
  onVideoReady?: (exporter: DirectorVideoExporter | null) => void;
  onNodePosesReady?: (snapshot: (() => DirectorNodePoses) | null) => void;
  onPosesReady?: (
    snapshot: (() => Record<string, Record<string, DirectorTransform>>) | null,
  ) => void;
  onCaptureReady?: (capture: (() => Promise<Blob>) | null) => void;
};
export function DirectorViewport(props: Props) {
  const [contextLost, setContextLost] = useState(false);
  const [recoveryEpoch, setRecoveryEpoch] = useState(0);
  const savedView = useRef<ReturnType<DirectorScene["directorView"]> | null>(
    null,
  );
  const axisButtons = useRef<(HTMLButtonElement | null)[]>([]);
  const host = useRef<HTMLDivElement>(null),
    runtime = useRef<DirectorScene | null>(null),
    latest = useRef(props);
  useEffect(() => {
    latest.current = props;
  }, [props]);
  useEffect(() => {
    if (!host.current) return;
    let engine: DirectorScene;
    const resources = new DirectorResources();
    try {
      engine = new DirectorScene(
        host.current,
        {
          onContextState: setContextLost,
          onOrientation: (rotation) => {
            const inverse = rotation.clone().invert();
            [
              [0, 0, 1],
              [0, 0, -1],
              [-1, 0, 0],
              [1, 0, 0],
              [0, 1, 0],
              [0, -1, 0],
            ].forEach((direction, index) => {
              const button = axisButtons.current[index];
              if (!button) return;
              const point = new THREE.Vector3()
                .fromArray(direction)
                .applyQuaternion(inverse);
              button.style.left = `${36 + point.x * 27}px`;
              button.style.top = `${36 - point.y * 27}px`;
              button.style.opacity = point.z < 0 ? ".45" : "1";
              button.style.zIndex = point.z < 0 ? "1" : "2";
            });
          },
          onTargetSelect: (id) => latest.current.onTargetSelect?.(id),
          onTargetTransform: (id, target) =>
            latest.current.onTargetTransform?.(id, target),
          onPlace: (position) => latest.current.onPlace?.(position),
          onSelect: (v) => latest.current.onSelect(v),
          onGestureStart: () => latest.current.onGestureStart(),
          onGestureCancel: () => latest.current.onGestureCancel?.(),
          onTransform: (s, t) => latest.current.onTransform(s, t),
          onError: (m) => latest.current.onError(m),
          onModelInfo: (info) => latest.current.onModelInfo?.(info),
          loadAsset: async (object, signal) => {
            if (object.builtin_model && !object.asset_id) {
              const preset = directorBuiltinModel(object.builtin_model);
              const loaded = await resources.load(
                preset.url,
                (lifetime) => loadDirectorGLB(preset.url, lifetime),
                signal,
              );
              const seen = new Set<THREE.Material>();
              loaded.traverse((node) => {
                if (!(node instanceof THREE.Mesh)) return;
                for (const material of Array.isArray(node.material)
                  ? node.material
                  : [node.material]) {
                  if (seen.has(material)) continue;
                  seen.add(material);
                  if (material instanceof THREE.MeshStandardMaterial)
                    material.color.multiply(new THREE.Color(object.color));
                  material.opacity *= object.opacity ?? 1;
                  material.transparent ||= material.opacity < 1;
                }
              });
              return loaded;
            }
            if (
              object.builtin_actor &&
              object.builtin_actor !== "mannequin" &&
              !object.asset_id
            ) {
              const model = await resources.load(
                actorModelURL(object.builtin_actor),
                (lifetime) =>
                  loadDirectorGLB(
                    actorModelURL(object.builtin_actor!),
                    lifetime,
                  ),
                signal,
              );
              try {
                reshapeDirectorQuaterniusActor(
                  model,
                  DIRECTOR_ACTORS.find(
                    (item) => item.id === object.builtin_actor,
                  )!.shape,
                  object.body_profile,
                );
                await attachBuiltinDirectorMotions(model, signal);
                model.traverse((child) => {
                  if (child instanceof THREE.Mesh) {
                    const materials = Array.isArray(child.material)
                      ? child.material
                      : [child.material];
                    materials.forEach((material) => material.dispose());
                    child.material = new THREE.MeshStandardMaterial({
                      color: object.color,
                      roughness: 0.7,
                      opacity: object.opacity ?? 1,
                      transparent: (object.opacity ?? 1) < 1,
                    });
                  }
                });
                return model;
              } catch (error) {
                disposeDirectorObject(model);
                throw error;
              }
            }
            if (!object.asset_id) throw new Error("缺少正式素材身份");
            const assetID = object.asset_id;
            const loaded = await resources.load(
              `${object.kind}:${assetID}`,
              async (lifetime) => {
                const preview = await getMediaPreview(
                  { pid: props.projectID, asset_id: assetID },
                  { signal: lifetime },
                );
                const kind = object.kind === "billboard" ? "image" : "model";
                if (
                  preview.asset?.project_id !== props.projectID ||
                  preview.asset?.kind !== kind
                )
                  throw new Error("素材归属或类型不匹配");
                const url = requirePreview(preview, assetID);
                lifetime.throwIfAborted();
                if (kind === "model") return loadDirectorGLB(url, lifetime);
                const texture = await new THREE.TextureLoader().loadAsync(url);
                texture.colorSpace = THREE.SRGBColorSpace;
                const aspect =
                  (preview.asset.width ?? 1) / (preview.asset.height ?? 1);
                const mesh = new THREE.Mesh(
                  new THREE.PlaneGeometry(aspect * 2, 2),
                  new THREE.MeshStandardMaterial({
                    map: texture,
                    transparent: true,
                    side: THREE.DoubleSide,
                  }),
                );
                if (lifetime.aborted) {
                  disposeDirectorObject(mesh);
                  lifetime.throwIfAborted();
                }
                return mesh;
              },
              signal,
            );
            loaded.traverse((node) => {
              if (!(node instanceof THREE.Mesh)) return;
              for (const material of Array.isArray(node.material)
                ? node.material
                : [node.material]) {
                if (material instanceof THREE.MeshStandardMaterial)
                  material.color.multiply(new THREE.Color(object.color));
                material.opacity *= object.opacity ?? 1;
                material.transparent ||= material.opacity < 1;
              }
            });
            return loaded;
          },
        },
        props.compact,
      );
      runtime.current = engine;
      if (savedView.current) {
        engine.restoreDirectorView(savedView.current);
        savedView.current = null;
      }
      latest.current.onGroundReady?.((selection) =>
        engine.groundSelection(selection),
      );
      latest.current.onViewReady?.(() => engine.directorView());
      latest.current.onVideoReady?.((options, signal, progress) =>
        engine.exportVideo(options, signal, progress),
      );
      latest.current.onPosesReady?.(() => engine.snapshotPoses());
      latest.current.onNodePosesReady?.(() => engine.snapshotNodePoses());
      latest.current.onCaptureReady?.(() => engine.capture());
    } catch (error) {
      resources.dispose();
      latest.current.onError(
        error instanceof Error ? error.message : "当前浏览器无法建立3D视口",
      );
      return;
    }
    return () => {
      latest.current.onGroundReady?.(null);
      latest.current.onCaptureReady?.(null);
      latest.current.onVideoReady?.(null);
      latest.current.onViewReady?.(null);
      latest.current.onPosesReady?.(null);
      latest.current.onNodePosesReady?.(null);
      engine.dispose();
      resources.dispose();
      runtime.current = null;
    };
  }, [props.projectID, props.compact, recoveryEpoch]);
  useEffect(() => {
    const view = props.views;
    if (view)
      runtime.current?.setViews(
        view.main,
        view.preview,
        view.visible,
        view.width,
        view,
      );
  }, [props.views, recoveryEpoch]);
  useEffect(() => {
    runtime.current?.setPlacement(props.placement ?? null);
  }, [props.placement, recoveryEpoch]);
  useEffect(() => {
    runtime.current?.setTargetCamera(props.targetCameraID ?? null);
    runtime.current?.setPreviewObject(props.previewObjectID ?? null);
    runtime.current?.update(
      props.scene,
      props.time,
      props.selected,
      props.tool,
      props.writable,
    );
  }, [
    props.scene,
    props.time,
    props.selected,
    props.tool,
    props.writable,
    props.targetCameraID,
    props.previewObjectID,
    recoveryEpoch,
  ]);
  useEffect(() => {
    if (props.focus) runtime.current?.focus(latest.current.selected);
  }, [props.focus]);
  return (
    <div
      ref={host}
      className="director-viewport"
      data-canvas-shortcuts-ignore
      data-canvas-no-zoom
      onMouseDown={(e) => e.stopPropagation()}
      onPointerDown={(e) => e.stopPropagation()}
    >
      {!props.compact && (props.views?.main ?? "director") === "director" && (
        <div
          role="group"
          aria-label="视角指示器"
          className="absolute bottom-4 left-4 z-3 h-24 w-24 rounded-lg border border-border bg-background/60"
        >
          {(
            [
              ["front", "前"],
              ["back", "后"],
              ["left", "左"],
              ["right", "右"],
              ["top", "顶"],
              ["bottom", "底"],
            ] as const
          ).map(([view, label], index) => (
            <Button
              ref={(node) => {
                axisButtons.current[index] = node;
              }}
              key={view}
              variant="ghost"
              size="icon"
              className="absolute h-6 w-6 rounded-full bg-secondary text-xs"
              aria-label={`切换${label}视图`}
              onClick={() => runtime.current?.standardView(view)}
            >
              {label}
            </Button>
          ))}
          <Button
            variant="ghost"
            size="sm"
            className="absolute bottom-0 left-6 h-5 px-1 text-xs"
            aria-label="切换透视视图"
            onClick={() => runtime.current?.standardView("perspective")}
          >
            透视
          </Button>
        </div>
      )}

      {contextLost && (
        <div
          role="alert"
          className="absolute inset-0 z-10 flex flex-col items-center justify-center gap-3 bg-background/80 text-foreground"
        >
          <p>三维视口暂时中断，场景数据已保留</p>
          <Button
            onClick={() => {
              savedView.current = runtime.current?.directorView() ?? null;
              setContextLost(false);
              setRecoveryEpoch((value) => value + 1);
            }}
          >
            恢复视口
          </Button>
        </div>
      )}
    </div>
  );
}
