"use client";

import { Component, Suspense, useEffect, useRef, type ReactNode } from "react";
import { Canvas, useFrame, useThree } from "@react-three/fiber";
import {
  GizmoHelper,
  GizmoViewport,
  OrbitControls,
  OrthographicCamera,
  PerspectiveCamera,
  TransformControls,
} from "@react-three/drei";
import {
  BasicDepthPacking,
  Group,
  MeshDepthMaterial,
  MeshNormalMaterial,
  MeshStandardMaterial,
  PerspectiveCamera as ThreeCamera,
  Quaternion,
  Vector3,
} from "three";
import { AuthorizedModel, AuthorizedImage } from "./asset-view";
import { recordDirectorCanvas } from "./recording";
import { createDirectorCaptureContext } from "./outputs";
import { cropDirectorCanvas } from "./aspect-ratio";
import {
  directorPoseBoneDeltas,
  interpolateDirectorBoneRotation,
  interpolateDirectorTransform,
} from "./scene";
import { directorGroundSettings } from "./ground";
import {
  directorStageTransform,
  directorStageLocalCamera,
} from "./stage-transform";
import {
  resolveDirectorOrthographicFraming,
  resolveDirectorOrthographicFrustum,
  resolveDirectorViewFraming,
  type DirectorViewMode,
} from "./view-modes";
import type {
  DirectorHumanoidBone,
  DirectorObject,
  DirectorScene,
  DirectorTransform,
} from "./model";

export type DirectorCapture = (
  mode: "beauty" | "depth" | "normal",
) => Promise<File>;
export type DirectorRecord = (
  duration: number,
  fps: 24 | 25 | 30,
  signal: AbortSignal,
) => Promise<File>;
type Props = {
  projectId: string;
  scene: DirectorScene;
  time: number;
  selectedId?: string;
  viewMode: DirectorViewMode;
  transformMode: "translate" | "rotate" | "scale";
  renderMode: "beauty" | "clay" | "depth" | "normal" | "pose";
  readOnly: boolean;
  onSelect: (id: string) => void;
  onTransform: (id: string, transform: DirectorTransform) => void;
  onViewReady: (view: (() => DirectorTransform) | null) => void;
  onModelMetadata: (
    id: string,
    value: {
      boneNames: string[];
      animations: { name: string; duration: number }[];
    },
  ) => void;
  onCaptureReady: (capture: DirectorCapture | null) => void;
  onRecordReady?: (record: DirectorRecord | null) => void;
  onRecordTime?: (time: number) => void;
};
class ViewportBoundary extends Component<
  { children: ReactNode },
  { failed: boolean }
> {
  state = { failed: false };
  static getDerivedStateFromError() {
    return { failed: true };
  }
  render() {
    return this.state.failed ? (
      <p role="alert" className="grid h-full place-items-center p-8 text-sm">
        3D 视口无法启动。请使用支持 WebGL 的浏览器；编辑状态仍可保存。
      </p>
    ) : (
      this.props.children
    );
  }
}
export function DirectorViewport(props: Props) {
  return (
    <ViewportBoundary>
      <Canvas
        shadows="percentage"
        frameloop="demand"
        dpr={[1, 1.5]}
        camera={{ position: [6, 4, 8], fov: 50, near: 0.05, far: 500 }}
        gl={{ preserveDrawingBuffer: false }}
        onPointerMissed={() => props.onSelect("")}
      >
        <Suspense fallback={null}>
          <Stage {...props} />
        </Suspense>
      </Canvas>
    </ViewportBoundary>
  );
}

/** 标签是浏览器叠层；与画布一起清理，不为每个标签创建另一个 React root。 */
function DirectorLabel({
  text,
  position = [0, 0, 0],
}: {
  text: string;
  position?: [number, number, number];
}) {
  const gl = useThree((state) => state.gl);
  const invalidate = useThree((state) => state.invalidate);
  const anchor = useRef<Group>(null);
  const label = useRef<HTMLSpanElement | null>(null);
  const projected = useRef(new Vector3());
  useEffect(() => {
    const parent = gl.domElement.parentElement;
    if (!parent) return;
    const element = document.createElement("span");
    element.textContent = text;
    element.setAttribute("aria-hidden", "true");
    Object.assign(element.style, {
      position: "absolute",
      left: "0",
      top: "0",
      display: "none",
      whiteSpace: "nowrap",
      pointerEvents: "none",
      fontSize: "10px",
      color: "white",
      background: "rgba(0,0,0,0.6)",
      padding: "1px 4px",
      borderRadius: "4px",
      zIndex: "1",
    });
    parent.append(element);
    label.current = element;
    invalidate();
    return () => {
      label.current = null;
      element.remove();
    };
  }, [gl, text, invalidate]);
  useFrame(({ camera, size }) => {
    if (!anchor.current || !label.current) return;
    anchor.current.updateWorldMatrix(true, false);
    const point = anchor.current
      .getWorldPosition(projected.current)
      .project(camera);
    const visible =
      point.z >= -1 &&
      point.z <= 1 &&
      Math.abs(point.x) <= 1 &&
      Math.abs(point.y) <= 1;
    label.current.style.display = visible ? "block" : "none";
    if (visible)
      label.current.style.transform = `translate(-50%, -50%) translate(${((point.x + 1) * size.width) / 2}px, ${((1 - point.y) * size.height) / 2}px)`;
  });
  return <group ref={anchor} position={position} />;
}
function Stage(props: Props) {
  const { scene, time, viewMode, renderMode } = props;
  const gl = useThree((state) => state.gl);
  const threeScene = useThree((state) => state.scene);
  const size = useThree((state) => state.size);
  const get = useThree((state) => state.get);
  const stage = directorStageTransform(scene),
    ground = directorGroundSettings(scene);
  const framing = resolveDirectorViewFraming({
    scene,
    mode: "camera",
    playhead: time,
  });
  const ortho = resolveDirectorOrthographicFraming({ scene, mode: viewMode });
  const frustum = ortho
    ? resolveDirectorOrthographicFrustum({
        ...ortho,
        aspect: size.width / size.height,
      })
    : null;
  const assetsReady = useRef(new Map<string, boolean>());
  const committedRenderMode = useRef(renderMode);
  const snapshots = useRef({ scene, time });
  useEffect(() => {
    snapshots.current = { scene, time };
  }, [scene, time]);
  const pendingRecordFrame = useRef<{
    time: number;
    frames: number;
    resolve: () => void;
    reject: (error: Error) => void;
    timer: ReturnType<typeof setTimeout>;
  } | null>(null);
  const pendingCaptureFrame = useRef<{
    frames: number;
    resolve: () => void;
    reject: (error: Error) => void;
    timer: ReturnType<typeof setTimeout>;
  } | null>(null);
  useEffect(() => {
    committedRenderMode.current = renderMode;
  }, [renderMode]);
  useFrame(() => {
    const recordFrame = pendingRecordFrame.current;
    if (recordFrame) {
      if (
        committedRenderMode.current === "clay" &&
        Math.abs(snapshots.current.time - recordFrame.time) < 1e-6
      )
        recordFrame.frames += 1;
      if (recordFrame.frames >= 2) {
        clearTimeout(recordFrame.timer);
        pendingRecordFrame.current = null;
        recordFrame.resolve();
      } else get().invalidate();
    }
    const pending = pendingCaptureFrame.current;
    if (!pending) return;
    if (committedRenderMode.current === "beauty") pending.frames += 1;
    if (pending.frames >= 2) {
      clearTimeout(pending.timer);
      pendingCaptureFrame.current = null;
      pending.resolve();
    } else get().invalidate();
  });
  const onViewReady = props.onViewReady;
  useEffect(() => {
    onViewReady(() => {
      const camera = get().camera;
      return directorStageLocalCamera(
        directorStageTransform(snapshots.current.scene),
        {
          position: camera.position.toArray(),
          rotation: [camera.rotation.x, camera.rotation.y, camera.rotation.z],
          scale: [1, 1, 1],
        },
      );
    });
    return () => onViewReady(null);
  }, [get, onViewReady]);
  const captureReady = props.onCaptureReady;
  useEffect(() => {
    let cancelled = false;
    captureReady(async (mode) => {
      if (cancelled) throw new Error("导演台已关闭。");
      await new Promise<void>((resolve, reject) => {
        if (pendingCaptureFrame.current) {
          reject(new Error("已有截图正在等待视口。"));
          return;
        }
        const timer = setTimeout(() => {
          pendingCaptureFrame.current = null;
          reject(new Error("场景材质尚未就绪，请重新捕获。"));
        }, 3000);
        pendingCaptureFrame.current = { frames: 0, resolve, reject, timer };
        get().invalidate();
      });
      if (cancelled) throw new Error("导演台已关闭。");
      if (gl.getContext().isContextLost())
        throw new Error("3D 视口上下文已失效，请重新打开导演台后截图。");
      const current = snapshots.current;
      const required = current.scene.objects
        .filter((object) => object.visible && object.assetId)
        .map((object) => object.id);
      if (current.scene.panorama)
        required.push(`panorama:${current.scene.panorama.assetId}`);
      if (required.some((id) => !assetsReady.current.get(id)))
        throw new Error(
          "项目素材仍在载入或已读取失败，请待所有素材就绪后截图。",
        );
      const camera = resolveDirectorViewFraming({
        scene: current.scene,
        mode: "camera",
        playhead: current.time,
      });
      if (!camera) throw new Error("当前摄影机取景无效。");
      const snapshot = new ThreeCamera(
        camera.fov,
        size.width / size.height,
        camera.near,
        camera.far,
      );
      snapshot.position.set(...camera.position);
      snapshot.up.set(...camera.up);
      snapshot.lookAt(...camera.target);
      snapshot.updateMatrixWorld();
      const original = threeScene.overrideMaterial;
      const material =
        mode === "depth"
          ? new MeshDepthMaterial({ depthPacking: BasicDepthPacking })
          : mode === "normal"
            ? new MeshNormalMaterial()
            : null;
      const hidden: { item: import("three").Object3D; visible: boolean }[] = [];
      threeScene.traverse((item) => {
        if (item.userData.directorEditorOnly) {
          hidden.push({ item, visible: item.visible });
          item.visible = false;
        }
      });
      try {
        threeScene.overrideMaterial = material;
        gl.render(threeScene, snapshot);
        const output = cropDirectorCanvas(
          gl.domElement,
          current.scene.aspectRatio ?? "adaptive",
        );
        const blob = await new Promise<Blob>((resolve, reject) =>
          output.toBlob(
            (value) =>
              value
                ? resolve(value)
                : reject(new Error("摄影机截图编码失败。")),
            "image/png",
          ),
        );
        if (cancelled) throw new Error("导演台已关闭。");
        return new File([blob], `${current.scene.title}-${mode}.png`, {
          type: "image/png",
        });
      } finally {
        threeScene.overrideMaterial = original;
        material?.dispose();
        hidden.forEach(({ item, visible }) => {
          item.visible = visible;
        });
        const current = get();
        gl.render(threeScene, current.camera);
        current.invalidate();
      }
    });
    return () => {
      cancelled = true;
      const pending = pendingCaptureFrame.current;
      if (pending) {
        clearTimeout(pending.timer);
        pendingCaptureFrame.current = null;
        pending.reject(new Error("导演台已关闭。"));
      }
      captureReady(null);
    };
  }, [captureReady, get, gl, threeScene, size.width, size.height]);
  const recordReady = props.onRecordReady;
  const recordTime = props.onRecordTime;
  useEffect(() => {
    if (!recordReady || !recordTime) return;
    const lifecycle = new AbortController();
    recordReady(async (duration, fps, signal) => {
      if (lifecycle.signal.aborted || signal.aborted)
        throw new Error("录制已取消。");
      const active = new AbortController();
      const abort = () => active.abort();
      signal.addEventListener("abort", abort, { once: true });
      lifecycle.signal.addEventListener("abort", abort, { once: true });
      const originalScene = snapshots.current.scene;
      const expected = createDirectorCaptureContext(originalScene);
      const frameCanvas = document.createElement("canvas");
      frameCanvas.width = gl.domElement.width;
      frameCanvas.height = gl.domElement.height;
      const frameContext = frameCanvas.getContext("2d");
      const material = new MeshStandardMaterial({
        color: "#aab2ba",
        roughness: 0.8,
      });
      try {
        if (!frameContext) throw new Error("无法创建白膜录制画布。");
        const blob = await recordDirectorCanvas({
          duration,
          fps,
          aspectRatio: originalScene.aspectRatio ?? "adaptive",
          signal: active.signal,
          renderFrame: async (seconds) => {
            if (active.signal.aborted) throw new Error("录制已取消。");
            recordTime(seconds);
            await new Promise<void>((resolve, reject) => {
              if (pendingRecordFrame.current) {
                reject(new Error("已有录制帧正在等待视口。"));
                return;
              }
              const timer = setTimeout(() => {
                pendingRecordFrame.current = null;
                reject(new Error("白膜动画帧尚未就绪，请重试。"));
              }, 3000);
              pendingRecordFrame.current = {
                time: seconds,
                frames: 0,
                resolve,
                reject,
                timer,
              };
              get().invalidate();
            });
            if (active.signal.aborted || gl.getContext().isContextLost())
              throw new Error("录制已取消或3D上下文已失效。");
            const current = snapshots.current;
            const actual = createDirectorCaptureContext(current.scene);
            if (
              actual.sceneId !== expected.sceneId ||
              actual.shotId !== expected.shotId ||
              actual.renderKey !== expected.renderKey
            )
              throw new Error("录制期间场景或镜头已改变，请重试。");
            if (
              gl.domElement.width !== frameCanvas.width ||
              gl.domElement.height !== frameCanvas.height
            )
              throw new Error("录制期间视口尺寸已改变，请重试。");
            if (
              current.scene.objects.some(
                (object) =>
                  object.visible &&
                  object.assetId &&
                  !assetsReady.current.get(object.id),
              )
            )
              throw new Error("场景素材仍在载入或读取失败，请待就绪后录制。");
            const framing = resolveDirectorViewFraming({
              scene: current.scene,
              mode: "camera",
              playhead: current.time,
            });
            if (!framing) throw new Error("当前摄影机取景无效。");
            const camera = new ThreeCamera(
              framing.fov,
              size.width / size.height,
              framing.near,
              framing.far,
            );
            camera.position.set(...framing.position);
            camera.up.set(...framing.up);
            camera.lookAt(...framing.target);
            camera.updateMatrixWorld();
            const previous = threeScene.overrideMaterial;
            const hidden: {
              item: import("three").Object3D;
              visible: boolean;
            }[] = [];
            threeScene.traverse((item) => {
              if (
                item.userData.directorEditorOnly ||
                item.userData.directorPanorama
              ) {
                hidden.push({ item, visible: item.visible });
                item.visible = false;
              }
            });
            try {
              threeScene.overrideMaterial = material;
              gl.render(threeScene, camera);
              frameContext.drawImage(gl.domElement, 0, 0);
              return frameCanvas;
            } finally {
              threeScene.overrideMaterial = previous;
              hidden.forEach(({ item, visible }) => {
                item.visible = visible;
              });
              gl.render(threeScene, get().camera);
              get().invalidate();
            }
          },
        });
        if (active.signal.aborted) throw new Error("录制已取消。");
        const name =
          Array.from(
            originalScene.title.replace(/[\\/\x00-\x1f\x7f]/g, " ").trim(),
          )
            .slice(0, 32)
            .join("") || "导演台";
        return new File([blob], `${name}-白膜.webm`, { type: "video/webm" });
      } finally {
        active.abort();
        signal.removeEventListener("abort", abort);
        lifecycle.signal.removeEventListener("abort", abort);
        const pending = pendingRecordFrame.current;
        if (pending) {
          clearTimeout(pending.timer);
          pendingRecordFrame.current = null;
          pending.reject(new Error("录制已结束。"));
        }
        material.dispose();
      }
    });
    return () => {
      lifecycle.abort();
      const pending = pendingRecordFrame.current;
      if (pending) {
        clearTimeout(pending.timer);
        pendingRecordFrame.current = null;
        pending.reject(
          new Error("录制视口已关闭或布局改变，请待视口稳定后重试。"),
        );
      }
      recordReady(null);
    };
  }, [recordReady, recordTime, get, gl, threeScene, size.width, size.height]);
  useEffect(() => {
    const lost = (event: Event) => {
      event.preventDefault();
    };
    gl.domElement.addEventListener("webglcontextlost", lost);
    return () => gl.domElement.removeEventListener("webglcontextlost", lost);
  }, [gl]);
  return (
    <>
      <color attach="background" args={[scene.background]} />
      <ambientLight intensity={scene.environmentIntensity * 0.4} />
      {scene.panorama ? (
        <group
          userData={{ directorPanorama: true }}
          rotation={[
            0,
            ((scene.panorama.rotation + (scene.panoramaRotation ?? 0)) *
              Math.PI) /
              180,
            0,
          ]}
        >
          <AuthorizedImage
            projectId={props.projectId}
            assetId={scene.panorama.assetId}
            onStatus={(ready) =>
              assetsReady.current.set(
                `panorama:${scene.panorama!.assetId}`,
                ready,
              )
            }
            panorama
            radius={scene.panoramaRadius ?? 60}
          />
        </group>
      ) : null}
      {viewMode === "camera" && framing ? (
        <PerspectiveCamera
          makeDefault
          position={framing.position}
          up={framing.up}
          fov={framing.fov}
          near={framing.near}
          far={framing.far}
          onUpdate={(camera) => {
            camera.lookAt(...framing.target);
            camera.updateProjectionMatrix();
          }}
        />
      ) : ortho && frustum ? (
        <OrthographicCamera
          makeDefault
          position={ortho.position}
          up={ortho.up}
          near={ortho.near}
          far={ortho.far}
          {...frustum}
          onUpdate={(camera) => {
            camera.lookAt(...ortho.target);
            camera.updateProjectionMatrix();
          }}
        />
      ) : (
        <>
          <PerspectiveCamera makeDefault position={[6, 4, 8]} fov={50} />
          <OrbitControls makeDefault target={[0, 1, 0]} />
        </>
      )}
      <group
        position={stage.position}
        rotation={
          stage.rotation.map((degrees) => (degrees * Math.PI) / 180) as [
            number,
            number,
            number,
          ]
        }
        scale={stage.scale}
      >
        {scene.lights.map((light) =>
          light.type === "ambient" ? (
            <ambientLight
              key={light.id}
              color={light.color}
              intensity={light.intensity}
            />
          ) : light.type === "point" ? (
            <pointLight
              key={light.id}
              position={light.transform.position}
              color={light.color}
              intensity={light.intensity}
              castShadow={light.castShadow}
            />
          ) : light.type === "spot" ? (
            <spotLight
              key={light.id}
              position={light.transform.position}
              rotation={light.transform.rotation}
              color={light.color}
              intensity={light.intensity}
              angle={light.angle}
              penumbra={light.penumbra}
              castShadow={light.castShadow}
            />
          ) : (
            <directionalLight
              key={light.id}
              position={light.transform.position}
              color={light.color}
              intensity={light.intensity}
              castShadow={light.castShadow}
            />
          ),
        )}
        {scene.gridVisible ? (
          <gridHelper
            args={[20, 40, "#555555", "#333333"]}
            userData={{ directorEditorOnly: true }}
          />
        ) : null}
        {ground.visible ? (
          <mesh
            rotation={[-Math.PI / 2, 0, 0]}
            position={[0, ground.height - 0.002, 0]}
            receiveShadow
          >
            <planeGeometry args={[200, 200]} />
            <meshStandardMaterial
              color="#343434"
              transparent
              opacity={ground.opacity}
            />
          </mesh>
        ) : null}
        {scene.objects
          .filter((object) => object.visible)
          .map((object) => (
            <ObjectView
              key={object.id}
              onAssetStatus={(id, ready) => assetsReady.current.set(id, ready)}
              onModelMetadata={props.onModelMetadata}
              projectId={props.projectId}
              object={object}
              time={time}
              selected={props.selectedId === object.id}
              transformMode={props.transformMode}
              mode={renderMode}
              labels={scene.labelsVisible !== false}
              canTransform={!props.readOnly && viewMode === "free"}
              gridSnap={scene.gridSnap ?? false}
              onSelect={props.onSelect}
              onTransform={props.onTransform}
            />
          ))}
        {viewMode !== "camera"
          ? scene.cameras.map((camera) => (
              <group
                key={camera.id}
                position={
                  interpolateDirectorTransform(
                    camera.transform,
                    camera.keyframes,
                    time,
                  ).position
                }
                userData={{ directorEditorOnly: true }}
              >
                <mesh
                  onClick={(event) => {
                    event.stopPropagation();
                    props.onSelect(camera.id);
                  }}
                >
                  <boxGeometry args={[0.2, 0.15, 0.3]} />
                  <meshBasicMaterial
                    color={
                      props.selectedId === camera.id ? "#ffd54a" : "#72aaff"
                    }
                  />
                </mesh>
                {scene.labelsVisible ? (
                  <DirectorLabel text={camera.name} />
                ) : null}
              </group>
            ))
          : null}
      </group>
      <GizmoHelper alignment="bottom-right" margin={[65, 65]}>
        <GizmoViewport />
      </GizmoHelper>
    </>
  );
}
function ObjectView({
  onAssetStatus,
  onModelMetadata,
  projectId,
  object,
  time,
  selected,
  transformMode,
  mode,
  labels,
  canTransform,
  gridSnap,
  onSelect,
  onTransform,
}: {
  onAssetStatus: (id: string, ready: boolean) => void;
  onModelMetadata: Props["onModelMetadata"];
  projectId: string;
  object: DirectorObject;
  time: number;
  selected: boolean;
  transformMode: Props["transformMode"];
  mode: Props["renderMode"];
  labels: boolean;
  canTransform: boolean;
  gridSnap: boolean;
  onSelect: Props["onSelect"];
  onTransform: Props["onTransform"];
}) {
  const group = useRef<Group>(null);
  const transform = interpolateDirectorTransform(
    object.transform,
    object.keyframes,
    time,
  );
  const material =
    mode === "normal" ? (
      <meshNormalMaterial />
    ) : mode === "depth" ? (
      <meshDepthMaterial depthPacking={BasicDepthPacking} />
    ) : (
      <meshStandardMaterial
        color={mode === "clay" ? "#aab2ba" : object.color}
        roughness={0.8}
      />
    );
  const content = (
    <group
      ref={group}
      position={transform.position}
      rotation={transform.rotation}
      scale={transform.scale}
      onClick={(event) => {
        event.stopPropagation();
        onSelect(object.id);
      }}
    >
      {object.builtinActor || object.primitive === "character" ? (
        <Mannequin object={object} time={time} material={material} />
      ) : object.kind === "billboard" && object.assetId ? (
        <AuthorizedImage
          projectId={projectId}
          assetId={object.assetId}
          onStatus={(ready) => onAssetStatus(object.id, ready)}
        />
      ) : object.assetId &&
        (object.kind === "model" || object.kind === "actor") ? (
        <AuthorizedModel
          onStatus={(ready) => onAssetStatus(object.id, ready)}
          projectId={projectId}
          onMetadata={(data) => onModelMetadata(object.id, data)}
          assetId={object.assetId}
          object={object}
          time={time}
          mode={mode}
        />
      ) : (
        <mesh
          castShadow={object.castShadow}
          receiveShadow={object.receiveShadow}
        >
          {object.primitive === "sphere" ? (
            <sphereGeometry args={[0.5, 24, 16]} />
          ) : object.primitive === "cylinder" ? (
            <cylinderGeometry args={[0.4, 0.4, 1, 24]} />
          ) : object.primitive === "plane" ? (
            <planeGeometry args={[2, 2]} />
          ) : (
            <boxGeometry args={[1, 1, 1]} />
          )}
          {material}
        </mesh>
      )}
      {labels ? (
        <DirectorLabel text={object.name} position={[0, 2.1, 0]} />
      ) : null}
    </group>
  );
  return selected && canTransform ? (
    <TransformControls
      mode={transformMode}
      translationSnap={gridSnap ? 0.5 : null}
      rotationSnap={gridSnap ? Math.PI / 12 : null}
      onMouseUp={() => {
        const item = group.current;
        if (item)
          onTransform(object.id, {
            position: item.position.toArray(),
            rotation: [item.rotation.x, item.rotation.y, item.rotation.z],
            scale: item.scale.toArray(),
          });
      }}
    >
      {content}
    </TransformControls>
  ) : (
    content
  );
}
function Mannequin({
  object,
  time,
  material,
}: {
  object: DirectorObject;
  time: number;
  material: ReactNode;
}) {
  const deltas = directorPoseBoneDeltas(object.pose ?? "stand");
  const rotations = new Map<
    DirectorHumanoidBone,
    [number, number, number, number]
  >();
  for (const bone of [
    "hips",
    "spine",
    "chest",
    "neck",
    "head",
    "leftUpperArm",
    "rightUpperArm",
    "leftLowerArm",
    "rightLowerArm",
    "leftHand",
    "rightHand",
    "leftUpperLeg",
    "rightUpperLeg",
    "leftLowerLeg",
    "rightLowerLeg",
  ] as const) {
    const pose = deltas[bone] ?? [0, 0, 0, 1];
    const override = object.boneOverrides?.[bone] ?? pose;
    rotations.set(
      bone,
      interpolateDirectorBoneRotation(
        override,
        object.boneTracks?.find((track) => track.bone === bone)?.keyframes ??
          [],
        time,
      ),
    );
  }
  const rotation = (bone: DirectorHumanoidBone) =>
    new Quaternion(...(rotations.get(bone) ?? [0, 0, 0, 1]));
  const limb = (
    upper: "leftUpperArm" | "rightUpperArm" | "leftUpperLeg" | "rightUpperLeg",
    lower: "leftLowerArm" | "rightLowerArm" | "leftLowerLeg" | "rightLowerLeg",
    x: number,
    y: number,
    length: number,
    arm = false,
  ) => (
    <group position={[x, y, 0]} quaternion={rotation(upper)}>
      <mesh
        position={arm ? [length / 2, 0, 0] : [0, -length / 2, 0]}
        rotation={arm ? [0, 0, Math.PI / 2] : [0, 0, 0]}
        castShadow
      >
        <capsuleGeometry args={[0.075, length - 0.15, 4, 8]} />
        {material}
      </mesh>
      <group
        position={arm ? [length, 0, 0] : [0, -length, 0]}
        quaternion={rotation(lower)}
      >
        <mesh
          position={arm ? [length / 2, 0, 0] : [0, -length / 2, 0]}
          rotation={arm ? [0, 0, Math.PI / 2] : [0, 0, 0]}
          castShadow
        >
          <capsuleGeometry args={[0.06, length - 0.12, 4, 8]} />
          {material}
        </mesh>
      </group>
    </group>
  );
  return (
    <group position={[0, 0.95, 0]} quaternion={rotation("hips")}>
      <mesh castShadow>
        <boxGeometry args={[0.34, 0.2, 0.22]} />
        {material}
      </mesh>
      <group position={[0, 0.14, 0]} quaternion={rotation("spine")}>
        <group quaternion={rotation("chest")}>
          <mesh position={[0, 0.22, 0]} castShadow>
            <capsuleGeometry args={[0.2, 0.25, 4, 12]} />
            {material}
          </mesh>
          <group position={[0, 0.5, 0]} quaternion={rotation("neck")}>
            <mesh
              position={[0, 0.18, 0]}
              quaternion={rotation("head")}
              castShadow
            >
              <sphereGeometry args={[0.15, 16, 12]} />
              {material}
            </mesh>
          </group>
          {limb("leftUpperArm", "leftLowerArm", 0.22, 0.35, 0.28, true)}
          <group scale={[-1, 1, 1]}>
            {limb("rightUpperArm", "rightLowerArm", 0.22, 0.35, 0.28, true)}
          </group>
        </group>
      </group>
      {limb("leftUpperLeg", "leftLowerLeg", 0.12, -0.05, 0.42)}
      {limb("rightUpperLeg", "rightLowerLeg", -0.12, -0.05, 0.42)}
    </group>
  );
}
