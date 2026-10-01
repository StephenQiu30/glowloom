"use client";

import { useCallback, useEffect, useRef, useState } from "react";
import {
  Camera,
  Copy,
  Download,
  Lightbulb,
  Pause,
  Play,
  Plus,
  Redo2,
  Save,
  Trash2,
  Undo2,
  User,
} from "lucide-react";
import { Euler, Quaternion } from "three";
import { Button } from "@/components/ui/button";
import { Checkbox } from "@/components/ui/checkbox";
import {
  Dialog,
  DialogContent,
  DialogDescription,
  DialogFooter,
  DialogHeader,
  DialogTitle,
} from "@/components/ui/dialog";
import { Field, FieldLabel } from "@/components/ui/field";
import { Input } from "@/components/ui/input";
import {
  Select,
  SelectContent,
  SelectItem,
  SelectTrigger,
  SelectValue,
} from "@/components/ui/select";
import { Textarea } from "@/components/ui/textarea";
import { Tabs, TabsContent, TabsList, TabsTrigger } from "@/components/ui/tabs";
import { DirectorGallery } from "./gallery";
import {
  createDirectorCaptureContext,
  reconcileDirectorCover,
  type DirectorCaptureContext,
} from "./outputs";
import type { CanvasNodeData } from "../model";
import {
  DIRECTOR_MODES,
  directorModeCapabilities,
  resolveDirectorModeTransition,
  type DirectorMode,
} from "./modes";
import {
  DIRECTOR_CAMERA_PRESETS,
  createDirectorCameraFromPreset,
  type DirectorCameraPresetId,
} from "./camera-presets";
import { cameraMoveTransform } from "./camera-moves";
import {
  resolveDirectorCameraMoveKeyframes,
  resolveDirectorCameraAlignment,
} from "./animation-semantics";
import { inferDirectorRig } from "./rig";
import { interpolateDirectorTransform } from "./scene";
import {
  resolveDirectorObjectTransformEdit,
  resolveDirectorKeyframeRecord,
  snapDirectorTime,
} from "./animation-semantics";
import { DirectorAssetControls } from "./asset-controls";
import { DIRECTOR_ASPECT_RATIOS } from "./aspect-ratio";
import {
  bindDirectorCameraFollow,
  removeDirectorCameraBindingsForObject,
  unbindDirectorCameraFollow,
} from "./camera-binding";
import {
  directorBoneNames,
  directorSchema,
  type DirectorCamera,
  type DirectorHumanoidBone,
  type DirectorObject,
  type DirectorPose,
  type DirectorScene,
  type DirectorTransform,
  type DirectorVec3,
} from "./model";
import { compileDirectorPrompt } from "./prompt-compiler";
import {
  applyDirectorUniformScale,
  createDirectorActor,
  createDirectorLight,
  createDirectorObject,
  directorBoneLabel,
  directorFocalLengthToFov,
  directorPoseLabel,
  removeDirectorBoneKeyframe,
  removeDirectorKeyframe,
  upsertDirectorBoneKeyframe,
} from "./scene";
import { DIRECTOR_VIEW_MODES, type DirectorViewMode } from "./view-modes";
import { DirectorViewport, type DirectorCapture } from "./viewport";

const poses: DirectorPose[] = [
  "stand",
  "neutral",
  "t_pose",
  "walk",
  "run",
  "sit",
  "squat",
  "kneel_single",
  "kneel_double",
  "hands_hips",
  "lean",
  "bow",
  "think",
  "fight",
  "kick",
  "throw",
  "push",
  "wave",
  "reach",
  "arms_crossed",
  "phone",
];
type Props = {
  projectId: string;
  nodes: CanvasNodeData[];
  value: DirectorScene;
  readOnly: boolean;
  onClose: () => void;
  onSave: (scene: DirectorScene) => Promise<boolean>;
  onCapture: (files: File[], context?: DirectorCaptureContext) => void;
  onPrompt: (prompt: string) => void;
  onGenerate: (prompt: string) => Promise<boolean>;
  modelControl?: (
    scene: DirectorScene,
    change: (scene: DirectorScene) => void,
  ) => React.ReactNode;
};
export function DirectorWorkbench({
  projectId,
  nodes,
  value,
  readOnly,
  onClose,
  onSave,
  onCapture,
  onPrompt,
  onGenerate,
  modelControl,
}: Props) {
  const [scene, setScene] = useState(() => directorSchema.parse(value));
  const [selectedId, setSelectedId] = useState(
    scene.objects[0]?.id ?? scene.cameras[0].id,
  );
  const [mode, setMode] = useState<DirectorMode>("layout");
  const [cameraPreset, setCameraPreset] =
    useState<DirectorCameraPresetId>("front-medium");
  const currentView = useRef<(() => DirectorTransform) | null>(null);
  const registerView = useCallback((view: (() => DirectorTransform) | null) => {
    currentView.current = view;
  }, []);
  const capabilities = directorModeCapabilities(mode);
  const [viewMode, setViewMode] = useState<DirectorViewMode>("free");
  const [transformMode, setTransformMode] = useState<
    "translate" | "rotate" | "scale"
  >("translate");
  const [renderMode, setRenderMode] = useState<
    "beauty" | "clay" | "depth" | "normal" | "pose"
  >("beauty");
  const [time, setTime] = useState(0);
  const [autoKey, setAutoKey] = useState(false);
  const [playing, setPlaying] = useState(false);
  const [saving, setSaving] = useState(false);
  const [captureReady, setCaptureReady] = useState(false);
  const [error, setError] = useState("");
  const [history, setHistory] = useState<{
    past: DirectorScene[];
    future: DirectorScene[];
  }>({ past: [], future: [] });
  const [bone, setBone] = useState<DirectorHumanoidBone>("head");
  const capture = useRef<DirectorCapture | null>(null);
  const captureBusy = useRef(false);
  const mounted = useRef(false);
  useEffect(() => {
    mounted.current = true;
    return () => {
      mounted.current = false;
    };
  }, []);
  const activeShot = scene.shots.find(
    (shot) => shot.id === scene.activeShotId,
  )!;
  const selectedObject = scene.objects.find(
    (object) => object.id === selectedId,
  );
  const selectedCamera = scene.cameras.find(
    (camera) => camera.id === selectedId,
  );
  const selectedLight = scene.lights.find((light) => light.id === selectedId);
  const selected = selectedObject ?? selectedCamera ?? selectedLight;
  const disabled = readOnly || saving;
  const prompt = compileDirectorPrompt(scene, activeShot);
  const viewScene = {
    ...scene,
    objects: scene.objects.map((object) =>
      object.assetId || !object.sourceNodeId
        ? object
        : {
            ...object,
            assetId: nodes.find((node) => node.id === object.sourceNodeId)
              ?.assetId,
          },
    ),
  };
  function change(next: DirectorScene) {
    if (disabled || next === scene) return;
    setHistory({ past: [...history.past.slice(-49), scene], future: [] });
    setScene(reconcileDirectorCover(scene, next));
  }
  function patchObject(patch: Partial<DirectorObject>) {
    if (selectedObject)
      change({
        ...scene,
        objects: scene.objects.map((object) =>
          object.id === selectedId ? { ...object, ...patch } : object,
        ),
      });
  }
  function patchCamera(patch: Partial<DirectorCamera>) {
    if (selectedCamera)
      change({
        ...scene,
        cameras: scene.cameras.map((camera) =>
          camera.id === selectedId ? { ...camera, ...patch } : camera,
        ),
      });
  }
  function transform(id: string, next: DirectorTransform) {
    change({
      ...scene,
      objects: scene.objects.map((object) =>
        object.id === id
          ? {
              ...object,
              ...resolveDirectorObjectTransformEdit({
                base: object.transform,
                keyframes: object.keyframes,
                rendered: interpolateDirectorTransform(
                  object.transform,
                  object.keyframes,
                  time,
                ),
                edited: next,
                autoKey,
                time: snapDirectorTime(time, activeShot.fps),
              }),
            }
          : object,
      ),
      cameras: scene.cameras.map((camera) =>
        camera.id === id
          ? {
              ...camera,
              ...resolveDirectorObjectTransformEdit({
                base: camera.transform,
                keyframes: camera.keyframes,
                rendered: interpolateDirectorTransform(
                  camera.transform,
                  camera.keyframes,
                  time,
                ),
                edited: next,
                autoKey,
                time: snapDirectorTime(time, activeShot.fps),
              }),
            }
          : camera,
      ),
      lights: scene.lights.map((light) =>
        light.id === id ? { ...light, transform: next } : light,
      ),
    });
  }
  function undo() {
    if (!history.past.length || disabled) return;
    const previous = history.past.at(-1)!;
    setHistory({
      past: history.past.slice(0, -1),
      future: [...history.future, scene],
    });
    setScene(previous);
  }
  function redo() {
    if (!history.future.length || disabled) return;
    const next = history.future.at(-1)!;
    setHistory({
      past: [...history.past, scene],
      future: history.future.slice(0, -1),
    });
    setScene(next);
  }
  const modelMetadata = useCallback(
    (
      id: string,
      data: {
        boneNames: string[];
        animations: { name: string; duration: number }[];
      },
    ) => {
      setScene((current) => ({
        ...current,
        objects: current.objects.map((object) => {
          if (object.id !== id || object.kind !== "actor") return object;
          const inferred = inferDirectorRig(
            data.boneNames,
            data.animations.map((animation) => animation.name),
          );
          return {
            ...object,
            rig: {
              ...inferred,
              boneMap: { ...inferred.boneMap, ...object.rig?.boneMap },
            },
            motionClips: object.motionClips?.length
              ? object.motionClips
              : data.animations
                  .filter((animation) => animation.duration >= 0.1)
                  .map((animation) => ({
                    id: crypto.randomUUID(),
                    name: animation.name.slice(0, 128) || "动作",
                    sourceAnimation: animation.name.slice(0, 128),
                    start: 0,
                    duration: Math.min(3600, animation.duration),
                    playbackRate: 1,
                    loop: true,
                  })),
          };
        }),
      }));
    },
    [],
  );
  const registerCapture = useCallback((next: DirectorCapture | null) => {
    capture.current = next;
    setCaptureReady(Boolean(next));
  }, []);
  useEffect(() => {
    if (!playing) return;
    const start = performance.now() - time * 1000;
    const timer = setInterval(() => {
      const next = (performance.now() - start) / 1000;
      if (next >= activeShot.duration) {
        setTime(activeShot.duration);
        setPlaying(false);
      } else setTime(next);
    }, 80);
    return () => clearInterval(timer);
    // The clock is anchored by the play event. Scrubbing pauses before setting time.
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [playing, activeShot.duration]);
  async function save(close = true) {
    const result = directorSchema.safeParse(scene);
    if (!result.success) {
      setError(result.error.issues[0].message);
      return false;
    }
    setSaving(true);
    setPlaying(false);
    setError("");
    try {
      const saved = await onSave(result.data);
      if (saved && close) onClose();
      else if (!saved) setError("导演台尚未确认保存，请修复画布错误后重试。");
      return saved;
    } catch {
      setError("导演台保存失败。");
      return false;
    } finally {
      setSaving(false);
    }
  }
  async function takeCapture(mode: "beauty" | "depth" | "normal") {
    if (!capture.current || disabled || captureBusy.current) return;
    captureBusy.current = true;
    try {
      if (!(await save(false)) || !mounted.current || !capture.current) return;
      setSaving(true);
      const context =
        mode === "beauty" ? createDirectorCaptureContext(scene) : undefined;
      // The viewport waits for its own committed beauty frames; the WebGL root
      // commits separately from the surrounding DOM dialog.
      setRenderMode("beauty");
      const file = await capture.current(mode);
      if (mounted.current) onCapture([file], context);
    } catch (failure) {
      if (mounted.current)
        setError(failure instanceof Error ? failure.message : "截图未完成。");
    } finally {
      captureBusy.current = false;
      if (mounted.current) {
        setRenderMode(renderMode);
        setSaving(false);
      }
    }
  }
  function deleteSelected() {
    if (disabled) return;
    if (selectedObject)
      change({
        ...scene,
        objects: scene.objects.filter((object) => object.id !== selectedId),
        cameras: scene.cameras.map((camera) =>
          removeDirectorCameraBindingsForObject(
            camera,
            selectedId,
            scene,
            time,
          ),
        ),
      });
    else if (selectedLight)
      change({
        ...scene,
        lights: scene.lights.filter((light) => light.id !== selectedId),
      });
    else if (selectedCamera && scene.cameras.length > 1) {
      const fallback = scene.cameras.find(
        (camera) => camera.id !== selectedId,
      )!;
      change({
        ...scene,
        cameras: scene.cameras.filter((camera) => camera.id !== selectedId),
        shots: scene.shots.map((shot) =>
          shot.cameraId === selectedId
            ? { ...shot, cameraId: fallback.id }
            : shot,
        ),
      });
    }
  }
  function recordKeyframe() {
    if (!capabilities.keyframes) return;
    if (selectedObject)
      patchObject({
        keyframes: resolveDirectorKeyframeRecord({
          base: selectedObject.transform,
          keyframes: selectedObject.keyframes,
          rawTime: time,
          snappedTime: snapDirectorTime(time, activeShot.fps),
        }).keyframes,
      });
    if (selectedCamera)
      patchCamera({
        keyframes: resolveDirectorKeyframeRecord({
          base: selectedCamera.transform,
          keyframes: selectedCamera.keyframes,
          rawTime: time,
          snappedTime: snapDirectorTime(time, activeShot.fps),
        }).keyframes,
      });
  }
  const frames = selectedObject?.keyframes ?? selectedCamera?.keyframes ?? [];
  const boneRotation = selectedObject?.boneOverrides?.[bone] ?? [0, 0, 0, 1];
  const boneEuler = new Euler().setFromQuaternion(
    new Quaternion(...boneRotation),
  );
  return (
    <Dialog
      open
      onOpenChange={(open) => {
        if (!open && !saving) onClose();
      }}
    >
      <DialogContent
        className="flex h-[94dvh] w-[98vw] flex-col gap-3 overflow-y-auto sm:max-w-[1600px] [&>*]:shrink-0"
        onEscapeKeyDown={(event) => {
          if (saving) event.preventDefault();
        }}
        onKeyDown={(event) => {
          if (
            (event.target as HTMLElement).closest(
              "input,textarea,select,[role=combobox],[contenteditable=true]",
            )
          )
            return;
          if (
            (event.ctrlKey || event.metaKey) &&
            event.key.toLowerCase() === "z"
          ) {
            event.preventDefault();
            event.stopPropagation();
            if (event.shiftKey) redo();
            else undo();
          } else if (["w", "e", "r"].includes(event.key.toLowerCase())) {
            event.stopPropagation();
            setTransformMode(
              event.key.toLowerCase() === "w"
                ? "translate"
                : event.key.toLowerCase() === "e"
                  ? "rotate"
                  : "scale",
            );
          } else if (event.key === "Delete" || event.key === "Backspace") {
            event.preventDefault();
            event.stopPropagation();
            deleteSelected();
          }
        }}
      >
        <DialogHeader className="shrink-0">
          <DialogTitle>{scene.title} · 导演台</DialogTitle>
          <DialogDescription>
            摆场、角色姿态、摄影机与运镜关键帧。W / E / R 切换移动、旋转和缩放。
          </DialogDescription>
        </DialogHeader>
        <div
          className="flex shrink-0 flex-wrap gap-2"
          role="group"
          aria-label="导演台创作模式"
        >
          {DIRECTOR_MODES.map((item) => (
            <Button
              key={item.mode}
              variant={mode === item.mode ? "default" : "outline"}
              size="sm"
              disabled={saving}
              onClick={() => {
                const next = resolveDirectorModeTransition({
                  mode: item.mode,
                  playing,
                  autoKey,
                  renderMode,
                });
                setMode(next.mode);
                setPlaying(next.playing);
                setAutoKey(next.autoKey);
                setRenderMode(next.renderMode);
              }}
            >
              {item.label}
            </Button>
          ))}
        </div>
        <div className="flex shrink-0 flex-wrap items-center gap-2">
          <Select
            value={viewMode}
            onValueChange={(mode: DirectorViewMode) => setViewMode(mode)}
          >
            <SelectTrigger className="w-32" aria-label="导演台取景视角">
              <SelectValue />
            </SelectTrigger>
            <SelectContent>
              {DIRECTOR_VIEW_MODES.map((item) => (
                <SelectItem key={item.mode} value={item.mode}>
                  {item.label}
                </SelectItem>
              ))}
            </SelectContent>
          </Select>
          <Select
            value={transformMode}
            disabled={disabled}
            onValueChange={(mode: typeof transformMode) =>
              setTransformMode(mode)
            }
          >
            <SelectTrigger className="w-28" aria-label="导演台变换方式">
              <SelectValue />
            </SelectTrigger>
            <SelectContent>
              <SelectItem value="translate">移动</SelectItem>
              <SelectItem value="rotate">旋转</SelectItem>
              <SelectItem value="scale">缩放</SelectItem>
            </SelectContent>
          </Select>
          <Select
            value={renderMode}
            onValueChange={(mode: typeof renderMode) => setRenderMode(mode)}
          >
            <SelectTrigger className="w-32" aria-label="导演台渲染模式">
              <SelectValue />
            </SelectTrigger>
            <SelectContent>
              <SelectItem
                value="beauty"
                disabled={!capabilities.renderModes.includes("beauty")}
              >
                场景
              </SelectItem>
              <SelectItem
                value="clay"
                disabled={!capabilities.renderModes.includes("clay")}
              >
                灰模
              </SelectItem>
              <SelectItem
                value="depth"
                disabled={!capabilities.renderModes.includes("depth")}
              >
                深度
              </SelectItem>
              <SelectItem
                value="normal"
                disabled={!capabilities.renderModes.includes("normal")}
              >
                法线
              </SelectItem>
              <SelectItem
                value="pose"
                disabled={!capabilities.renderModes.includes("pose")}
              >
                姿态
              </SelectItem>
            </SelectContent>
          </Select>
          <Button
            variant="outline"
            size="icon"
            aria-label="撤销导演台编辑"
            disabled={disabled || !history.past.length}
            onClick={undo}
          >
            <Undo2 />
          </Button>
          <Button
            variant="outline"
            size="icon"
            aria-label="重做导演台编辑"
            disabled={disabled || !history.future.length}
            onClick={redo}
          >
            <Redo2 />
          </Button>
          <div className="ml-auto flex items-center gap-2">
            <Checkbox
              id="director-grid"
              checked={scene.gridVisible}
              disabled={disabled}
              onCheckedChange={(visible) =>
                change({ ...scene, gridVisible: visible === true })
              }
            />
            <label htmlFor="director-grid" className="text-xs">
              网格
            </label>
            <Checkbox
              id="director-snap"
              checked={scene.gridSnap ?? false}
              disabled={disabled}
              onCheckedChange={(snap) =>
                change({ ...scene, gridSnap: snap === true })
              }
            />
            <label htmlFor="director-snap" className="text-xs">
              吸附
            </label>
            <Checkbox
              id="director-labels"
              checked={scene.labelsVisible ?? true}
              disabled={disabled}
              onCheckedChange={(labelsVisible) =>
                change({ ...scene, labelsVisible: labelsVisible === true })
              }
            />
            <label htmlFor="director-labels" className="text-xs">
              标签
            </label>
          </div>
        </div>
        <div className="grid shrink-0 grid-rows-[12rem_20rem_18rem] gap-3 lg:h-[min(54dvh,38rem)] lg:min-h-[280px] lg:grid-cols-[200px_1fr_320px] lg:grid-rows-1">
          <aside className="min-h-0 space-y-4 overflow-y-auto rounded border p-3">
            <div className="space-y-2">
              <p className="text-xs font-medium">演员与对象</p>
              <Button
                variant="outline"
                size="sm"
                disabled={disabled || scene.objects.length >= 128}
                onClick={() => {
                  const object = createDirectorActor(
                    `演员 ${scene.objects.filter((item) => item.kind === "actor").length + 1}`,
                    [scene.objects.length * 0.5, 0, 0],
                  );
                  change({ ...scene, objects: [...scene.objects, object] });
                  setSelectedId(object.id);
                }}
              >
                <User />
                演员
              </Button>
              <Select
                value=""
                disabled={disabled || scene.objects.length >= 128}
                onValueChange={(
                  primitive: NonNullable<DirectorObject["primitive"]>,
                ) => {
                  const object = createDirectorObject(primitive);
                  change({ ...scene, objects: [...scene.objects, object] });
                  setSelectedId(object.id);
                }}
              >
                <SelectTrigger aria-label="增加几何道具">
                  <SelectValue placeholder="增加道具" />
                </SelectTrigger>
                <SelectContent>
                  <SelectItem value="box">立方体</SelectItem>
                  <SelectItem value="sphere">球体</SelectItem>
                  <SelectItem value="cylinder">圆柱</SelectItem>
                  <SelectItem value="plane">平面</SelectItem>
                </SelectContent>
              </Select>
              {scene.objects.map((object) => (
                <div key={object.id} className="flex gap-1">
                  <Button
                    className="min-w-0 flex-1 justify-start truncate"
                    size="sm"
                    variant={selectedId === object.id ? "secondary" : "ghost"}
                    onClick={() => setSelectedId(object.id)}
                  >
                    {object.name}
                  </Button>
                  <Checkbox
                    className="my-auto"
                    aria-label={`${object.name}可见`}
                    disabled={disabled}
                    checked={object.visible}
                    onCheckedChange={(visible) =>
                      change({
                        ...scene,
                        objects: scene.objects.map((item) =>
                          item.id === object.id
                            ? { ...item, visible: visible === true }
                            : item,
                        ),
                      })
                    }
                  />
                </div>
              ))}
            </div>
            <DirectorAssetControls
              scene={viewScene}
              nodes={nodes}
              disabled={disabled}
              onChange={change}
            />
            <label className="flex items-center gap-2 text-sm">
              <Checkbox
                checked={autoKey}
                disabled={disabled || !capabilities.keyframes}
                onCheckedChange={(value) => setAutoKey(value === true)}
              />
              自动记录当前帧
            </label>
            {modelControl?.(scene, change)}
            <div className="space-y-2">
              <p className="text-xs font-medium">摄影机</p>
              <Select
                value={cameraPreset}
                disabled={disabled}
                onValueChange={(value: DirectorCameraPresetId) =>
                  setCameraPreset(value)
                }
              >
                <SelectTrigger aria-label="新增机位预设">
                  <SelectValue />
                </SelectTrigger>
                <SelectContent>
                  {DIRECTOR_CAMERA_PRESETS.map((preset) => (
                    <SelectItem key={preset.id} value={preset.id}>
                      {preset.label}
                    </SelectItem>
                  ))}
                </SelectContent>
              </Select>
              <Button
                variant="outline"
                size="sm"
                disabled={disabled || scene.cameras.length >= 16}
                onClick={() => {
                  const camera = createDirectorCameraFromPreset({
                    presetId: cameraPreset,
                    name: `摄影机 ${scene.cameras.length + 1}`,
                    target: selectedObject
                      ? [
                          selectedObject.transform.position[0],
                          selectedObject.transform.position[1] + 1,
                          selectedObject.transform.position[2],
                        ]
                      : [0, 1, 0],
                    currentView: currentView.current?.(),
                  });
                  change({ ...scene, cameras: [...scene.cameras, camera] });
                  setSelectedId(camera.id);
                }}
              >
                <Camera />
                增加
              </Button>
              {scene.cameras.map((camera) => (
                <Button
                  key={camera.id}
                  className="w-full justify-start"
                  size="sm"
                  variant={selectedId === camera.id ? "secondary" : "ghost"}
                  onClick={() => setSelectedId(camera.id)}
                >
                  {camera.name}
                </Button>
              ))}
            </div>
            <div className="space-y-2">
              <p className="text-xs font-medium">灯光</p>
              <Select
                value=""
                disabled={disabled || scene.lights.length >= 32}
                onValueChange={(
                  type: DirectorScene["lights"][number]["type"],
                ) => {
                  const light = createDirectorLight(
                    type,
                    `灯光 ${scene.lights.length + 1}`,
                    [4, 6, 4],
                  );
                  change({ ...scene, lights: [...scene.lights, light] });
                  setSelectedId(light.id);
                }}
              >
                <SelectTrigger aria-label="增加灯光">
                  <Lightbulb />
                  <SelectValue placeholder="增加灯光" />
                </SelectTrigger>
                <SelectContent>
                  <SelectItem value="directional">平行光</SelectItem>
                  <SelectItem value="point">点光</SelectItem>
                  <SelectItem value="spot">聚光</SelectItem>
                  <SelectItem value="ambient">环境光</SelectItem>
                </SelectContent>
              </Select>
              {scene.lights.map((light) => (
                <Button
                  key={light.id}
                  className="w-full justify-start"
                  size="sm"
                  variant={selectedId === light.id ? "secondary" : "ghost"}
                  onClick={() => setSelectedId(light.id)}
                >
                  {light.name}
                </Button>
              ))}
            </div>
          </aside>
          <main className="min-h-[280px] overflow-hidden rounded border bg-black">
            <DirectorViewport
              onViewReady={registerView}
              onModelMetadata={modelMetadata}
              projectId={projectId}
              scene={scene}
              time={time}
              selectedId={selectedId}
              viewMode={viewMode}
              transformMode={transformMode}
              renderMode={renderMode}
              readOnly={disabled}
              onSelect={setSelectedId}
              onTransform={transform}
              onCaptureReady={registerCapture}
            />
          </main>
          <aside className="min-h-0 space-y-4 overflow-y-auto rounded border p-3">
            <Tabs defaultValue="properties">
              <TabsList aria-label="导演台检查器">
                <TabsTrigger value="properties">属性</TabsTrigger>
                <TabsTrigger value="screenshots">机位截图</TabsTrigger>
              </TabsList>
              <TabsContent value="properties" className="space-y-4">
                {selected ? (
                  <>
                    <Field>
                      <FieldLabel htmlFor="director-element-name">
                        名称
                      </FieldLabel>
                      <Input
                        id="director-element-name"
                        value={selected.name}
                        maxLength={128}
                        disabled={disabled}
                        onChange={(event) => {
                          if (selectedObject)
                            patchObject({ name: event.target.value });
                          if (selectedCamera)
                            patchCamera({ name: event.target.value });
                          if (selectedLight)
                            change({
                              ...scene,
                              lights: scene.lights.map((light) =>
                                light.id === selectedId
                                  ? { ...light, name: event.target.value }
                                  : light,
                              ),
                            });
                        }}
                      />
                    </Field>
                    <TransformFields
                      value={selected.transform}
                      disabled={disabled}
                      onChange={(next) => transform(selectedId, next)}
                    />
                    {selectedObject ? (
                      <>
                        <Field>
                          <FieldLabel htmlFor="director-object-color">
                            颜色
                          </FieldLabel>
                          <Input
                            id="director-object-color"
                            type="color"
                            value={selectedObject.color}
                            disabled={disabled}
                            onChange={(event) =>
                              patchObject({ color: event.target.value })
                            }
                          />
                        </Field>
                        <Field>
                          <FieldLabel htmlFor="director-uniform-scale">
                            统一倍率
                          </FieldLabel>
                          <Input
                            id="director-uniform-scale"
                            type="number"
                            min={0.1}
                            max={10}
                            step={0.1}
                            value={selectedObject.uniformScale ?? 1}
                            disabled={disabled}
                            onChange={(event) => {
                              const next = applyDirectorUniformScale(
                                selectedObject,
                                Number(event.target.value),
                              );
                              patchObject(next);
                            }}
                          />
                        </Field>
                        {capabilities.bones &&
                        (selectedObject.kind === "actor" ||
                          selectedObject.primitive === "character") ? (
                          <>
                            <Field>
                              <FieldLabel>角色姿态</FieldLabel>
                              <Select
                                value={selectedObject.pose ?? "stand"}
                                disabled={disabled}
                                onValueChange={(pose: DirectorPose) =>
                                  patchObject({ pose })
                                }
                              >
                                <SelectTrigger aria-label="角色姿态">
                                  <SelectValue />
                                </SelectTrigger>
                                <SelectContent>
                                  {poses.map((pose) => (
                                    <SelectItem key={pose} value={pose}>
                                      {directorPoseLabel(pose)}
                                    </SelectItem>
                                  ))}
                                </SelectContent>
                              </Select>
                            </Field>
                            {selectedObject.assetId ? (
                              <>
                                <p className="text-xs text-muted-foreground">
                                  {
                                    Object.keys(
                                      selectedObject.rig?.boneMap ?? {},
                                    ).length
                                  }{" "}
                                  个已映射骨骼 ·{" "}
                                  {selectedObject.rig?.animationNames.length ??
                                    0}{" "}
                                  个内置动画
                                </p>
                                <Field>
                                  <FieldLabel>模型动作片段</FieldLabel>
                                  <Select
                                    value={
                                      selectedObject.activeMotionClipId ??
                                      "none"
                                    }
                                    disabled={
                                      disabled ||
                                      !selectedObject.motionClips?.length
                                    }
                                    onValueChange={(value) =>
                                      patchObject({
                                        activeMotionClipId:
                                          value === "none" ? undefined : value,
                                      })
                                    }
                                  >
                                    <SelectTrigger aria-label="模型动作片段">
                                      <SelectValue />
                                    </SelectTrigger>
                                    <SelectContent>
                                      <SelectItem value="none">
                                        无动作 · 使用姿势
                                      </SelectItem>
                                      {selectedObject.motionClips?.map(
                                        (clip) => (
                                          <SelectItem
                                            key={clip.id}
                                            value={clip.id}
                                          >
                                            {clip.name}
                                          </SelectItem>
                                        ),
                                      )}
                                    </SelectContent>
                                  </Select>
                                </Field>
                                {selectedObject.activeMotionClipId
                                  ? selectedObject.motionClips
                                      ?.filter(
                                        (clip) =>
                                          clip.id ===
                                          selectedObject.activeMotionClipId,
                                      )
                                      .map((clip) => (
                                        <div
                                          key={clip.id}
                                          className="space-y-2"
                                        >
                                          <Field>
                                            <FieldLabel htmlFor="director-motion-start">
                                              动作开始秒
                                            </FieldLabel>
                                            <Input
                                              id="director-motion-start"
                                              type="number"
                                              min={0}
                                              max={3600}
                                              step={0.1}
                                              disabled={disabled}
                                              value={clip.start}
                                              onChange={(event) =>
                                                patchObject({
                                                  motionClips:
                                                    selectedObject.motionClips?.map(
                                                      (item) =>
                                                        item.id === clip.id
                                                          ? {
                                                              ...item,
                                                              start: Number(
                                                                event.target
                                                                  .value,
                                                              ),
                                                            }
                                                          : item,
                                                    ),
                                                })
                                              }
                                            />
                                          </Field>
                                          <Field>
                                            <FieldLabel htmlFor="director-motion-rate">
                                              动作播放倍率
                                            </FieldLabel>
                                            <Input
                                              id="director-motion-rate"
                                              type="number"
                                              min={0.1}
                                              max={4}
                                              step={0.1}
                                              disabled={disabled}
                                              value={clip.playbackRate}
                                              onChange={(event) =>
                                                patchObject({
                                                  motionClips:
                                                    selectedObject.motionClips?.map(
                                                      (item) =>
                                                        item.id === clip.id
                                                          ? {
                                                              ...item,
                                                              playbackRate:
                                                                Number(
                                                                  event.target
                                                                    .value,
                                                                ),
                                                            }
                                                          : item,
                                                    ),
                                                })
                                              }
                                            />
                                          </Field>
                                          <label className="flex gap-2 text-sm">
                                            <Checkbox
                                              checked={clip.loop}
                                              disabled={disabled}
                                              onCheckedChange={(loop) =>
                                                patchObject({
                                                  motionClips:
                                                    selectedObject.motionClips?.map(
                                                      (item) =>
                                                        item.id === clip.id
                                                          ? {
                                                              ...item,
                                                              loop:
                                                                loop === true,
                                                            }
                                                          : item,
                                                    ),
                                                })
                                              }
                                            />
                                            循环播放动作
                                          </label>
                                        </div>
                                      ))
                                  : null}
                              </>
                            ) : null}
                            <Field>
                              <FieldLabel>骨骼</FieldLabel>
                              <Select
                                value={bone}
                                disabled={disabled}
                                onValueChange={(value: DirectorHumanoidBone) =>
                                  setBone(value)
                                }
                              >
                                <SelectTrigger aria-label="角色骨骼">
                                  <SelectValue />
                                </SelectTrigger>
                                <SelectContent>
                                  {directorBoneNames.map((name) => (
                                    <SelectItem key={name} value={name}>
                                      {directorBoneLabel(name)}
                                    </SelectItem>
                                  ))}
                                </SelectContent>
                              </Select>
                            </Field>
                            <div className="grid grid-cols-3 gap-2">
                              {[boneEuler.x, boneEuler.y, boneEuler.z].map(
                                (angle, index) => (
                                  <Field key={index}>
                                    <FieldLabel
                                      htmlFor={`director-bone-${index}`}
                                    >
                                      {["X", "Y", "Z"][index]} (°)
                                    </FieldLabel>
                                    <Input
                                      id={`director-bone-${index}`}
                                      type="number"
                                      step={1}
                                      value={Number(
                                        ((angle * 180) / Math.PI).toFixed(2),
                                      )}
                                      disabled={disabled}
                                      onChange={(event) => {
                                        const values = [
                                          boneEuler.x,
                                          boneEuler.y,
                                          boneEuler.z,
                                        ];
                                        values[index] =
                                          (Number(event.target.value) *
                                            Math.PI) /
                                          180;
                                        const rotation = new Quaternion()
                                          .setFromEuler(
                                            new Euler(
                                              ...(values as DirectorVec3),
                                            ),
                                          )
                                          .toArray();
                                        patchObject({
                                          boneOverrides: {
                                            ...selectedObject.boneOverrides,
                                            [bone]: rotation,
                                          },
                                        });
                                      }}
                                    />
                                  </Field>
                                ),
                              )}
                            </div>
                            <Button
                              variant="outline"
                              size="sm"
                              disabled={disabled}
                              onClick={() =>
                                patchObject({
                                  boneTracks: upsertDirectorBoneKeyframe(
                                    selectedObject.boneTracks ?? [],
                                    bone,
                                    time,
                                    boneRotation,
                                  ),
                                })
                              }
                            >
                              记录骨骼关键帧
                            </Button>
                            {selectedObject.boneTracks
                              ?.find((track) => track.bone === bone)
                              ?.keyframes.map((frame) => (
                                <div
                                  key={frame.id}
                                  className="flex items-center gap-2 text-xs"
                                >
                                  <button
                                    onClick={() => {
                                      setPlaying(false);
                                      setTime(frame.time);
                                    }}
                                  >
                                    {frame.time.toFixed(3)} s
                                  </button>
                                  <Select
                                    value={frame.easing ?? "linear"}
                                    disabled={disabled}
                                    onValueChange={(
                                      easing: "step" | "linear" | "smooth",
                                    ) =>
                                      patchObject({
                                        boneTracks:
                                          selectedObject.boneTracks!.map(
                                            (track) =>
                                              track.bone === bone
                                                ? {
                                                    ...track,
                                                    keyframes:
                                                      track.keyframes.map(
                                                        (item) =>
                                                          item.id === frame.id
                                                            ? {
                                                                ...item,
                                                                easing,
                                                              }
                                                            : item,
                                                      ),
                                                  }
                                                : track,
                                          ),
                                      })
                                    }
                                  >
                                    <SelectTrigger
                                      aria-label="骨骼关键帧缓动"
                                      className="h-7"
                                    >
                                      <SelectValue />
                                    </SelectTrigger>
                                    <SelectContent>
                                      <SelectItem value="linear">
                                        线性
                                      </SelectItem>
                                      <SelectItem value="smooth">
                                        平滑
                                      </SelectItem>
                                      <SelectItem value="step">保持</SelectItem>
                                    </SelectContent>
                                  </Select>
                                  <Button
                                    variant="ghost"
                                    size="icon"
                                    disabled={disabled}
                                    aria-label="删除骨骼关键帧"
                                    onClick={() =>
                                      patchObject({
                                        boneTracks: removeDirectorBoneKeyframe(
                                          selectedObject.boneTracks ?? [],
                                          bone,
                                          frame.id,
                                        ),
                                      })
                                    }
                                  >
                                    <Trash2 />
                                  </Button>
                                </div>
                              ))}
                          </>
                        ) : null}
                      </>
                    ) : null}
                    {selectedCamera ? (
                      <>
                        <div className="grid grid-cols-2 gap-2">
                          {(
                            [
                              ["focalLength", "焦距 (mm)"],
                              ["aperture", "光圈"],
                              ["focusDistance", "焦点 (m)"],
                              ["near", "近裁剪 (m)"],
                              ["far", "远裁剪 (m)"],
                            ] as const
                          ).map(([key, label]) => (
                            <Field key={key}>
                              <FieldLabel htmlFor={`director-camera-${key}`}>
                                {label}
                              </FieldLabel>
                              <Input
                                id={`director-camera-${key}`}
                                type="number"
                                step={0.1}
                                value={selectedCamera[key]}
                                disabled={disabled}
                                onChange={(event) => {
                                  const next = Number(event.target.value);
                                  patchCamera({
                                    [key]: next,
                                    ...(key === "focalLength"
                                      ? { fov: directorFocalLengthToFov(next) }
                                      : {}),
                                  });
                                }}
                              />
                            </Field>
                          ))}
                        </div>
                        <VectorFields
                          label="注视坐标"
                          value={selectedCamera.target}
                          disabled={disabled}
                          onChange={(target) => patchCamera({ target })}
                        />
                        <Field>
                          <FieldLabel>注视方式</FieldLabel>
                          <Select
                            value={selectedCamera.lookAtMode ?? "coordinates"}
                            disabled={disabled}
                            onValueChange={(
                              lookAtMode: NonNullable<
                                DirectorCamera["lookAtMode"]
                              >,
                            ) => patchCamera({ lookAtMode })}
                          >
                            <SelectTrigger aria-label="摄影机注视方式">
                              <SelectValue />
                            </SelectTrigger>
                            <SelectContent>
                              <SelectItem value="coordinates">坐标</SelectItem>
                              <SelectItem value="rotation">旋转方向</SelectItem>
                              <SelectItem value="object">对象</SelectItem>
                            </SelectContent>
                          </Select>
                        </Field>
                        {selectedCamera.lookAtMode === "object" ? (
                          <Select
                            value={selectedCamera.lookAtObjectId ?? "none"}
                            disabled={disabled}
                            onValueChange={(lookAtObjectId) =>
                              patchCamera({
                                lookAtObjectId:
                                  lookAtObjectId === "none"
                                    ? undefined
                                    : lookAtObjectId,
                              })
                            }
                          >
                            <SelectTrigger aria-label="摄影机注视对象">
                              <SelectValue />
                            </SelectTrigger>
                            <SelectContent>
                              <SelectItem value="none">未选择</SelectItem>
                              {scene.objects.map((object) => (
                                <SelectItem key={object.id} value={object.id}>
                                  {object.name}
                                </SelectItem>
                              ))}
                            </SelectContent>
                          </Select>
                        ) : null}
                        <Field>
                          <FieldLabel>跟随对象</FieldLabel>
                          <Select
                            value={selectedCamera.followObjectId ?? "none"}
                            disabled={disabled}
                            onValueChange={(target) =>
                              patchCamera(
                                target === "none"
                                  ? unbindDirectorCameraFollow(
                                      selectedCamera,
                                      scene,
                                      time,
                                    )
                                  : bindDirectorCameraFollow(
                                      selectedCamera,
                                      scene,
                                      target,
                                      time,
                                    ),
                              )
                            }
                          >
                            <SelectTrigger aria-label="摄影机跟随对象">
                              <SelectValue />
                            </SelectTrigger>
                            <SelectContent>
                              <SelectItem value="none">自由机位</SelectItem>
                              {scene.objects.map((object) => (
                                <SelectItem key={object.id} value={object.id}>
                                  {object.name}
                                </SelectItem>
                              ))}
                            </SelectContent>
                          </Select>
                        </Field>
                      </>
                    ) : null}
                    {selectedLight ? (
                      <>
                        <Field>
                          <FieldLabel htmlFor="director-light-color">
                            灯光颜色
                          </FieldLabel>
                          <Input
                            id="director-light-color"
                            type="color"
                            value={selectedLight.color}
                            disabled={disabled}
                            onChange={(event) =>
                              change({
                                ...scene,
                                lights: scene.lights.map((light) =>
                                  light.id === selectedId
                                    ? { ...light, color: event.target.value }
                                    : light,
                                ),
                              })
                            }
                          />
                        </Field>
                        {(
                          [
                            ["intensity", "强度"],
                            ["angle", "聚光角 (rad)"],
                            ["penumbra", "柔边 (0～1)"],
                          ] as const
                        ).map(([key, label]) => (
                          <Field key={key}>
                            <FieldLabel htmlFor={`director-light-${key}`}>
                              {label}
                            </FieldLabel>
                            <Input
                              id={`director-light-${key}`}
                              type="number"
                              min={0}
                              step={0.05}
                              value={selectedLight[key] ?? 0}
                              disabled={disabled}
                              onChange={(event) =>
                                change({
                                  ...scene,
                                  lights: scene.lights.map((light) =>
                                    light.id === selectedId
                                      ? {
                                          ...light,
                                          [key]: Number(event.target.value),
                                        }
                                      : light,
                                  ),
                                })
                              }
                            />
                          </Field>
                        ))}
                      </>
                    ) : null}
                    {selectedObject || selectedCamera ? (
                      <Button
                        variant="outline"
                        className="w-full"
                        disabled={disabled}
                        onClick={recordKeyframe}
                      >
                        记录位置关键帧
                      </Button>
                    ) : null}
                    <Button
                      variant="outline"
                      className="w-full"
                      disabled={
                        disabled ||
                        Boolean(selectedCamera && scene.cameras.length <= 1)
                      }
                      onClick={deleteSelected}
                    >
                      <Trash2 />
                      删除选中元素
                    </Button>
                  </>
                ) : (
                  <p className="text-sm text-muted-foreground">
                    选择对象、摄影机或灯光编辑。
                  </p>
                )}
                <div className="space-y-2 border-t pt-3">
                  <p className="text-xs font-medium">场景</p>
                  <Field>
                    <FieldLabel htmlFor="director-scene-title">
                      场景名称
                    </FieldLabel>
                    <Input
                      id="director-scene-title"
                      value={scene.title}
                      maxLength={128}
                      disabled={disabled}
                      onChange={(event) =>
                        change({ ...scene, title: event.target.value })
                      }
                    />
                  </Field>
                  <Field>
                    <FieldLabel htmlFor="director-background">
                      背景颜色
                    </FieldLabel>
                    <Input
                      id="director-background"
                      type="color"
                      value={scene.background}
                      disabled={disabled}
                      onChange={(event) =>
                        change({ ...scene, background: event.target.value })
                      }
                    />
                  </Field>
                  <Field>
                    <FieldLabel>画幅</FieldLabel>
                    <Select
                      value={scene.aspectRatio ?? "adaptive"}
                      disabled={disabled}
                      onValueChange={(
                        aspectRatio: NonNullable<DirectorScene["aspectRatio"]>,
                      ) => change({ ...scene, aspectRatio })}
                    >
                      <SelectTrigger aria-label="导演台画幅">
                        <SelectValue />
                      </SelectTrigger>
                      <SelectContent>
                        {DIRECTOR_ASPECT_RATIOS.map((ratio) => (
                          <SelectItem key={ratio} value={ratio}>
                            {ratio === "adaptive" ? "自适应" : ratio}
                          </SelectItem>
                        ))}
                      </SelectContent>
                    </Select>
                  </Field>
                  <Field>
                    <FieldLabel htmlFor="director-ground-opacity">
                      地面透明度
                    </FieldLabel>
                    <Input
                      id="director-ground-opacity"
                      type="number"
                      min={0}
                      max={1}
                      step={0.1}
                      value={scene.ground?.opacity ?? 0.4}
                      disabled={disabled}
                      onChange={(event) =>
                        change({
                          ...scene,
                          ground: {
                            visible: scene.ground?.visible ?? true,
                            height: scene.ground?.height ?? 0,
                            opacity: Number(event.target.value),
                          },
                        })
                      }
                    />
                  </Field>
                  <Field>
                    <FieldLabel htmlFor="director-environment">
                      环境光强度
                    </FieldLabel>
                    <Input
                      id="director-environment"
                      type="number"
                      min={0}
                      max={10}
                      step={0.1}
                      disabled={disabled}
                      value={scene.environmentIntensity}
                      onChange={(event) =>
                        change({
                          ...scene,
                          environmentIntensity: Number(event.target.value),
                        })
                      }
                    />
                  </Field>
                  <label className="flex items-center gap-2 text-sm">
                    <Checkbox
                      checked={scene.ground?.visible ?? true}
                      disabled={disabled}
                      onCheckedChange={(visible) =>
                        change({
                          ...scene,
                          ground: {
                            ...scene.ground!,
                            visible: visible === true,
                            opacity: scene.ground?.opacity ?? 0.4,
                            height: scene.ground?.height ?? 0,
                          },
                        })
                      }
                    />
                    显示地面
                  </label>
                  <Field>
                    <FieldLabel htmlFor="director-ground-height">
                      地面高度
                    </FieldLabel>
                    <Input
                      id="director-ground-height"
                      type="number"
                      min={-2}
                      max={2}
                      step={0.1}
                      disabled={disabled}
                      value={scene.ground?.height ?? 0}
                      onChange={(event) =>
                        change({
                          ...scene,
                          ground: {
                            visible: scene.ground?.visible ?? true,
                            opacity: scene.ground?.opacity ?? 0.4,
                            height: Number(event.target.value),
                          },
                        })
                      }
                    />
                  </Field>
                  <Field>
                    <FieldLabel htmlFor="director-stage-scale">
                      舞台整体倍率
                    </FieldLabel>
                    <Input
                      id="director-stage-scale"
                      type="number"
                      min={0.1}
                      max={10}
                      step={0.1}
                      disabled={disabled}
                      value={scene.stageTransform?.scale ?? 1}
                      onChange={(event) =>
                        change({
                          ...scene,
                          stageTransform: {
                            position: scene.stageTransform?.position ?? [
                              0, 0, 0,
                            ],
                            rotation: scene.stageTransform?.rotation ?? [
                              0, 0, 0,
                            ],
                            scale: Number(event.target.value),
                          },
                        })
                      }
                    />
                  </Field>
                  <VectorFields
                    label="舞台旋转 (°)"
                    value={scene.stageTransform?.rotation ?? [0, 0, 0]}
                    disabled={disabled}
                    onChange={(rotation) =>
                      change({
                        ...scene,
                        stageTransform: {
                          position: scene.stageTransform?.position ?? [0, 0, 0],
                          scale: scene.stageTransform?.scale ?? 1,
                          rotation,
                        },
                      })
                    }
                  />
                  {scene.panorama ? (
                    <>
                      <Field>
                        <FieldLabel htmlFor="director-panorama-rotation">
                          全景旋转角度
                        </FieldLabel>
                        <Input
                          id="director-panorama-rotation"
                          type="number"
                          min={-360}
                          max={360}
                          disabled={disabled}
                          value={scene.panoramaRotation ?? 0}
                          onChange={(event) =>
                            change({
                              ...scene,
                              panoramaRotation: Number(event.target.value),
                            })
                          }
                        />
                      </Field>
                      <Field>
                        <FieldLabel htmlFor="director-panorama-radius">
                          全景半径
                        </FieldLabel>
                        <Input
                          id="director-panorama-radius"
                          type="number"
                          min={1}
                          max={200}
                          disabled={disabled}
                          value={scene.panoramaRadius ?? 60}
                          onChange={(event) =>
                            change({
                              ...scene,
                              panoramaRadius: Number(event.target.value),
                            })
                          }
                        />
                      </Field>
                    </>
                  ) : null}
                  <VectorFields
                    label="舞台位置"
                    value={scene.stageTransform?.position ?? [0, 0, 0]}
                    disabled={disabled}
                    onChange={(position) =>
                      change({
                        ...scene,
                        stageTransform: {
                          scale: scene.stageTransform?.scale ?? 1,
                          rotation: scene.stageTransform?.rotation ?? [0, 0, 0],
                          position,
                        },
                      })
                    }
                  />
                </div>
              </TabsContent>
              <TabsContent value="screenshots">
                <DirectorGallery
                  projectId={projectId}
                  scene={scene}
                  disabled={disabled}
                  onChange={change}
                />
              </TabsContent>
            </Tabs>
          </aside>
        </div>
        <div className="grid shrink-0 gap-3 lg:grid-cols-[200px_1fr_320px]">
          <div className="space-y-2">
            <Field>
              <FieldLabel>当前分镜</FieldLabel>
              <Select
                value={scene.activeShotId}
                disabled={disabled}
                onValueChange={(activeShotId) => {
                  setPlaying(false);
                  setTime(0);
                  change({ ...scene, activeShotId });
                }}
              >
                <SelectTrigger aria-label="当前分镜">
                  <SelectValue />
                </SelectTrigger>
                <SelectContent>
                  {scene.shots.map((shot) => (
                    <SelectItem key={shot.id} value={shot.id}>
                      {shot.name}
                    </SelectItem>
                  ))}
                </SelectContent>
              </Select>
            </Field>
            <Button
              variant="outline"
              size="sm"
              disabled={disabled || scene.shots.length >= 128}
              onClick={() => {
                const shot = {
                  ...activeShot,
                  id: crypto.randomUUID(),
                  name: `镜头 ${scene.shots.length + 1}`,
                  screenshots: undefined,
                };
                change({
                  ...scene,
                  shots: [...scene.shots, shot],
                  activeShotId: shot.id,
                });
              }}
            >
              <Plus />
              分镜
            </Button>
            <Button
              variant="ghost"
              size="icon"
              aria-label="删除当前分镜"
              disabled={disabled || scene.shots.length <= 1}
              onClick={() => {
                const shots = scene.shots.filter(
                  (shot) => shot.id !== activeShot.id,
                );
                change({ ...scene, shots, activeShotId: shots[0].id });
              }}
            >
              <Trash2 />
            </Button>
          </div>
          <div className="space-y-2">
            <div className="flex items-center gap-3">
              <Button
                variant="outline"
                size="icon"
                aria-label={playing ? "暂停运镜" : "播放运镜"}
                onClick={() => {
                  if (time >= activeShot.duration) setTime(0);
                  setPlaying(!playing);
                }}
              >
                {playing ? <Pause /> : <Play />}
              </Button>
              <input
                type="range"
                aria-label="导演台播放头"
                className="flex-1"
                min={0}
                max={activeShot.duration}
                step={1 / activeShot.fps}
                value={time}
                onChange={(event) => {
                  setPlaying(false);
                  setTime(Number(event.target.value));
                }}
              />
              <span className="w-24 text-right text-xs tabular-nums">
                {time.toFixed(2)} / {activeShot.duration}s
              </span>
            </div>
            <div className="flex gap-1 overflow-x-auto">
              {frames.map((frame) => (
                <div
                  key={frame.id}
                  className="flex items-center gap-1 rounded border p-1 text-xs"
                >
                  <button
                    className="px-1"
                    onClick={() => {
                      setPlaying(false);
                      setTime(frame.time);
                    }}
                  >
                    {frame.time.toFixed(2)} s
                  </button>
                  <Select
                    value={frame.easing ?? "linear"}
                    disabled={disabled}
                    onValueChange={(easing: "step" | "linear" | "smooth") => {
                      const next = frames.map((item) =>
                        item.id === frame.id ? { ...item, easing } : item,
                      );
                      if (selectedObject) patchObject({ keyframes: next });
                      if (selectedCamera) patchCamera({ keyframes: next });
                    }}
                  >
                    <SelectTrigger
                      aria-label="位置关键帧缓动"
                      className="h-7 w-20"
                    >
                      <SelectValue />
                    </SelectTrigger>
                    <SelectContent>
                      <SelectItem value="linear">线性</SelectItem>
                      <SelectItem value="smooth">平滑</SelectItem>
                      <SelectItem value="step">保持</SelectItem>
                    </SelectContent>
                  </Select>
                  <Button
                    size="icon"
                    variant="ghost"
                    disabled={disabled}
                    aria-label="删除位置关键帧"
                    onClick={() => {
                      if (selectedObject)
                        patchObject({
                          keyframes: removeDirectorKeyframe(frames, frame.id),
                        });
                      if (selectedCamera)
                        patchCamera({
                          keyframes: removeDirectorKeyframe(frames, frame.id),
                        });
                    }}
                  >
                    <Trash2 />
                  </Button>
                </div>
              ))}
            </div>
            <Field>
              <FieldLabel htmlFor="director-shot-name">镜头名称</FieldLabel>
              <Input
                id="director-shot-name"
                value={activeShot.name}
                maxLength={128}
                disabled={disabled}
                onChange={(event) =>
                  change({
                    ...scene,
                    shots: scene.shots.map((shot) =>
                      shot.id === activeShot.id
                        ? { ...shot, name: event.target.value }
                        : shot,
                    ),
                  })
                }
              />
            </Field>
            {capabilities.cameraTools ? (
              <div className="flex flex-wrap gap-2">
                <Button
                  variant="outline"
                  size="sm"
                  disabled={disabled}
                  onClick={() => {
                    const camera = scene.cameras.find(
                      (camera) => camera.id === activeShot.cameraId,
                    )!;
                    change({
                      ...scene,
                      cameras: scene.cameras.map((item) =>
                        item.id === camera.id
                          ? {
                              ...item,
                              keyframes: resolveDirectorCameraMoveKeyframes(
                                item.keyframes,
                                item.transform,
                                cameraMoveTransform(
                                  item.transform,
                                  activeShot.cameraMove,
                                ),
                                activeShot.duration,
                              ),
                            }
                          : item,
                      ),
                    });
                  }}
                >
                  生成运镜首尾帧
                </Button>
                <Button
                  variant="outline"
                  size="sm"
                  disabled={disabled}
                  onClick={() => {
                    const view = currentView.current?.();
                    if (!view) return;
                    change({
                      ...scene,
                      cameras: scene.cameras.map((camera) =>
                        camera.id === activeShot.cameraId
                          ? resolveDirectorCameraAlignment(
                              camera,
                              view,
                              snapDirectorTime(time, activeShot.fps),
                            )
                          : camera,
                      ),
                    });
                  }}
                >
                  将当前视角对齐摄影机
                </Button>
              </div>
            ) : null}
            <div className="grid grid-cols-2 gap-2">
              <Select
                value={activeShot.shotSize}
                disabled={disabled}
                onValueChange={(shotSize: typeof activeShot.shotSize) =>
                  change({
                    ...scene,
                    shots: scene.shots.map((shot) =>
                      shot.id === activeShot.id ? { ...shot, shotSize } : shot,
                    ),
                  })
                }
              >
                <SelectTrigger aria-label="镜头景别">
                  <SelectValue />
                </SelectTrigger>
                <SelectContent>
                  {(
                    [
                      ["extreme_wide", "大远景"],
                      ["wide", "远景"],
                      ["full", "全景"],
                      ["medium", "中景"],
                      ["close_up", "近景"],
                      ["extreme_close_up", "特写"],
                    ] as const
                  ).map(([value, label]) => (
                    <SelectItem key={value} value={value}>
                      {label}
                    </SelectItem>
                  ))}
                </SelectContent>
              </Select>
              <Select
                value={activeShot.cameraMove}
                disabled={disabled}
                onValueChange={(cameraMove: typeof activeShot.cameraMove) =>
                  change({
                    ...scene,
                    shots: scene.shots.map((shot) =>
                      shot.id === activeShot.id
                        ? { ...shot, cameraMove }
                        : shot,
                    ),
                  })
                }
              >
                <SelectTrigger aria-label="摄影机运动">
                  <SelectValue />
                </SelectTrigger>
                <SelectContent>
                  {(
                    [
                      ["static", "固定"],
                      ["push_in", "推近"],
                      ["pull_out", "拉远"],
                      ["pan_left", "左摇"],
                      ["pan_right", "右摇"],
                      ["tilt_up", "上摇"],
                      ["tilt_down", "下摇"],
                      ["orbit_left", "左环绕"],
                      ["orbit_right", "右环绕"],
                      ["handheld", "手持"],
                    ] as const
                  ).map(([value, label]) => (
                    <SelectItem key={value} value={value}>
                      {label}
                    </SelectItem>
                  ))}
                </SelectContent>
              </Select>
            </div>
            <div className="flex gap-2">
              <Select
                value={activeShot.cameraId}
                disabled={disabled}
                onValueChange={(cameraId) =>
                  change({
                    ...scene,
                    shots: scene.shots.map((shot) =>
                      shot.id === activeShot.id ? { ...shot, cameraId } : shot,
                    ),
                  })
                }
              >
                <SelectTrigger aria-label="分镜摄影机" className="w-40">
                  <SelectValue />
                </SelectTrigger>
                <SelectContent>
                  {scene.cameras.map((camera) => (
                    <SelectItem key={camera.id} value={camera.id}>
                      {camera.name}
                    </SelectItem>
                  ))}
                </SelectContent>
              </Select>
              <Input
                type="number"
                aria-label="分镜时长秒"
                className="w-24"
                min={0.1}
                max={3600}
                step={0.1}
                value={activeShot.duration}
                disabled={disabled}
                onChange={(event) =>
                  change({
                    ...scene,
                    shots: scene.shots.map((shot) =>
                      shot.id === activeShot.id
                        ? { ...shot, duration: Number(event.target.value) }
                        : shot,
                    ),
                  })
                }
              />
              <Select
                value={String(activeShot.fps)}
                disabled={disabled}
                onValueChange={(fps) =>
                  change({
                    ...scene,
                    shots: scene.shots.map((shot) =>
                      shot.id === activeShot.id
                        ? { ...shot, fps: Number(fps) as 24 | 25 | 30 }
                        : shot,
                    ),
                  })
                }
              >
                <SelectTrigger aria-label="分镜帧率" className="w-28">
                  <SelectValue />
                </SelectTrigger>
                <SelectContent>
                  {[24, 25, 30].map((fps) => (
                    <SelectItem key={fps} value={String(fps)}>
                      {fps} fps
                    </SelectItem>
                  ))}
                </SelectContent>
              </Select>
            </div>
          </div>
          <div className="flex flex-wrap items-center gap-2">
            <Button
              variant="outline"
              size="sm"
              disabled={disabled || !captureReady}
              onClick={() => {
                void takeCapture("beauty");
              }}
            >
              <Download />
              场景截图
            </Button>
            <Button
              variant="outline"
              size="sm"
              disabled={disabled || !captureReady}
              onClick={() => {
                void takeCapture("depth");
              }}
            >
              深度图
            </Button>
            <Button
              variant="outline"
              size="sm"
              disabled={disabled || !captureReady}
              onClick={() => {
                void takeCapture("normal");
              }}
            >
              法线图
            </Button>
          </div>
        </div>
        <details className="shrink-0 rounded border p-3">
          <summary className="cursor-pointer text-sm">分镜提示词</summary>
          <div className="mt-3 grid gap-3 lg:grid-cols-2">
            <Textarea
              aria-label="分镜创作描述"
              maxLength={10000}
              value={activeShot.prompt}
              disabled={disabled}
              onChange={(event) =>
                change({
                  ...scene,
                  shots: scene.shots.map((shot) =>
                    shot.id === activeShot.id
                      ? { ...shot, prompt: event.target.value }
                      : shot,
                  ),
                })
              }
            />
            <Textarea aria-label="编译后的分镜提示词" value={prompt} readOnly />
            <Button
              variant="outline"
              disabled={disabled}
              onClick={() => {
                if (!navigator.clipboard) {
                  setError("当前浏览器剪贴板不可用。");
                  return;
                }
                void navigator.clipboard
                  .writeText(prompt)
                  .catch(() =>
                    setError("剪贴板写入失败，请允许浏览器权限后重试。"),
                  );
              }}
            >
              <Copy />
              复制提示词
            </Button>
            <Button
              variant="outline"
              disabled={disabled || Array.from(prompt).length > 10000}
              onClick={() => {
                void save(false).then((saved) => {
                  if (saved) onPrompt(prompt);
                });
              }}
            >
              添加提示词到画布
            </Button>
            <Button
              disabled={disabled || Array.from(prompt).length > 10000}
              onClick={() => {
                void save(false).then(async (saved) => {
                  if (!saved) return;
                  setSaving(true);
                  try {
                    if (await onGenerate(prompt)) onClose();
                    else setError("视频草稿尚未确认保存。");
                  } catch {
                    setError("视频草稿保存失败。");
                  } finally {
                    setSaving(false);
                  }
                });
              }}
            >
              创建视频生成草稿
            </Button>
            {Array.from(prompt).length > 10000 ? (
              <p role="alert" className="text-sm text-destructive">
                编译提示词超过 10,000 字符，请删减场景或描述后创建草稿。
              </p>
            ) : null}
          </div>
        </details>
        {error ? (
          <p role="alert" className="shrink-0 text-sm text-destructive">
            {error}
          </p>
        ) : null}
        <DialogFooter className="shrink-0">
          <Button variant="outline" disabled={saving} onClick={onClose}>
            取消
          </Button>
          <Button
            disabled={disabled}
            onClick={() => {
              if (scene.cover) void save();
              else if (captureReady) void takeCapture("beauty");
              else setError("场景视口尚未就绪，请等待后保存并关闭。");
            }}
          >
            <Save />
            {saving ? "保存中…" : "保存并关闭"}
          </Button>
        </DialogFooter>
      </DialogContent>
    </Dialog>
  );
}
function VectorFields({
  label,
  value,
  disabled,
  onChange,
}: {
  label: string;
  value: DirectorVec3;
  disabled: boolean;
  onChange: (value: DirectorVec3) => void;
}) {
  return (
    <div>
      <p className="mb-2 text-xs font-medium">{label}</p>
      <div className="grid grid-cols-3 gap-2">
        {value.map((coordinate, axis) => (
          <Input
            key={axis}
            aria-label={`${label} ${["X", "Y", "Z"][axis]}`}
            type="number"
            step={0.1}
            value={Number(coordinate.toFixed(4))}
            disabled={disabled}
            onChange={(event) => {
              const next = [...value] as DirectorVec3;
              next[axis] = Number(event.target.value);
              if (next.every(Number.isFinite)) onChange(next);
            }}
          />
        ))}
      </div>
    </div>
  );
}
function TransformFields({
  value,
  disabled,
  onChange,
}: {
  value: DirectorTransform;
  disabled: boolean;
  onChange: (value: DirectorTransform) => void;
}) {
  return (
    <div className="space-y-3">
      <VectorFields
        label="位置 (m)"
        value={value.position}
        disabled={disabled}
        onChange={(position) => onChange({ ...value, position })}
      />
      <VectorFields
        label="旋转 (°)"
        value={
          value.rotation.map((angle) => (angle * 180) / Math.PI) as DirectorVec3
        }
        disabled={disabled}
        onChange={(angles) =>
          onChange({
            ...value,
            rotation: angles.map(
              (angle) => (angle * Math.PI) / 180,
            ) as DirectorVec3,
          })
        }
      />
      <VectorFields
        label="缩放"
        value={value.scale}
        disabled={disabled}
        onChange={(scale) => onChange({ ...value, scale })}
      />
    </div>
  );
}
