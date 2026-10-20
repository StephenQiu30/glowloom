"use client";

import {
  useEffect,
  useEffectEvent,
  useImperativeHandle,
  useRef,
  useState,
} from "react";
import type * as THREE from "three";
import Image from "next/image";
import {
  ArrowLeft,
  ArrowRight,
  Camera,
  CameraIcon,
  Circle,
  Cuboid,
  Download,
  Lightbulb,
  Move3D,
  Pause,
  Play,
  Plus,
  Rotate3D,
  Scaling,
  Trash2,
  UserRound,
  X,
} from "lucide-react";
import { Button } from "@/components/ui/button";
import { Checkbox } from "@/components/ui/checkbox";
import {
  Dialog,
  DialogContent,
  DialogDescription,
  DialogTitle,
} from "@/components/ui/dialog";
import {
  Field,
  FieldGroup,
  FieldLabel,
  FieldLegend,
  FieldSet,
} from "@/components/ui/field";
import { Input } from "@/components/ui/input";
import {
  NativeSelect,
  NativeSelectOption,
} from "@/components/ui/native-select";
import { Textarea } from "@/components/ui/textarea";
import { ToggleGroup, ToggleGroupItem } from "@/components/ui/toggle-group";
import { getMediaPreview, uploadMediaAsset } from "@/api/media";
import { cn } from "cn";
import { ProjectMediaPreview } from "@/components/media/project-media";
import type { Node } from "../document";
import {
  DIRECTOR_SCENE_COLORS,
  createDirectorConfig,
  createDirectorCamera,
  createDirectorLight,
  createDirectorObject,
  createDirectorShot,
  directorTransform,
  directorTransformAt,
  directorShotAt,
  directorBoneAt,
  directorPose,
  moveDirectorShot,
  putDirectorKeyframe,
  removeDirectorEntity,
  type DirectorSelection,
} from "./model";
export { createDirectorConfig } from "./model";

type Props = {
  node: Node;
  nodes: Node[];
  projectID?: string;
  onUpdate: (config: API.DirectorConfig) => void;
  onClose: () => void;
  onSave?: () => Promise<boolean>;
  saveMessage?: string;
  saving?: boolean;
};
type TransformMode = "translate" | "rotate" | "scale";
type StageHandle = {
  capture: () => Promise<Blob>;
  useView: () => API.DirectorCamera;
  home: () => void;
};
const poses = [
  ["stand", "站立"],
  ["t_pose", "T 字"],
  ["sit", "坐姿"],
  ["squat", "下蹲"],
  ["kneel_single", "单膝跪地"],
  ["kneel_double", "双膝跪地"],
  ["hands_hips", "双手叉腰"],
  ["lean", "倚靠"],
  ["bow", "鞠躬"],
  ["think", "思考"],
  ["fight", "格斗"],
  ["kick", "踢腿"],
  ["throw", "投掷"],
  ["push", "推物"],
  ["wave", "挥手"],
  ["reach", "伸手"],
  ["arms_crossed", "抱臂"],
  ["phone", "通话"],
  ["walk", "行走"],
  ["run", "跑步"],
];
const boneNames = [
  "hips",
  "spine",
  "chest",
  "neck",
  "head",
  "leftShoulder",
  "leftUpperArm",
  "leftLowerArm",
  "leftHand",
  "rightShoulder",
  "rightUpperArm",
  "rightLowerArm",
  "rightHand",
  "leftUpperLeg",
  "leftLowerLeg",
  "leftFoot",
  "rightUpperLeg",
  "rightLowerLeg",
  "rightFoot",
];
const radians = (degrees: number) => (degrees * Math.PI) / 180;
const degrees = (angle: number) => (angle * 180) / Math.PI;

function NumberField({
  label,
  value,
  onChange,
  min = -10000,
  max = 10000,
  step = 0.1,
}: {
  label: string;
  value: number;
  onChange: (value: number) => void;
  min?: number;
  max?: number;
  step?: number;
}) {
  return (
    <Field>
      <FieldLabel>
        {label}
        <Input
          aria-label={label}
          type="number"
          value={Number(value.toFixed(3))}
          min={min}
          max={max}
          step={step}
          onChange={(event) => {
            const value = event.currentTarget.valueAsNumber;
            if (Number.isFinite(value))
              onChange(Math.max(min, Math.min(max, value)));
          }}
        />
      </FieldLabel>
    </Field>
  );
}
function Choice({
  label,
  value,
  options,
  onChange,
}: {
  label: string;
  value: string;
  options: string[][];
  onChange: (value: string) => void;
}) {
  return (
    <Field>
      <FieldLabel>
        {label}
        <NativeSelect
          aria-label={label}
          value={value}
          onChange={(event) => onChange(event.target.value)}
          className="w-full"
        >
          {options.map(([key, title]) => (
            <NativeSelectOption key={key} value={key}>
              {title}
            </NativeSelectOption>
          ))}
        </NativeSelect>
      </FieldLabel>
    </Field>
  );
}
function Check({
  label,
  checked,
  onChange,
}: {
  label: string;
  checked: boolean;
  onChange: (value: boolean) => void;
}) {
  return (
    <Field orientation="horizontal">
      <FieldLabel>
        <Checkbox
          checked={checked}
          onCheckedChange={(value) => onChange(value === true)}
        />
        {label}
      </FieldLabel>
    </Field>
  );
}
function TransformFields({
  value,
  onChange,
}: {
  value: API.DirectorTransform | undefined;
  onChange: (value: API.DirectorTransform) => void;
}) {
  const transform = value ?? directorTransform();
  return (
    <FieldGroup className="gap-3">
      {(
        [
          ["position", "位置"],
          ["rotation", "旋转 °"],
          ["scale", "缩放"],
        ] as const
      ).map(([key, title]) => (
        <FieldSet key={key} className="gap-2">
          <FieldLegend variant="label">{title}</FieldLegend>
          <FieldGroup className="grid grid-cols-3 gap-2">
            {["X", "Y", "Z"].map((axis, i) => (
              <NumberField
                key={axis}
                label={`${title} ${axis}`}
                min={key === "scale" ? 0.01 : -10000}
                max={key === "scale" ? 100 : 10000}
                value={
                  key === "rotation"
                    ? degrees(transform[key]?.[i] ?? 0)
                    : (transform[key]?.[i] ?? (key === "scale" ? 1 : 0))
                }
                onChange={(value) => {
                  const vector = [
                    ...(transform[key] ??
                      (key === "scale" ? [1, 1, 1] : [0, 0, 0])),
                  ];
                  vector[i] = key === "rotation" ? radians(value) : value;
                  onChange({ ...transform, [key]: vector });
                }}
              />
            ))}
          </FieldGroup>
        </FieldSet>
      ))}
    </FieldGroup>
  );
}

export function CanvasDirector({
  node,
  nodes,
  projectID,
  onUpdate,
  onClose,
  onSave,
  saveMessage,
  saving = false,
}: Props) {
  const [saveNotice, setSaveNotice] = useState("");
  const [savePending, setSavePending] = useState(false);
  const [config, setConfig] = useState(() =>
    structuredClone(node.config.director ?? createDirectorConfig()),
  );
  const [selection, setSelection] = useState<DirectorSelection>();
  const [time, setTime] = useState(0),
    [playing, setPlaying] = useState(false);
  const [cameraView, setCameraView] = useState(false),
    [mode, setMode] = useState<TransformMode>("translate");
  const [easing, setEasing] = useState("linear"),
    [bone, setBone] = useState("head");
  const [error, setError] = useState(""),
    [pending, setPending] = useState(false);
  const [snapshot, setSnapshot] = useState<{
    file: File;
    key: string;
    shotID: string;
    url: string;
  }>();
  const [confirmed, setConfirmed] = useState(false);
  const stage = useRef<StageHandle | null>(null),
    snapshotBusy = useRef(false);
  const request = useRef<AbortController | null>(null);
  const current = useRef(config);
  const objects = config.objects ?? [],
    cameras = config.cameras ?? [],
    lights = config.lights ?? [],
    shots = config.shots ?? [];
  const shot = shots.find((s) => s.id === config.active_shot_id) ?? shots[0];
  const camera = cameras.find((c) => c.id === shot?.camera_id) ?? cameras[0];
  const total = shots.reduce((sum, s) => sum + (s.duration ?? 5), 0);
  const entity =
    selection?.kind === "object"
      ? objects.find((o) => o.id === selection.id)
      : selection?.kind === "camera"
        ? cameras.find((c) => c.id === selection.id)
        : lights.find((l) => l.id === selection?.id);
  const object =
    selection?.kind === "object"
      ? (entity as API.DirectorObject | undefined)
      : undefined;
  const selectedCamera =
    selection?.kind === "camera"
      ? (entity as API.DirectorCamera | undefined)
      : undefined;
  const light =
    selection?.kind === "light"
      ? (entity as API.DirectorLight | undefined)
      : undefined;
  const frames =
    entity && "keyframes" in entity ? (entity.keyframes ?? []) : [];
  const frameTime = Math.round(time * (shot?.fps ?? 24)) / (shot?.fps ?? 24);
  const selectedFrame = frames.find(
    (frame) => Math.abs((frame.time ?? 0) - frameTime) < 0.001,
  );
  function update(next: API.DirectorConfig) {
    setSaveNotice("");
    current.current = next;
    setConfig(next);
    onUpdate(next);
  }
  function patchEntity(
    patch: Partial<API.DirectorObject & API.DirectorCamera & API.DirectorLight>,
  ) {
    if (!selection) return;
    const key =
      selection.kind === "object"
        ? "objects"
        : selection.kind === "camera"
          ? "cameras"
          : "lights";
    update({
      ...current.current,
      [key]: current.current[key]?.map((item) =>
        item.id === selection.id ? { ...item, ...patch } : item,
      ),
    });
  }
  function changeTransform(
    transform: API.DirectorTransform,
    selected = selection,
  ) {
    if (!selected) return;
    const next = current.current,
      key =
        selected.kind === "object"
          ? "objects"
          : selected.kind === "camera"
            ? "cameras"
            : "lights";
    update({
      ...next,
      [key]: next[key]?.map((item) => {
        if (item.id !== selected.id) return item;
        const cameraPatch =
          selected.kind === "camera"
            ? {
                follow_object_id: undefined,
                ...(mode === "rotate"
                  ? { look_at_mode: "rotation", look_at_object_id: undefined }
                  : {}),
              }
            : {};
        if ("keyframes" in item && item.keyframes?.length)
          return {
            ...item,
            ...cameraPatch,
            keyframes: putDirectorKeyframe(
              item.keyframes,
              frameTime,
              transform,
              easing,
            ),
          };
        return { ...item, ...cameraPatch, transform };
      }),
    });
  }
  const tick = useEffectEvent((delta: number) => {
    const next = Math.min(total, time + delta);
    const active = directorShotAt(shots, Math.max(0, next - 0.00001));
    if (active && active.shot.id !== current.current.active_shot_id) {
      const nextConfig = { ...current.current, active_shot_id: active.shot.id };
      current.current = nextConfig;
      setConfig(nextConfig);
    }
    setTime(next);
    if (next >= total) setPlaying(false);
  });
  useEffect(() => {
    if (!playing) return;
    let last = performance.now(),
      frame = 0;
    function animate(now: number) {
      tick(Math.min((now - last) / 1000, 0.1));
      last = now;
      frame = requestAnimationFrame(animate);
    }
    frame = requestAnimationFrame(animate);
    return () => cancelAnimationFrame(frame);
  }, [playing]);
  useEffect(() => () => request.current?.abort(), []);
  useEffect(
    () => () => {
      if (snapshot) URL.revokeObjectURL(snapshot.url);
    },
    [snapshot],
  );
  function addObject(primitive: string) {
    if (objects.length >= 128) return;
    const object = createDirectorObject(primitive);
    update({ ...config, objects: [...objects, object] });
    setSelection({ kind: "object", id: object.id! });
  }
  function selectShot(id: string) {
    let start = 0;
    for (const item of shots) {
      if (item.id === id) break;
      start += item.duration ?? 5;
    }
    setPlaying(false);
    setTime(start);
    update({ ...config, active_shot_id: id });
  }
  function patchShot(patch: API.DirectorShot) {
    update({
      ...config,
      shots: shots.map((s) => (s.id === shot?.id ? { ...s, ...patch } : s)),
    });
  }
  async function saveCanvas() {
    if (!onSave || saving || savePending) return;
    setSavePending(true);
    try {
      setSaveNotice(
        (await onSave()) ? "已保存到画布。" : "尚未确认保存，请检查保存状态。",
      );
    } catch {
      setSaveNotice("尚未确认保存，请检查保存状态。");
    } finally {
      setSavePending(false);
    }
  }
  async function capture() {
    if (!stage.current || snapshotBusy.current || !shot?.id) return;
    snapshotBusy.current = true;
    setPending(true);
    setError("");
    try {
      const blob = await stage.current.capture();
      const file = new File(
        [blob],
        `${Array.from(shot.name ?? "镜头")
          .slice(0, 90)
          .join("")}-${frameTime.toFixed(2)}s.png`,
        { type: "image/png" },
      );
      setSnapshot({
        file,
        key: crypto.randomUUID(),
        shotID: shot.id,
        url: URL.createObjectURL(file),
      });
      setConfirmed(false);
    } catch {
      setError("无法截取当前场景，请确认 WebGL 及素材已加载。");
    } finally {
      snapshotBusy.current = false;
      setPending(false);
    }
  }
  async function saveSnapshot() {
    if (!snapshot || !projectID || !confirmed || snapshotBusy.current) return;
    snapshotBusy.current = true;
    setPending(true);
    setError("");
    const controller = new AbortController();
    request.current = controller;
    try {
      const receipt = await uploadMediaAsset(
        { pid: projectID },
        { local_review_confirmed: true },
        snapshot.file,
        {
          headers: { "Idempotency-Key": snapshot.key },
          signal: controller.signal,
        },
      );
      if (
        !receipt.asset?.id ||
        receipt.asset.project_id !== projectID ||
        receipt.asset.kind !== "image"
      )
        throw new Error("invalid_receipt");
      if (controller.signal.aborted) return;
      const assetID = receipt.asset.id,
        latest = current.current;
      update({
        ...latest,
        shots: latest.shots?.map((s) =>
          s.id === snapshot.shotID
            ? {
                ...s,
                screenshots: [
                  ...(s.screenshots ?? []),
                  {
                    id: snapshot.key,
                    asset_id: assetID,
                    name: snapshot.file.name,
                    created_at: new Date().toISOString(),
                  },
                ],
              }
            : s,
        ),
      });
      setSnapshot(undefined);
    } catch {
      if (!controller.signal.aborted)
        setError("快照尚未确认保存。可使用同一快照重试，或先下载保留。");
    } finally {
      snapshotBusy.current = false;
      setPending(false);
    }
  }
  function importNode(source: Node) {
    if (!source.ref_id || objects.length >= 128) return;
    const imported: API.DirectorObject = {
      ...createDirectorObject("plane"),
      name: source.title || "画布素材",
      kind: source.node_type === "model" ? "model" : "billboard",
      primitive: undefined,
      asset_id: source.ref_id,
      source_node_id: source.id,
      transform: directorTransform([0, 1, 0]),
      color: DIRECTOR_SCENE_COLORS.light,
    };
    update({ ...config, objects: [...objects, imported] });
    setSelection({ kind: "object", id: imported.id! });
  }
  const mediaNodes = nodes.filter(
    (n) =>
      ["image", "model"].includes(n.node_type) &&
      n.ref_type === "media_asset" &&
      n.ref_id,
  );
  const inspectorTransform = entity
    ? directorTransformAt(entity.transform, frames, time)
    : undefined;
  return (
    <Dialog
      open
      onOpenChange={(open) => {
        if (!open && !pending) onClose();
      }}
    >
      <DialogContent
        showCloseButton={false}
        className="flex h-[94dvh] w-[96vw] max-w-none flex-col gap-0 overflow-hidden p-0 sm:max-w-none"
        aria-describedby="director-description"
      >
        <header className="flex shrink-0 flex-wrap items-center justify-between gap-2 border-b px-4 py-3">
          <div>
            <DialogTitle>导演台</DialogTitle>
            <DialogDescription id="director-description">
              搭建场景、编排镜头与关键帧，修改同步到画布。
            </DialogDescription>
          </div>
          <div className="flex items-center gap-2">
            {onSave && (
              <Button
                size="sm"
                disabled={saving || savePending || pending}
                onClick={() => void saveCanvas()}
              >
                {saving || savePending ? "保存中…" : "保存画布"}
              </Button>
            )}
            <Button
              size="sm"
              variant="outline"
              disabled={pending}
              onClick={() => void capture()}
            >
              <CameraIcon />
              快照
            </Button>
            <Button
              size="icon-sm"
              variant="ghost"
              aria-label="关闭导演台"
              disabled={pending}
              onClick={onClose}
            >
              <X />
            </Button>
          </div>
        </header>
        {(saveMessage || saveNotice || saving) && (
          <div
            role="status"
            aria-live="polite"
            className="flex flex-wrap gap-2 border-b bg-muted/30 px-4 py-2 text-sm"
          >
            {saveNotice && <span>{saveNotice}</span>}
            {saveMessage && <span>{saveMessage}</span>}
            {saving && <span>正在保存画布…</span>}
          </div>
        )}
        {error && (
          <p role="alert" className="border-b px-4 py-2 text-destructive">
            {error}
          </p>
        )}
        <div className="grid min-h-0 flex-1 grid-cols-[minmax(0,1fr)] overflow-auto md:grid-cols-[180px_minmax(0,1fr)_260px]">
          <aside
            aria-label="场景大纲"
            className="flex flex-col gap-3 overflow-auto border-r p-3 max-md:max-h-44"
          >
            <div className="flex items-center justify-between">
              <h3 className="text-sm font-medium">场景</h3>
              <span className="text-xs text-muted-foreground">
                {objects.length} 个对象
              </span>
            </div>
            <div className="grid grid-cols-3 gap-1">
              <Button
                variant="outline"
                size="icon-sm"
                title="添加立方体"
                aria-label="添加立方体"
                disabled={objects.length >= 128}
                onClick={() => addObject("box")}
              >
                <Cuboid />
              </Button>
              <Button
                variant="outline"
                size="icon-sm"
                title="添加球体"
                aria-label="添加球体"
                disabled={objects.length >= 128}
                onClick={() => addObject("sphere")}
              >
                <Circle />
              </Button>
              <Button
                variant="outline"
                size="icon-sm"
                title="添加人偶"
                aria-label="添加人偶"
                disabled={objects.length >= 128}
                onClick={() => addObject("character")}
              >
                <UserRound />
              </Button>
            </div>
            <Choice
              label="添加其他物体"
              value=""
              options={[
                ["", "选择物体…"],
                ["cylinder", "圆柱"],
                ["plane", "平面"],
              ]}
              onChange={(v) => {
                if (v) addObject(v);
              }}
            />
            {objects.map((item) => (
              <Button
                key={item.id}
                className="justify-start truncate"
                variant={selection?.id === item.id ? "secondary" : "ghost"}
                size="sm"
                onClick={() => setSelection({ kind: "object", id: item.id! })}
              >
                <Cuboid />
                {item.name}
              </Button>
            ))}
            <div className="flex items-center justify-between">
              <h3 className="text-sm font-medium">摄像机</h3>
              <Button
                size="icon-sm"
                variant="ghost"
                aria-label="添加摄像机"
                disabled={cameras.length >= 16}
                onClick={() => {
                  const camera = createDirectorCamera(cameras.length + 1);
                  update({ ...config, cameras: [...cameras, camera] });
                  setSelection({ kind: "camera", id: camera.id! });
                }}
              >
                <Plus />
              </Button>
            </div>
            {cameras.map((item) => (
              <Button
                key={item.id}
                className="justify-start truncate"
                variant={selection?.id === item.id ? "secondary" : "ghost"}
                size="sm"
                onClick={() => setSelection({ kind: "camera", id: item.id! })}
              >
                <Camera />
                {item.name}
              </Button>
            ))}
            <div className="flex items-center justify-between">
              <h3 className="text-sm font-medium">灯光</h3>
              <Button
                size="icon-sm"
                variant="ghost"
                aria-label="添加灯光"
                disabled={lights.length >= 32}
                onClick={() => {
                  const light = createDirectorLight("point", lights.length + 1);
                  update({ ...config, lights: [...lights, light] });
                  setSelection({ kind: "light", id: light.id! });
                }}
              >
                <Plus />
              </Button>
            </div>
            {lights.map((item) => (
              <Button
                key={item.id}
                className="justify-start truncate"
                variant={selection?.id === item.id ? "secondary" : "ghost"}
                size="sm"
                onClick={() => setSelection({ kind: "light", id: item.id! })}
              >
                <Lightbulb />
                {item.name}
              </Button>
            ))}
            <h3 className="text-sm font-medium">画布素材</h3>
            {mediaNodes.length === 0 && (
              <p className="text-xs text-muted-foreground">
                将图片或 GLB 模型添加到画布后，可在这里置入场景。
              </p>
            )}
            {mediaNodes.map((source) => (
              <Button
                key={source.id}
                variant="outline"
                size="sm"
                disabled={!projectID || objects.length >= 128}
                onClick={() => importNode(source)}
                className="justify-start truncate"
              >
                <Plus />
                {source.title}
              </Button>
            ))}
          </aside>
          <section
            className="flex min-h-[380px] min-w-0 flex-col bg-muted/30"
            aria-label="3D 场景"
          >
            <div className="flex shrink-0 flex-wrap items-center justify-between gap-2 border-b bg-background p-2">
              <ToggleGroup
                type="single"
                value={mode}
                onValueChange={(value) => {
                  if (value) setMode(value as TransformMode);
                }}
                aria-label="变换工具"
              >
                <ToggleGroupItem value="translate" aria-label="移动">
                  <Move3D />
                </ToggleGroupItem>
                <ToggleGroupItem value="rotate" aria-label="旋转">
                  <Rotate3D />
                </ToggleGroupItem>
                <ToggleGroupItem value="scale" aria-label="缩放">
                  <Scaling />
                </ToggleGroupItem>
              </ToggleGroup>
              <div className="flex gap-1">
                <Button
                  size="sm"
                  variant={cameraView ? "secondary" : "ghost"}
                  aria-pressed={cameraView}
                  onClick={() => setCameraView(!cameraView)}
                >
                  {cameraView ? "返回场景" : "镜头预览"}
                </Button>
                <Button
                  size="sm"
                  variant="ghost"
                  onClick={() => stage.current?.home()}
                >
                  重置视角
                </Button>
              </div>
            </div>
            <DirectorStage
              config={config}
              selection={selection}
              mode={mode}
              time={time}
              cameraView={cameraView}
              projectID={projectID}
              handle={stage}
              onSelect={setSelection}
              onTransform={changeTransform}
              onModelInfo={(id, rig) => {
                const latest = current.current;
                if (!latest.objects?.some((o) => o.id === id)) return;
                update({
                  ...latest,
                  objects: latest.objects.map((o) =>
                    o.id === id ? { ...o, rig: { ...o.rig, ...rig } } : o,
                  ),
                });
              }}
            />
            <div className="flex shrink-0 flex-wrap justify-between gap-2 border-t bg-background px-3 py-2 text-xs text-muted-foreground">
              <span>左键旋转 · 右键平移 · 滚轮缩放 · 点击选择</span>
              <span>
                {cameraView ? camera?.name : "自由视角"} ·{" "}
                {frameTime.toFixed(2)}s
              </span>
            </div>
          </section>
          <aside
            aria-label="导演台属性"
            className="flex flex-col gap-4 overflow-auto border-l p-3"
          >
            {entity && selection ? (
              <>
                <div className="flex items-center justify-between">
                  <h3 className="text-sm font-medium">
                    {selection.kind === "object"
                      ? "对象"
                      : selection.kind === "camera"
                        ? "摄像机"
                        : "灯光"}
                    属性
                  </h3>
                  <Button
                    size="icon-sm"
                    variant="ghost"
                    aria-label="删除选中对象"
                    disabled={
                      selection.kind === "camera" && cameras.length <= 1
                    }
                    onClick={() => {
                      update(
                        removeDirectorEntity(
                          config,
                          selection.kind,
                          selection.id,
                        ),
                      );
                      setSelection(undefined);
                    }}
                  >
                    <Trash2 />
                  </Button>
                </div>
                <FieldGroup className="gap-3">
                  <Field>
                    <FieldLabel>
                      {selection.kind === "object" ? "对象名称" : "名称"}
                      <Input
                        aria-label={
                          selection.kind === "object" ? "对象名称" : "名称"
                        }
                        value={entity.name ?? ""}
                        maxLength={128}
                        onChange={(e) => {
                          if (e.target.value.trim())
                            patchEntity({ name: e.target.value });
                        }}
                      />
                    </FieldLabel>
                  </Field>
                  <TransformFields
                    value={inspectorTransform}
                    onChange={(value) => changeTransform(value)}
                  />
                  {object && (
                    <>
                      <Field>
                        <FieldLabel>
                          对象颜色
                          <Input
                            type="color"
                            aria-label="对象颜色"
                            value={object.color ?? DIRECTOR_SCENE_COLORS.object}
                            onChange={(e) =>
                              patchEntity({ color: e.target.value })
                            }
                          />
                        </FieldLabel>
                      </Field>
                      <Check
                        label="显示对象"
                        checked={object.visible !== false}
                        onChange={(visible) => patchEntity({ visible })}
                      />
                      <Check
                        label="投射阴影"
                        checked={!!object.cast_shadow}
                        onChange={(cast_shadow) => patchEntity({ cast_shadow })}
                      />
                      {object.builtin_actor === "mannequin" && (
                        <Choice
                          label="人偶姿态"
                          value={object.pose ?? "stand"}
                          options={poses}
                          onChange={(pose) => patchEntity({ pose })}
                        />
                      )}
                      {(object.builtin_actor === "mannequin" ||
                        object.rig?.bone_map) && (
                        <BoneEditor
                          object={object}
                          bone={bone}
                          onBone={setBone}
                          time={frameTime}
                          easing={easing}
                          onChange={patchEntity}
                          onTime={(value) => {
                            setTime(value);
                            setPlaying(false);
                          }}
                        />
                      )}
                      {(object.rig?.animation_names?.length ?? 0) > 0 && (
                        <>
                          <Choice
                            label="模型动作"
                            value={
                              object.motion_clips?.find(
                                (c) => c.id === object.active_motion_clip_id,
                              )?.source_animation ?? ""
                            }
                            options={[
                              ["", "不播放"],
                              ...(object.rig?.animation_names ?? []).map(
                                (name) => [name, name],
                              ),
                            ]}
                            onChange={(name) => {
                              if (!name)
                                return patchEntity({
                                  active_motion_clip_id: undefined,
                                  motion_clips: [],
                                });
                              const clip: API.DirectorMotionClip = {
                                id: crypto.randomUUID(),
                                name,
                                source_animation: name,
                                start: 0,
                                duration: total,
                                playback_rate: 1,
                                loop: true,
                              };
                              patchEntity({
                                motion_clips: [clip],
                                active_motion_clip_id: clip.id,
                              });
                            }}
                          />
                          {object.motion_clips?.map((clip) => (
                            <FieldSet key={clip.id} className="gap-2">
                              <FieldLegend variant="label">
                                {clip.name}
                              </FieldLegend>
                              <NumberField
                                label="动作开始 s"
                                value={clip.start ?? 0}
                                min={0}
                                max={Math.max(0, 3600 - (clip.duration ?? 1))}
                                onChange={(start) =>
                                  patchEntity({
                                    motion_clips: object.motion_clips?.map(
                                      (c) =>
                                        c.id === clip.id ? { ...c, start } : c,
                                    ),
                                  })
                                }
                              />
                              <NumberField
                                label="动作时长 s"
                                value={clip.duration ?? 1}
                                min={0.1}
                                max={3600 - (clip.start ?? 0)}
                                onChange={(duration) =>
                                  patchEntity({
                                    motion_clips: object.motion_clips?.map(
                                      (c) =>
                                        c.id === clip.id
                                          ? { ...c, duration }
                                          : c,
                                    ),
                                  })
                                }
                              />
                              <NumberField
                                label="动作速度"
                                value={clip.playback_rate ?? 1}
                                min={0.1}
                                max={4}
                                onChange={(playback_rate) =>
                                  patchEntity({
                                    motion_clips: object.motion_clips?.map(
                                      (c) =>
                                        c.id === clip.id
                                          ? { ...c, playback_rate }
                                          : c,
                                    ),
                                  })
                                }
                              />
                              <Check
                                label="循环动作"
                                checked={!!clip.loop}
                                onChange={(loop) =>
                                  patchEntity({
                                    motion_clips: object.motion_clips?.map(
                                      (c) =>
                                        c.id === clip.id ? { ...c, loop } : c,
                                    ),
                                  })
                                }
                              />
                            </FieldSet>
                          ))}
                        </>
                      )}
                    </>
                  )}
                  {selectedCamera && (
                    <>
                      <NumberField
                        label="视野 °"
                        value={selectedCamera.fov ?? 50}
                        min={1}
                        max={179}
                        step={1}
                        onChange={(fov) =>
                          patchEntity({
                            fov,
                            focal_length: 35 / (2 * Math.tan(radians(fov) / 2)),
                          })
                        }
                      />
                      <NumberField
                        label="焦距 mm"
                        value={selectedCamera.focal_length ?? 35}
                        min={1}
                        max={1000}
                        onChange={(focal_length) =>
                          patchEntity({
                            focal_length,
                            fov: degrees(
                              2 * Math.atan(35 / (2 * focal_length)),
                            ),
                          })
                        }
                      />
                      <Choice
                        label="朝向方式"
                        value={selectedCamera.look_at_mode ?? "coordinates"}
                        options={[
                          ["coordinates", "目标坐标"],
                          ["rotation", "旋转角度"],
                          ["object", "追踪对象"],
                        ]}
                        onChange={(look_at_mode) =>
                          patchEntity({ look_at_mode })
                        }
                      />
                      {selectedCamera.look_at_mode !== "rotation" && (
                        <>
                          <FieldSet>
                            <FieldLegend variant="label">目标坐标</FieldLegend>
                            <FieldGroup className="grid grid-cols-3 gap-2">
                              {["X", "Y", "Z"].map((axis, i) => (
                                <NumberField
                                  key={axis}
                                  label={`目标 ${axis}`}
                                  value={selectedCamera.target?.[i] ?? 0}
                                  onChange={(v) => {
                                    const target = [
                                      ...(selectedCamera.target ?? [0, 0, 0]),
                                    ];
                                    target[i] = v;
                                    patchEntity({ target });
                                  }}
                                />
                              ))}
                            </FieldGroup>
                          </FieldSet>
                          <Choice
                            label="注视对象"
                            value={selectedCamera.look_at_object_id ?? ""}
                            options={[
                              ["", "无"],
                              ...objects.map((o) => [o.id!, o.name!]),
                            ]}
                            onChange={(id) =>
                              patchEntity({
                                look_at_object_id: id || undefined,
                              })
                            }
                          />
                        </>
                      )}
                      <Choice
                        label="跟随对象"
                        value={selectedCamera.follow_object_id ?? ""}
                        options={[
                          ["", "无"],
                          ...objects.map((o) => [o.id!, o.name!]),
                        ]}
                        onChange={(id) =>
                          patchEntity({ follow_object_id: id || undefined })
                        }
                      />
                      <Button
                        variant="outline"
                        size="sm"
                        onClick={() => {
                          const view = stage.current?.useView();
                          if (view)
                            patchEntity({
                              transform: view.transform,
                              target: view.target,
                              fov: view.fov,
                              look_at_mode: "coordinates",
                              follow_object_id: undefined,
                              look_at_object_id: undefined,
                            });
                        }}
                      >
                        使用当前自由视角
                      </Button>
                    </>
                  )}
                  {light && (
                    <>
                      <Choice
                        label="灯光类型"
                        value={light.type ?? "point"}
                        options={[
                          ["point", "点光"],
                          ["directional", "平行光"],
                          ["spot", "聚光"],
                          ["ambient", "环境光"],
                        ]}
                        onChange={(type) => patchEntity({ type })}
                      />
                      <NumberField
                        label="光线强度"
                        value={light.intensity ?? 1}
                        min={0}
                        max={100}
                        onChange={(intensity) => patchEntity({ intensity })}
                      />
                      <Field>
                        <FieldLabel>
                          灯光颜色
                          <Input
                            aria-label="灯光颜色"
                            type="color"
                            value={light.color ?? DIRECTOR_SCENE_COLORS.light}
                            onChange={(e) =>
                              patchEntity({ color: e.target.value })
                            }
                          />
                        </FieldLabel>
                      </Field>
                      {light.type === "spot" && (
                        <>
                          <NumberField
                            label="光锥角度 °"
                            value={degrees(light.angle ?? Math.PI / 4)}
                            min={1}
                            max={90}
                            onChange={(angle) =>
                              patchEntity({ angle: radians(angle) })
                            }
                          />
                          <NumberField
                            label="边缘柔和"
                            value={light.penumbra ?? 0.3}
                            min={0}
                            max={1}
                            onChange={(penumbra) => patchEntity({ penumbra })}
                          />
                        </>
                      )}
                      <Check
                        label="灯光投影"
                        checked={!!light.cast_shadow}
                        onChange={(cast_shadow) => patchEntity({ cast_shadow })}
                      />
                    </>
                  )}
                </FieldGroup>
              </>
            ) : (
              <>
                <h3 className="text-sm font-medium">场景设置</h3>
                <FieldGroup className="gap-3">
                  <Field>
                    <FieldLabel>
                      场景名称
                      <Input
                        value={config.title ?? ""}
                        maxLength={128}
                        onChange={(e) => {
                          if (e.target.value.trim())
                            update({ ...config, title: e.target.value });
                        }}
                      />
                    </FieldLabel>
                  </Field>
                  <Field>
                    <FieldLabel>
                      背景颜色
                      <Input
                        aria-label="背景颜色"
                        type="color"
                        value={
                          config.background ?? DIRECTOR_SCENE_COLORS.background
                        }
                        onChange={(e) =>
                          update({ ...config, background: e.target.value })
                        }
                      />
                    </FieldLabel>
                  </Field>
                  <Choice
                    label="画面比例"
                    value={config.aspect_ratio ?? "16:9"}
                    options={["21:9", "16:9", "4:3", "1:1", "3:4", "9:16"].map(
                      (v) => [v, v],
                    )}
                    onChange={(aspect_ratio) =>
                      update({ ...config, aspect_ratio })
                    }
                  />
                  <NumberField
                    label="环境亮度"
                    value={config.environment_intensity ?? 0.5}
                    min={0}
                    max={10}
                    onChange={(environment_intensity) =>
                      update({ ...config, environment_intensity })
                    }
                  />
                  <Check
                    label="显示网格"
                    checked={!!config.grid_visible}
                    onChange={(grid_visible) =>
                      update({ ...config, grid_visible })
                    }
                  />
                  <Check
                    label="网格吸附"
                    checked={!!config.grid_snap}
                    onChange={(grid_snap) => update({ ...config, grid_snap })}
                  />
                  <Check
                    label="显示地面"
                    checked={config.ground?.visible !== false}
                    onChange={(visible) =>
                      update({
                        ...config,
                        ground: {
                          ...config.ground,
                          visible,
                          height: config.ground?.height ?? 0,
                          opacity: config.ground?.opacity ?? 1,
                        },
                      })
                    }
                  />
                  <NumberField
                    label="地面高度"
                    value={config.ground?.height ?? 0}
                    min={-2}
                    max={2}
                    onChange={(height) =>
                      update({
                        ...config,
                        ground: {
                          visible: config.ground?.visible ?? true,
                          opacity: config.ground?.opacity ?? 1,
                          height,
                        },
                      })
                    }
                  />
                  <NumberField
                    label="地面不透明度"
                    value={config.ground?.opacity ?? 1}
                    min={0}
                    max={1}
                    onChange={(opacity) =>
                      update({
                        ...config,
                        ground: {
                          visible: config.ground?.visible ?? true,
                          height: config.ground?.height ?? 0,
                          opacity,
                        },
                      })
                    }
                  />
                  <NumberField
                    label="整体场景缩放"
                    value={config.stage_transform?.scale ?? 1}
                    min={0.1}
                    max={10}
                    onChange={(scale) =>
                      update({
                        ...config,
                        stage_transform: {
                          position: config.stage_transform?.position ?? [
                            0, 0, 0,
                          ],
                          rotation: config.stage_transform?.rotation ?? [
                            0, 0, 0,
                          ],
                          scale,
                        },
                      })
                    }
                  />
                </FieldGroup>
              </>
            )}
            <Button
              size="sm"
              variant="ghost"
              onClick={() => setSelection(undefined)}
            >
              场景设置
            </Button>
            {shot && (
              <FieldSet className="border-t pt-3">
                <FieldLegend variant="label">镜头设置</FieldLegend>
                <FieldGroup className="gap-3">
                  <Field>
                    <FieldLabel>
                      镜头名称
                      <Input
                        value={shot.name ?? ""}
                        maxLength={128}
                        onChange={(e) => {
                          if (e.target.value.trim())
                            patchShot({ name: e.target.value });
                        }}
                      />
                    </FieldLabel>
                  </Field>
                  <Choice
                    label="镜头摄像机"
                    value={shot.camera_id ?? ""}
                    options={cameras.map((c) => [c.id!, c.name!])}
                    onChange={(camera_id) => patchShot({ camera_id })}
                  />
                  <NumberField
                    label="镜头时长 s"
                    value={shot.duration ?? 5}
                    min={0.1}
                    max={Math.min(3600, 3600 - total + (shot.duration ?? 5))}
                    onChange={(duration) => patchShot({ duration })}
                  />
                  <Choice
                    label="帧率"
                    value={String(shot.fps ?? 24)}
                    options={["24", "25", "30"].map((v) => [v, v])}
                    onChange={(v) => patchShot({ fps: Number(v) })}
                  />
                  <Choice
                    label="景别"
                    value={shot.shot_size ?? "full"}
                    options={[
                      ["extreme_wide", "大远景"],
                      ["wide", "远景"],
                      ["full", "全景"],
                      ["medium", "中景"],
                      ["close_up", "近景"],
                      ["extreme_close_up", "特写"],
                    ]}
                    onChange={(shot_size) => patchShot({ shot_size })}
                  />
                  <Choice
                    label="镜头运动意图"
                    value={shot.camera_move ?? "static"}
                    options={[
                      ["static", "固定"],
                      ["push_in", "推镜"],
                      ["pull_out", "拉镜"],
                      ["pan_left", "左摇"],
                      ["pan_right", "右摇"],
                      ["tilt_up", "上摇"],
                      ["tilt_down", "下摇"],
                      ["orbit_left", "左环绕"],
                      ["orbit_right", "右环绕"],
                      ["handheld", "手持"],
                    ]}
                    onChange={(camera_move) => patchShot({ camera_move })}
                  />
                  <p className="text-xs text-muted-foreground">
                    运动意图用于镜头说明；实际运动由摄像机关键帧控制。
                  </p>
                  <Field>
                    <FieldLabel>
                      镜头描述
                      <Textarea
                        value={shot.prompt ?? ""}
                        maxLength={10000}
                        onChange={(e) => patchShot({ prompt: e.target.value })}
                      />
                    </FieldLabel>
                  </Field>
                </FieldGroup>
              </FieldSet>
            )}
            {projectID && (shot?.screenshots?.length ?? 0) > 0 && (
              <FieldSet className="border-t pt-3">
                <FieldLegend variant="label">镜头快照</FieldLegend>
                <div className="grid grid-cols-2 gap-2">
                  {shot?.screenshots?.map((item) => (
                    <div key={item.id} className="flex flex-col gap-1">
                      <div className="relative aspect-video overflow-hidden rounded-md border">
                        <ProjectMediaPreview
                          projectID={projectID}
                          id={item.asset_id!}
                          kind="image"
                        />
                      </div>
                      <span className="truncate text-xs" title={item.name}>
                        {item.name}
                      </span>
                      <Button
                        size="xs"
                        variant={
                          config.cover?.asset_id === item.asset_id
                            ? "secondary"
                            : "outline"
                        }
                        onClick={() =>
                          update({
                            ...config,
                            cover: {
                              asset_id: item.asset_id,
                              shot_id: shot.id,
                            },
                          })
                        }
                      >
                        {config.cover?.asset_id === item.asset_id
                          ? "当前封面"
                          : "设为封面"}
                      </Button>
                      <Button
                        size="xs"
                        variant="ghost"
                        aria-label={`移除快照${item.name}`}
                        onClick={() =>
                          patchShot({
                            screenshots: shot.screenshots?.filter(
                              (s) => s.id !== item.id,
                            ),
                          })
                        }
                      >
                        移除快照
                      </Button>
                    </div>
                  ))}
                </div>
              </FieldSet>
            )}
          </aside>
        </div>
        <footer className="flex shrink-0 flex-col gap-2 border-t bg-background px-4 py-3">
          <div className="flex flex-wrap items-center gap-2">
            <Button
              size="icon-sm"
              variant="outline"
              aria-label={playing ? "暂停" : "播放"}
              onClick={() => {
                if (time >= total) setTime(0);
                setPlaying(!playing);
              }}
            >
              {playing ? <Pause /> : <Play />}
            </Button>
            <span className="min-w-24 text-xs tabular-nums">
              {time.toFixed(2)} / {total.toFixed(2)}s
            </span>
            <Input
              aria-label="时间轴"
              className="h-6 min-w-24 flex-1 p-0"
              type="range"
              min={0}
              max={total}
              step={1 / (shot?.fps ?? 24)}
              value={time}
              onChange={(e) => {
                setPlaying(false);
                const next = Number(e.target.value);
                setTime(next);
                const active = directorShotAt(shots, next);
                if (active)
                  update({ ...config, active_shot_id: active.shot.id });
              }}
            />
            <Button
              size="sm"
              variant="outline"
              aria-label="添加关键帧"
              disabled={
                !entity || selection?.kind === "light" || frames.length >= 256
              }
              onClick={() => {
                if (entity)
                  patchEntity({
                    keyframes: putDirectorKeyframe(
                      frames,
                      frameTime,
                      inspectorTransform!,
                      easing,
                    ),
                  });
              }}
            >
              ◆ 关键帧
            </Button>
            {selectedFrame && (
              <Button
                size="icon-sm"
                variant="ghost"
                aria-label="删除当前关键帧"
                onClick={() =>
                  patchEntity({
                    keyframes: frames.filter((f) => f.id !== selectedFrame.id),
                  })
                }
              >
                <Trash2 />
              </Button>
            )}
            <NativeSelect
              aria-label="关键帧插值"
              size="sm"
              value={easing}
              onChange={(e) => {
                setEasing(e.target.value);
                if (selectedFrame)
                  patchEntity({
                    keyframes: frames.map((f) =>
                      f.id === selectedFrame.id
                        ? { ...f, easing: e.target.value }
                        : f,
                    ),
                  });
              }}
            >
              <NativeSelectOption value="linear">线性</NativeSelectOption>
              <NativeSelectOption value="smooth">平滑</NativeSelectOption>
              <NativeSelectOption value="step">保持</NativeSelectOption>
            </NativeSelect>
          </div>
          <div className="flex min-h-9 gap-2 overflow-x-auto">
            {shots.map((item, i) => (
              <div
                key={item.id}
                className={cn(
                  "flex shrink-0 items-center gap-1 rounded-md border px-1",
                  item.id === shot?.id && "border-ring bg-muted",
                )}
              >
                <Button
                  size="sm"
                  variant="ghost"
                  onClick={() => selectShot(item.id!)}
                >
                  {i + 1}. {item.name} · {item.duration}s
                </Button>
                {shots.length > 1 && (
                  <>
                    <Button
                      size="icon-xs"
                      variant="ghost"
                      aria-label={`前移${item.name}`}
                      disabled={i === 0}
                      onClick={() => {
                        update(moveDirectorShot(config, item.id!, -1));
                        setPlaying(false);
                      }}
                    >
                      <ArrowLeft />
                    </Button>
                    <Button
                      size="icon-xs"
                      variant="ghost"
                      aria-label={`后移${item.name}`}
                      disabled={i === shots.length - 1}
                      onClick={() => {
                        update(moveDirectorShot(config, item.id!, 1));
                        setPlaying(false);
                      }}
                    >
                      <ArrowRight />
                    </Button>
                  </>
                )}
                {shots.length > 1 && (
                  <Button
                    size="icon-xs"
                    variant="ghost"
                    aria-label={`删除${item.name}`}
                    onClick={() => {
                      const rest = shots.filter((s) => s.id !== item.id);
                      update({
                        ...config,
                        shots: rest,
                        active_shot_id:
                          item.id === config.active_shot_id
                            ? rest[0].id
                            : config.active_shot_id,
                        cover:
                          config.cover?.shot_id === item.id
                            ? undefined
                            : config.cover,
                      });
                      setTime(0);
                      setPlaying(false);
                    }}
                  >
                    <X />
                  </Button>
                )}
              </div>
            ))}
            <Button
              size="sm"
              variant="outline"
              aria-label="添加镜头"
              disabled={shots.length >= 128 || total > 3595}
              onClick={() => {
                const next = createDirectorShot(
                  camera?.id ?? cameras[0].id!,
                  shots.length + 1,
                );
                update({
                  ...config,
                  shots: [...shots, next],
                  active_shot_id: next.id,
                });
                setTime(total);
                setPlaying(false);
              }}
            >
              <Plus />
              镜头
            </Button>
          </div>
          {frames.length > 0 && (
            <div aria-label="关键帧列表" className="flex gap-1 overflow-x-auto">
              {frames.map((frame) => (
                <Button
                  key={frame.id}
                  variant={
                    frame.id === selectedFrame?.id ? "secondary" : "ghost"
                  }
                  size="xs"
                  onClick={() => {
                    setTime(frame.time ?? 0);
                    setPlaying(false);
                  }}
                >
                  ◆ {frame.time?.toFixed(2)}s
                </Button>
              ))}
            </div>
          )}
        </footer>
        {snapshot && (
          <Dialog
            open
            onOpenChange={(open) => {
              if (!open && !pending) setSnapshot(undefined);
            }}
          >
            <DialogContent>
              <DialogTitle>场景快照</DialogTitle>
              <DialogDescription>
                当前镜头的真实 PNG 渲染，可下载或保存到项目素材库。
              </DialogDescription>
              {error && (
                <p role="alert" className="text-destructive">
                  {error}
                </p>
              )}
              <Image
                src={snapshot.url}
                alt="当前场景快照"
                width={640}
                height={360}
                unoptimized
                className="max-h-64 w-full object-contain"
              />
              <Button asChild variant="outline">
                <a href={snapshot.url} download={snapshot.file.name}>
                  <Download />
                  下载 PNG
                </a>
              </Button>
              <Check
                label="已确认素材使用权限且不含需另行授权的真人素材"
                checked={confirmed}
                onChange={setConfirmed}
              />
              <Button
                disabled={
                  !confirmed ||
                  !projectID ||
                  pending ||
                  shots.reduce(
                    (sum, item) => sum + (item.screenshots?.length ?? 0),
                    0,
                  ) >= 512 ||
                  (shots.find((s) => s.id === snapshot.shotID)?.screenshots
                    ?.length ?? 0) >= 64
                }
                onClick={() => void saveSnapshot()}
              >
                {pending ? "正在保存…" : "保存到项目素材库"}
              </Button>
            </DialogContent>
          </Dialog>
        )}
      </DialogContent>
    </Dialog>
  );
}

function BoneEditor({
  object,
  bone,
  onBone,
  time,
  easing,
  onChange,
  onTime,
}: {
  object: API.DirectorObject;
  bone: string;
  onBone: (bone: string) => void;
  time: number;
  easing: string;
  onChange: (patch: API.DirectorObject) => void;
  onTime: (time: number) => void;
}) {
  const available =
    object.builtin_actor === "mannequin"
      ? boneNames
      : Object.keys(object.rig?.bone_map ?? {}).filter((key) =>
          boneNames.includes(key),
        );
  const currentBone = available.includes(bone) ? bone : available[0];
  if (!currentBone) return null;
  const quaternion = object.bone_overrides?.[currentBone] as
    number[] | undefined;
  // Quaternion conversion remains pure so editing never requires a loaded WebGL scene.
  const q = directorBoneAt(
    object.bone_tracks?.find((track) => track.bone === currentBone)?.keyframes,
    time,
    quaternion ?? [0, 0, 0, 1],
  );
  const angles = [
    Math.atan2(
      2 * (q[3] * q[0] - q[1] * q[2]),
      1 - 2 * (q[0] ** 2 + q[1] ** 2),
    ),
    Math.asin(Math.max(-1, Math.min(1, 2 * (q[3] * q[1] + q[2] * q[0])))),
    Math.atan2(
      2 * (q[3] * q[2] - q[0] * q[1]),
      1 - 2 * (q[1] ** 2 + q[2] ** 2),
    ),
  ];
  function setAngle(index: number, value: number) {
    const next = [...angles];
    next[index] = radians(value);
    const [x, y, z] = next.map((a) => a / 2),
      [c1, c2, c3] = [Math.cos(x), Math.cos(y), Math.cos(z)],
      [s1, s2, s3] = [Math.sin(x), Math.sin(y), Math.sin(z)];
    const rotation = [
      s1 * c2 * c3 + c1 * s2 * s3,
      c1 * s2 * c3 - s1 * c2 * s3,
      c1 * c2 * s3 + s1 * s2 * c3,
      c1 * c2 * c3 - s1 * s2 * s3,
    ];
    const tracks = object.bone_tracks ?? [],
      track = tracks.find((t) => t.bone === currentBone);
    if (track?.keyframes?.length) {
      const existing = track.keyframes.find(
        (f) => Math.abs((f.time ?? 0) - time) < 0.001,
      );
      if (!existing && track.keyframes.length >= 256) return;
      onChange({
        bone_tracks: tracks.map((t) =>
          t.bone === currentBone
            ? {
                ...t,
                keyframes: [
                  ...(t.keyframes ?? []).filter((f) => f.id !== existing?.id),
                  {
                    id: existing?.id ?? crypto.randomUUID(),
                    time,
                    rotation,
                    easing,
                  },
                ].toSorted((a, b) => a.time! - b.time!),
              }
            : t,
        ),
      });
    } else
      onChange({
        bone_overrides: { ...object.bone_overrides, [currentBone]: rotation },
      });
  }
  return (
    <FieldSet>
      <FieldLegend variant="label">骨骼姿态</FieldLegend>
      <FieldGroup className="gap-3">
        <Choice
          label="骨骼"
          value={currentBone}
          options={available.map((name) => [name, name])}
          onChange={onBone}
        />
        <FieldGroup className="grid grid-cols-3 gap-2">
          {["X", "Y", "Z"].map((axis, i) => (
            <NumberField
              key={axis}
              label={`骨骼 ${axis} °`}
              min={-180}
              max={180}
              value={degrees(angles[i])}
              onChange={(v) => setAngle(i, v)}
            />
          ))}
        </FieldGroup>
        <Button
          size="sm"
          variant="outline"
          disabled={
            (object.bone_tracks?.find((t) => t.bone === currentBone)?.keyframes
              ?.length ?? 0) >= 256
          }
          onClick={() => {
            const tracks = object.bone_tracks ?? [],
              track = tracks.find((t) => t.bone === currentBone);
            const existing = track?.keyframes?.find(
              (f) => Math.abs((f.time ?? 0) - time) < 0.001,
            );
            const keyframes = [
              ...(track?.keyframes ?? []).filter((f) => f.id !== existing?.id),
              {
                id: existing?.id ?? crypto.randomUUID(),
                time,
                rotation: [...q],
                easing,
              },
            ].toSorted((a, b) => a.time! - b.time!);
            onChange({
              bone_tracks: [
                ...tracks.filter((t) => t.bone !== currentBone),
                { bone: currentBone, keyframes },
              ],
            });
          }}
        >
          添加骨骼关键帧
        </Button>
        <div className="flex flex-wrap gap-1">
          {object.bone_tracks
            ?.find((t) => t.bone === currentBone)
            ?.keyframes?.map((frame) => (
              <div key={frame.id} className="flex items-center">
                <Button
                  size="xs"
                  variant="ghost"
                  onClick={() => onTime(frame.time ?? 0)}
                >
                  ◆ {frame.time?.toFixed(2)}s
                </Button>
                <Button
                  size="icon-xs"
                  variant="ghost"
                  aria-label={`删除骨骼关键帧${frame.time}`}
                  onClick={() =>
                    onChange({
                      bone_tracks: object.bone_tracks?.map((t) =>
                        t.bone === currentBone
                          ? {
                              ...t,
                              keyframes: t.keyframes?.filter(
                                (f) => f.id !== frame.id,
                              ),
                            }
                          : t,
                      ),
                    })
                  }
                >
                  <X />
                </Button>
              </div>
            ))}
        </div>
      </FieldGroup>
    </FieldSet>
  );
}

type StageProps = {
  config: API.DirectorConfig;
  selection: DirectorSelection | undefined;
  mode: TransformMode;
  time: number;
  cameraView: boolean;
  projectID?: string;
  handle: React.RefObject<StageHandle | null>;
  onSelect: (value: DirectorSelection | undefined) => void;
  onModelInfo: (id: string, rig: API.DirectorRig) => void;
  onTransform: (
    value: API.DirectorTransform,
    selected?: DirectorSelection,
  ) => void;
};
function DirectorStage(props: StageProps) {
  const container = useRef<HTMLDivElement>(null);
  const runtime = useRef<StageHandle | null>(null);
  useImperativeHandle(
    props.handle,
    () => ({
      capture: () =>
        runtime.current?.capture() ??
        Promise.reject(new Error("视口尚未初始化")),
      useView: () => runtime.current?.useView() ?? {},
      home: () => runtime.current?.home(),
    }),
    [],
  );
  const [attempt, setAttempt] = useState(0);
  const [status, setStatus] = useState("正在初始化 3D 场景…"),
    [failure, setFailure] = useState("");
  const read = useEffectEvent(() => props);
  const select = useEffectEvent((value: DirectorSelection | undefined) =>
    props.onSelect(value),
  );
  const transform = useEffectEvent(
    (value: API.DirectorTransform, selected: DirectorSelection) =>
      props.onTransform(value, selected),
  );
  const modelInfo = useEffectEvent((id: string, rig: API.DirectorRig) =>
    props.onModelInfo(id, rig),
  );
  const { projectID } = props;
  useEffect(() => {
    const element = container.current;
    if (!element) return;
    const host: HTMLDivElement = element;
    let disposed = false,
      destroy: (() => void) | undefined;
    const controller = new AbortController();
    async function mount() {
      const [
        T,
        { OrbitControls },
        { TransformControls },
        { GLTFLoader },
        { clone },
      ] = await Promise.all([
        import("three"),
        import("three/addons/controls/OrbitControls.js"),
        import("three/addons/controls/TransformControls.js"),
        import("three/addons/loaders/GLTFLoader.js"),
        import("three/addons/utils/SkeletonUtils.js"),
      ]);
      if (disposed) return;
      const renderer = new T.WebGLRenderer({
        antialias: true,
        preserveDrawingBuffer: true,
      });
      renderer.setPixelRatio(Math.min(devicePixelRatio, 2));
      renderer.shadowMap.enabled = true;
      renderer.shadowMap.type = T.PCFSoftShadowMap;
      renderer.outputColorSpace = T.SRGBColorSpace;
      renderer.toneMapping = T.ACESFilmicToneMapping;
      renderer.domElement.className = "absolute inset-0 size-full touch-none";
      renderer.domElement.setAttribute("aria-label", "可交互 3D 场景视口");
      renderer.domElement.setAttribute("role", "img");
      host.appendChild(renderer.domElement);
      const scene = new T.Scene(),
        content = new T.Group(),
        helpers = new T.Group();
      scene.add(content, helpers);
      const freeCamera = new T.PerspectiveCamera(50, 1, 0.01, 10000);
      freeCamera.position.set(6, 4, 8);
      const shotCamera = new T.PerspectiveCamera(50, 1, 0.01, 1000);
      const orbit = new OrbitControls(freeCamera, renderer.domElement);
      orbit.target.set(0, 1, 0);
      orbit.enableDamping = true;
      orbit.update();
      const control = new TransformControls(freeCamera, renderer.domElement);
      const gizmo = control.getHelper();
      helpers.add(gizmo);
      control.addEventListener("dragging-changed", (event) => {
        orbit.enabled = !event.value && !read().cameraView;
      });
      let draggingSelection: DirectorSelection | undefined;
      control.addEventListener("mouseDown", () => {
        draggingSelection = read().selection;
      });
      control.addEventListener("mouseUp", () => {
        const object = control.object;
        if (object && draggingSelection)
          transform(
            {
              position: object.position.toArray(),
              rotation: [
                object.rotation.x,
                object.rotation.y,
                object.rotation.z,
              ],
              scale: object.scale
                .toArray()
                .map((v) => Math.max(0.01, Math.min(100, v))),
            },
            draggingSelection,
          );
        draggingSelection = undefined;
      });
      const grid = new T.GridHelper(
        40,
        40,
        DIRECTOR_SCENE_COLORS.gridMajor,
        DIRECTOR_SCENE_COLORS.gridMinor,
      );
      helpers.add(grid);
      const groundMaterial = new T.MeshStandardMaterial({
        color: DIRECTOR_SCENE_COLORS.ground,
        roughness: 0.9,
        transparent: true,
      });
      const ground = new T.Mesh(new T.PlaneGeometry(200, 200), groundMaterial);
      ground.rotation.x = -Math.PI / 2;
      ground.receiveShadow = true;
      scene.add(ground);
      const environment = new T.HemisphereLight(
        DIRECTOR_SCENE_COLORS.light,
        DIRECTOR_SCENE_COLORS.environmentGround,
        0.5,
      );
      scene.add(environment);
      const selectionBox = new T.BoxHelper(
        new T.Object3D(),
        DIRECTOR_SCENE_COLORS.selectionHelper,
      );
      helpers.add(selectionBox);
      selectionBox.visible = false;
      const gltfLoader = new GLTFLoader(),
        textureLoader = new T.TextureLoader();
      const assetPromises = new Map<string, Promise<API.Preview>>();
      const modelPromises = new Map<
        string,
        Promise<Awaited<ReturnType<typeof gltfLoader.loadAsync>>>
      >();
      const texturePromises = new Map<string, Promise<THREE.Texture>>();
      const resources = new Set<
        THREE.BufferGeometry | THREE.Material | THREE.Texture
      >();
      type RecordEntry = {
        root: THREE.Group;
        signature: string;
        bones: Map<string, THREE.Object3D>;
        bind: Map<string, THREE.Quaternion>;
        mixer?: THREE.AnimationMixer;
        animations?: THREE.AnimationClip[];
        loading?: boolean;
        error?: string;
      };
      const records = new Map<string, RecordEntry>(),
        cameraHelpers = new Map<string, THREE.CameraHelper>(),
        lightRecords = new Map<
          string,
          { light: THREE.Light; marker: THREE.Object3D; type: string }
        >();
      const cameras = new Map<string, THREE.PerspectiveCamera>();
      let attachedID = "",
        capturing = false;
      function retain(object: THREE.Object3D) {
        object.traverse((node) => {
          if (!(node instanceof T.Mesh || node instanceof T.LineSegments))
            return;
          resources.add(node.geometry);
          const materials = Array.isArray(node.material)
            ? node.material
            : [node.material];
          for (const material of materials) {
            resources.add(material);
            for (const value of Object.values(material))
              if (value instanceof T.Texture) resources.add(value);
          }
        });
      }
      function getPreview(assetID: string) {
        if (!projectID)
          return Promise.reject(new Error("缺少项目范围，无法加载素材"));
        let promise = assetPromises.get(assetID);
        if (!promise) {
          promise = getMediaPreview(
            { pid: projectID, asset_id: assetID },
            { signal: controller.signal },
          ).then((r) => {
            if (
              r.asset?.id !== assetID ||
              r.asset.project_id !== projectID ||
              !r.url ||
              !/^https?:\/\//.test(r.url) ||
              Date.parse(r.expires_at ?? "") <= Date.now() ||
              !Number.isFinite(Date.parse(r.expires_at ?? ""))
            )
              throw new Error("素材预览授权无效");
            return r;
          });
          assetPromises.set(assetID, promise);
        }
        return promise;
      }
      function applyTransform(
        root: THREE.Object3D,
        value: API.DirectorTransform | undefined,
      ) {
        root.position.fromArray(value?.position ?? [0, 0, 0]);
        root.rotation.set(
          ...((value?.rotation ?? [0, 0, 0]) as [number, number, number]),
        );
        root.scale.fromArray(value?.scale ?? [1, 1, 1]);
      }
      function makeMannequin(record: RecordEntry, color: string) {
        const material = new T.MeshStandardMaterial({ color, roughness: 0.65 });
        function mesh(
          geometry: THREE.BufferGeometry,
          parent: THREE.Object3D,
          position: number[],
        ) {
          const part = new T.Mesh(geometry, material);
          part.position.fromArray(position);
          part.castShadow = true;
          part.receiveShadow = true;
          parent.add(part);
          return part;
        }
        function joint(
          name: string,
          parent: THREE.Object3D,
          position: number[],
        ) {
          const pivot = new T.Group();
          pivot.name = name;
          pivot.position.fromArray(position);
          parent.add(pivot);
          record.bones.set(name, pivot);
          record.bind.set(name, pivot.quaternion.clone());
          return pivot;
        }
        const hips = joint("hips", record.root, [0, 0.95, 0]);
        mesh(new T.SphereGeometry(0.18, 16, 12), hips, [0, 0, 0]);
        const spine = joint("spine", hips, [0, 0.18, 0]);
        mesh(new T.CapsuleGeometry(0.17, 0.2, 5, 12), spine, [0, 0.08, 0]);
        const chest = joint("chest", spine, [0, 0.23, 0]);
        mesh(new T.SphereGeometry(0.2, 16, 12), chest, [0, 0, 0]);
        const neck = joint("neck", chest, [0, 0.13, 0]);
        mesh(new T.CylinderGeometry(0.06, 0.07, 0.12, 12), neck, [0, 0.03, 0]);
        const head = joint("head", neck, [0, 0.18, 0]);
        mesh(new T.SphereGeometry(0.13, 20, 16), head, [0, 0, 0]);
        for (const side of ["left", "right"]) {
          const direction = side === "left" ? -1 : 1;
          const shoulder = joint(`${side}Shoulder`, chest, [
            direction * 0.16,
            0.02,
            0,
          ]);
          const arm = joint(`${side}UpperArm`, shoulder, [
            direction * 0.08,
            0,
            0,
          ]);
          mesh(new T.CapsuleGeometry(0.065, 0.18, 4, 10), arm, [0, -0.13, 0]);
          const elbow = joint(`${side}LowerArm`, arm, [0, -0.3, 0]);
          mesh(new T.CapsuleGeometry(0.05, 0.17, 4, 10), elbow, [0, -0.12, 0]);
          const hand = joint(`${side}Hand`, elbow, [0, -0.27, 0]);
          mesh(new T.SphereGeometry(0.065, 12, 10), hand, [0, -0.025, 0]);
          const leg = joint(`${side}UpperLeg`, hips, [
            direction * 0.11,
            -0.06,
            0,
          ]);
          mesh(new T.CapsuleGeometry(0.085, 0.25, 4, 10), leg, [0, -0.18, 0]);
          const knee = joint(`${side}LowerLeg`, leg, [0, -0.4, 0]);
          mesh(new T.CapsuleGeometry(0.065, 0.25, 4, 10), knee, [0, -0.18, 0]);
          const foot = joint(`${side}Foot`, knee, [0, -0.4, 0]);
          mesh(new T.BoxGeometry(0.13, 0.1, 0.25), foot, [0, -0.01, 0.065]);
        }
      }
      async function loadAsset(
        object: API.DirectorObject,
        record: RecordEntry,
      ) {
        const assetID = object.asset_id;
        if (!assetID) {
          record.error = "素材身份缺失";
          return;
        }
        record.loading = true;
        setStatus("正在加载场景素材…");
        try {
          const preview = await getPreview(assetID);
          if (disposed || records.get(object.id!) !== record) return;
          if (object.kind === "billboard") {
            if (preview.asset?.kind !== "image")
              throw new Error("只支持图片素材作为场景平面");
            let promise = texturePromises.get(assetID);
            if (!promise) {
              promise = textureLoader.loadAsync(preview.url!);
              texturePromises.set(assetID, promise);
            }
            const texture = await promise;
            resources.add(texture);
            texture.colorSpace = T.SRGBColorSpace;
            if (disposed || records.get(object.id!) !== record) {
              if (disposed) texture.dispose();
              return;
            }
            const image = texture.image as { width?: number; height?: number };
            const ratio = (image.width ?? 1) / (image.height ?? 1);
            const mesh = new T.Mesh(
              new T.PlaneGeometry(ratio * 2, 2),
              new T.MeshStandardMaterial({
                map: texture,
                side: T.DoubleSide,
                transparent: true,
                roughness: 0.8,
              }),
            );
            record.root.add(mesh);
            retain(mesh);
          } else {
            if (preview.asset?.kind !== "model")
              throw new Error("素材类型不是 GLB 模型");
            let promise = modelPromises.get(assetID);
            if (!promise) {
              promise = gltfLoader.loadAsync(preview.url!);
              modelPromises.set(assetID, promise);
            }
            const model = await promise;
            retain(model.scene);
            if (disposed || records.get(object.id!) !== record) {
              if (disposed) {
                for (const resource of resources) resource.dispose();
              }
              return;
            }
            const modelRoot = clone(model.scene);
            record.root.add(modelRoot);
            modelRoot.traverse((part) => {
              if (part instanceof T.Mesh) {
                part.castShadow = true;
                part.receiveShadow = true;
                part.material = Array.isArray(part.material)
                  ? part.material.map((m) => m.clone())
                  : part.material.clone();
              }
              const declared = Object.entries(object.rig?.bone_map ?? {}).find(
                ([, nodeName]) => nodeName === part.name,
              )?.[0];
              const simpleName = part.name.replace(/^mixamorig[:_]?/i, "");
              const name =
                declared ??
                boneNames.find(
                  (bone) => bone.toLowerCase() === simpleName.toLowerCase(),
                );
              if (name) {
                record.bones.set(name, part);
                record.bind.set(name, part.quaternion.clone());
              }
            });
            modelInfo(object.id!, {
              bone_map: Object.fromEntries(
                [...record.bones.entries()].map(([name, part]) => [
                  name,
                  part.name,
                ]),
              ),
              animation_names: model.animations
                .map((a) => a.name)
                .filter(
                  (name) => !!name.trim() && Array.from(name).length <= 128,
                )
                .slice(0, 128),
            });
            record.animations = model.animations;
            record.mixer = new T.AnimationMixer(modelRoot);
            retain(modelRoot);
          }
        } catch (error) {
          if (!disposed && !controller.signal.aborted) {
            record.error =
              error instanceof Error ? error.message : "素材加载失败";
            setFailure(`素材「${object.name}」加载失败：${record.error}`);
          }
        } finally {
          record.loading = false;
          if (!disposed)
            setStatus(
              [...records.values()].some((r) => r.loading)
                ? "正在加载场景素材…"
                : "",
            );
        }
      }
      function syncStructure(config: API.DirectorConfig) {
        const ids = new Set((config.objects ?? []).map((o) => o.id));
        for (const [id, record] of records)
          if (!ids.has(id)) {
            content.remove(record.root);
            record.mixer?.stopAllAction();
            records.delete(id);
          }
        for (const object of config.objects ?? []) {
          if (!object.id) continue;
          const signature = [
            object.kind,
            object.primitive,
            object.asset_id,
            object.builtin_actor,
          ].join(":");
          const existing = records.get(object.id);
          if (existing?.signature === signature) continue;
          if (existing) {
            content.remove(existing.root);
            existing.mixer?.stopAllAction();
          }
          const root = new T.Group();
          root.userData.director = { kind: "object", id: object.id };
          content.add(root);
          const record: RecordEntry = {
            root,
            signature,
            bones: new Map(),
            bind: new Map(),
          };
          records.set(object.id, record);
          if (
            object.builtin_actor === "mannequin" ||
            object.primitive === "character"
          )
            makeMannequin(record, object.color ?? DIRECTOR_SCENE_COLORS.object);
          else if (
            object.kind === "model" ||
            object.kind === "actor" ||
            object.kind === "billboard"
          )
            void loadAsset(object, record);
          else {
            const geometry =
              object.primitive === "sphere"
                ? new T.SphereGeometry(0.5, 32, 24)
                : object.primitive === "cylinder"
                  ? new T.CylinderGeometry(0.5, 0.5, 1, 32)
                  : object.primitive === "plane"
                    ? new T.PlaneGeometry(2, 2)
                    : new T.BoxGeometry(1, 1, 1);
            const material = new T.MeshStandardMaterial({
              color: object.color ?? DIRECTOR_SCENE_COLORS.object,
              roughness: 0.65,
              side: T.DoubleSide,
            });
            const mesh = new T.Mesh(geometry, material);
            if (object.primitive === "plane") mesh.rotation.x = -Math.PI / 2;
            root.add(mesh);
          }
          retain(root);
        }
        const cameraIDs = new Set((config.cameras ?? []).map((c) => c.id));
        for (const [id, camera] of cameras)
          if (!cameraIDs.has(id)) {
            const helper = cameraHelpers.get(id);
            if (helper) {
              helpers.remove(helper);
              helper.dispose();
            }
            cameras.delete(id);
            cameraHelpers.delete(id);
            scene.remove(camera);
          }
        for (const value of config.cameras ?? [])
          if (value.id && !cameras.has(value.id)) {
            const camera = new T.PerspectiveCamera();
            camera.userData.director = { kind: "camera", id: value.id };
            cameras.set(value.id, camera);
            scene.add(camera);
            const helper = new T.CameraHelper(camera);
            helper.userData.director = camera.userData.director;
            cameraHelpers.set(value.id, helper);
            helpers.add(helper);
          }
        const lightIDs = new Set((config.lights ?? []).map((l) => l.id));
        for (const [id, entry] of lightRecords)
          if (!lightIDs.has(id)) {
            scene.remove(entry.light);
            helpers.remove(entry.marker);
            if (
              entry.light instanceof T.DirectionalLight ||
              entry.light instanceof T.SpotLight
            )
              scene.remove(entry.light.target);
            entry.light.dispose();
            lightRecords.delete(id);
          }
        for (const value of config.lights ?? []) {
          if (!value.id) continue;
          const existing = lightRecords.get(value.id);
          if (existing?.type === value.type) continue;
          if (existing) {
            scene.remove(existing.light);
            helpers.remove(existing.marker);
            if (
              existing.light instanceof T.DirectionalLight ||
              existing.light instanceof T.SpotLight
            )
              scene.remove(existing.light.target);
            existing.light.dispose();
          }
          const light =
            value.type === "ambient"
              ? new T.AmbientLight()
              : value.type === "directional"
                ? new T.DirectionalLight()
                : value.type === "spot"
                  ? new T.SpotLight()
                  : new T.PointLight();
          light.userData.director = { kind: "light", id: value.id };
          scene.add(light);
          if (
            light instanceof T.DirectionalLight ||
            light instanceof T.SpotLight
          )
            scene.add(light.target);
          const marker = new T.Mesh(
            new T.SphereGeometry(0.1, 10, 8),
            new T.MeshBasicMaterial({
              color: value.color ?? DIRECTOR_SCENE_COLORS.light,
            }),
          );
          marker.userData.director = light.userData.director;
          helpers.add(marker);
          retain(marker);
          lightRecords.set(value.id, {
            light,
            marker,
            type: value.type ?? "point",
          });
        }
      }
      function poseMannequin(record: RecordEntry, pose: string, time: number) {
        for (const [bone, part] of record.bones)
          part.quaternion.copy(record.bind.get(bone)!);
        const articulatedPose = directorPose(pose, time);
        for (const [name, angles] of Object.entries(articulatedPose.joints))
          record.bones.get(name)?.rotation.set(...angles);
        const hips = record.bones.get("hips");
        if (hips) hips.position.y = articulatedPose.height;
      }
      function animateBones(
        record: RecordEntry,
        object: API.DirectorObject,
        time: number,
      ) {
        for (const [name, quaternion] of Object.entries(
          object.bone_overrides ?? {},
        ))
          if (Array.isArray(quaternion) && quaternion.length === 4)
            record.bones
              .get(name)
              ?.quaternion.fromArray(quaternion as number[]);
        for (const track of object.bone_tracks ?? []) {
          const bone = record.bones.get(track.bone ?? ""),
            frames = (track.keyframes ?? []).toSorted(
              (a, b) => a.time! - b.time!,
            );
          if (!bone || !frames.length) continue;
          bone.quaternion.fromArray(
            directorBoneAt(frames, time, bone.quaternion.toArray()),
          );
        }
      }
      function render() {
        if (disposed || capturing) return;
        const { config, selection, mode, time, cameraView } = read();
        syncStructure(config);
        scene.background = new T.Color(
          config.background ?? DIRECTOR_SCENE_COLORS.background,
        );
        environment.intensity = config.environment_intensity ?? 0.5;
        content.position.fromArray(
          config.stage_transform?.position ?? [0, 0, 0],
        );
        content.rotation.set(
          ...((config.stage_transform?.rotation ?? [0, 0, 0]) as [
            number,
            number,
            number,
          ]),
        );
        content.scale.setScalar(config.stage_transform?.scale ?? 1);
        grid.visible = !!config.grid_visible;
        ground.visible = config.ground?.visible !== false;
        ground.position.y = config.ground?.height ?? 0;
        groundMaterial.opacity = config.ground?.opacity ?? 1;
        for (const value of config.objects ?? []) {
          const record = records.get(value.id!);
          if (!record) continue;
          if (!control.dragging || control.object !== record.root)
            applyTransform(
              record.root,
              directorTransformAt(value.transform, value.keyframes, time),
            );
          record.root.visible = value.visible !== false;
          record.root.traverse((node) => {
            if (node instanceof T.Mesh) {
              node.castShadow = !!value.cast_shadow;
              node.receiveShadow = !!value.receive_shadow;
              if (value.kind === "primitive" || value.builtin_actor) {
                const materials = Array.isArray(node.material)
                  ? node.material
                  : [node.material];
                for (const material of materials)
                  if (material instanceof T.MeshStandardMaterial)
                    material.color.set(
                      value.color ?? DIRECTOR_SCENE_COLORS.object,
                    );
              }
            }
          });
          if (value.builtin_actor === "mannequin")
            poseMannequin(record, value.pose ?? "stand", time);
          if (record.mixer) {
            record.mixer.stopAllAction();
            for (const clip of value.motion_clips ?? []) {
              if (
                time < (clip.start ?? 0) ||
                time > (clip.start ?? 0) + (clip.duration ?? 0)
              )
                continue;
              const animation = record.animations?.find(
                (a) => a.name === clip.source_animation,
              );
              if (!animation) continue;
              const action = record.mixer.clipAction(animation);
              action.setLoop(
                clip.loop ? T.LoopRepeat : T.LoopOnce,
                clip.loop ? Infinity : 1,
              );
              action.clampWhenFinished = true;
              action.play();
              action.time =
                (time - (clip.start ?? 0)) * (clip.playback_rate ?? 1);
              if (clip.loop) action.time %= Math.max(0.001, animation.duration);
              record.mixer.update(0);
            }
          }
          animateBones(record, value, time);
        }
        content.updateMatrixWorld(true);
        const aspect = aspectRatio(
          config.aspect_ratio,
          host.clientWidth / Math.max(1, host.clientHeight),
        );
        for (const value of config.cameras ?? []) {
          const camera = cameras.get(value.id!);
          if (!camera) continue;
          if (!control.dragging || control.object !== camera)
            applyTransform(
              camera,
              directorTransformAt(value.transform, value.keyframes, time),
            );
          const cameraDragging = control.dragging && control.object === camera;
          const follow = records.get(value.follow_object_id ?? "");
          if (follow && !cameraDragging) {
            const position = follow.root.getWorldPosition(new T.Vector3());
            camera.position
              .add(position)
              .add(new T.Vector3().fromArray(value.follow_anchor ?? [0, 0, 0]));
          }
          const look = records.get(value.look_at_object_id ?? "");
          if (!cameraDragging && value.look_at_mode === "object" && look)
            camera.lookAt(look.root.getWorldPosition(new T.Vector3()));
          else if (!cameraDragging && value.look_at_mode !== "rotation")
            camera.lookAt(new T.Vector3().fromArray(value.target ?? [0, 1, 0]));
          camera.fov = value.fov ?? 50;
          camera.near = value.near ?? 0.01;
          camera.far = value.far ?? 1000;
          camera.aspect = aspect;
          camera.updateProjectionMatrix();
          camera.updateMatrixWorld();
          cameraHelpers.get(value.id!)?.update();
        }
        for (const value of config.lights ?? []) {
          const entry = lightRecords.get(value.id!);
          if (!entry) continue;
          if (!control.dragging || control.object !== entry.light)
            applyTransform(entry.light, value.transform);
          entry.light.color.set(value.color ?? DIRECTOR_SCENE_COLORS.light);
          entry.light.intensity = value.intensity ?? 1;
          entry.light.castShadow = !!value.cast_shadow;
          if (entry.light instanceof T.SpotLight) {
            entry.light.angle = value.angle ?? Math.PI / 4;
            entry.light.penumbra = value.penumbra ?? 0.3;
          }
          if (
            entry.light instanceof T.DirectionalLight ||
            entry.light instanceof T.SpotLight
          ) {
            const direction = new T.Vector3(0, 0, -1).applyQuaternion(
              entry.light.quaternion,
            );
            entry.light.target.position
              .copy(entry.light.position)
              .add(direction);
          }
          entry.marker.position.copy(entry.light.position);
        }
        const selected =
          selection?.kind === "object"
            ? records.get(selection.id)?.root
            : selection?.kind === "camera"
              ? cameras.get(selection.id)
              : lightRecords.get(selection?.id ?? "")?.light;
        const id = selected?.uuid ?? "";
        if (attachedID !== id) {
          control.detach();
          if (selected) control.attach(selected);
          attachedID = id;
        }
        control.setMode(mode);
        control.setTranslationSnap(config.grid_snap ? 0.5 : null);
        control.setRotationSnap(config.grid_snap ? Math.PI / 12 : null);
        control.setScaleSnap(config.grid_snap ? 0.1 : null);
        helpers.visible = !cameraView;
        control.enabled = !!selected && !cameraView;
        orbit.enabled = !cameraView && !control.dragging;
        orbit.update();
        if (selected && selection?.kind === "object") {
          selectionBox.setFromObject(selected);
          selectionBox.visible = true;
        } else selectionBox.visible = false;
        const shot =
          config.shots?.find((s) => s.id === config.active_shot_id) ??
          config.shots?.[0];
        const activeCamera = cameras.get(shot?.camera_id ?? "");
        if (activeCamera) {
          shotCamera.copy(activeCamera, false);
          shotCamera.updateProjectionMatrix();
        }
        renderer.setScissorTest(false);
        renderer.setViewport(0, 0, host.clientWidth, host.clientHeight);
        renderer.clear();
        if (cameraView) {
          const width = host.clientWidth,
            height = host.clientHeight,
            viewWidth = Math.min(width, height * aspect),
            viewHeight = viewWidth / aspect;
          renderer.setScissorTest(true);
          renderer.setScissor(
            (width - viewWidth) / 2,
            (height - viewHeight) / 2,
            viewWidth,
            viewHeight,
          );
          renderer.setViewport(
            (width - viewWidth) / 2,
            (height - viewHeight) / 2,
            viewWidth,
            viewHeight,
          );
          renderer.render(scene, shotCamera);
          renderer.setScissorTest(false);
        } else renderer.render(scene, freeCamera);
      }
      function resize() {
        if (disposed) return;
        const width = Math.max(1, host.clientWidth),
          height = Math.max(1, host.clientHeight);
        renderer.setSize(width, height, false);
        freeCamera.aspect = width / height;
        freeCamera.updateProjectionMatrix();
      }
      const observer = new ResizeObserver(resize);
      observer.observe(host);
      resize();
      const raycaster = new T.Raycaster(),
        pointer = new T.Vector2();
      let pointerStart = { x: 0, y: 0 };
      function down(event: PointerEvent) {
        pointerStart = { x: event.clientX, y: event.clientY };
      }
      function up(event: PointerEvent) {
        if (
          event.button !== 0 ||
          read().cameraView ||
          control.dragging ||
          !!control.axis ||
          Math.hypot(
            event.clientX - pointerStart.x,
            event.clientY - pointerStart.y,
          ) > 4
        )
          return;
        const rect = renderer.domElement.getBoundingClientRect();
        pointer.set(
          ((event.clientX - rect.left) / rect.width) * 2 - 1,
          (-(event.clientY - rect.top) / rect.height) * 2 + 1,
        );
        raycaster.setFromCamera(pointer, freeCamera);
        const targets = [...records.values()]
          .map((r) => r.root)
          .concat(
            [...lightRecords.values()].map(
              (entry) => entry.marker,
            ) as THREE.Group[],
          );
        const hits = raycaster.intersectObjects(
          [...targets, ...cameraHelpers.values()],
          true,
        );
        for (const hit of hits) {
          let node: THREE.Object3D | null = hit.object;
          while (node && !node.userData.director) node = node.parent;
          if (node?.userData.director) {
            select(node.userData.director as DirectorSelection);
            return;
          }
        }
        if (!control.axis) select(undefined);
      }
      function lost(event: Event) {
        event.preventDefault();
        setFailure(
          "WebGL 上下文已丢失，请关闭并重新打开导演台。配置修改仍保留在画布中。",
        );
      }
      renderer.domElement.addEventListener("pointerdown", down);
      renderer.domElement.addEventListener("pointerup", up);
      renderer.domElement.addEventListener("webglcontextlost", lost);
      runtime.current = {
        home() {
          freeCamera.position.set(6, 4, 8);
          orbit.target.set(0, 1, 0);
          orbit.update();
        },
        useView() {
          return {
            transform: {
              position: freeCamera.position.toArray(),
              rotation: [
                freeCamera.rotation.x,
                freeCamera.rotation.y,
                freeCamera.rotation.z,
              ],
              scale: [1, 1, 1],
            },
            target: orbit.target.toArray(),
            fov: freeCamera.fov,
          };
        },
        async capture() {
          render();
          if ([...records.values()].some((r) => r.loading || r.error))
            throw new Error("场景素材尚未加载成功");
          capturing = true;
          const ratio = aspectRatio(read().config.aspect_ratio, 16 / 9),
            width = ratio >= 1 ? 1920 : Math.round(1920 * ratio),
            height = Math.round(width / ratio);
          helpers.visible = false;
          renderer.setPixelRatio(1);
          renderer.setSize(width, height, false);
          renderer.setScissorTest(false);
          renderer.setViewport(0, 0, width, height);
          shotCamera.aspect = ratio;
          shotCamera.updateProjectionMatrix();
          renderer.render(scene, shotCamera);
          try {
            return await new Promise<Blob>((resolve, reject) =>
              renderer.domElement.toBlob((blob) => {
                if (blob) resolve(blob);
                else reject(new Error("PNG 编码失败"));
              }, "image/png"),
            );
          } finally {
            if (!disposed) {
              renderer.setPixelRatio(Math.min(devicePixelRatio, 2));
              resize();
              capturing = false;
              render();
            }
          }
        },
      };
      renderer.setAnimationLoop(render);
      destroy = () => {
        renderer.setAnimationLoop(null);
        observer.disconnect();
        control.detach();
        control.dispose();
        orbit.dispose();
        renderer.domElement.removeEventListener("pointerdown", down);
        renderer.domElement.removeEventListener("pointerup", up);
        renderer.domElement.removeEventListener("webglcontextlost", lost);
        for (const record of records.values()) record.mixer?.stopAllAction();
        for (const helper of cameraHelpers.values()) helper.dispose();
        for (const entry of lightRecords.values()) entry.light.dispose();
        retain(scene);
        for (const resource of resources) resource.dispose();
        renderer.dispose();
        renderer.forceContextLoss();
        renderer.domElement.remove();
        runtime.current = null;
      };
      setStatus("");
    }
    void mount().catch(() => {
      if (!disposed) {
        destroy?.();
        setStatus("");
        setFailure(
          "无法初始化真实 3D 视口。请启用浏览器硬件加速或使用支持 WebGL 2 的浏览器。场景参数仍可编辑保存。",
        );
      }
    });
    return () => {
      disposed = true;
      controller.abort();
      destroy?.();
    };
  }, [projectID, attempt]);
  return (
    <div
      ref={container}
      className="relative min-h-[300px] flex-1 overflow-hidden"
    >
      {status && (
        <p
          role="status"
          className="pointer-events-none absolute top-3 left-3 rounded-md bg-background/90 px-3 py-2 text-xs"
        >
          {status}
        </p>
      )}
      {failure && (
        <p
          role="alert"
          className="absolute top-3 right-3 left-3 rounded-md border bg-background/95 px-3 py-2 text-sm text-destructive"
        >
          {failure}{" "}
          <Button
            size="sm"
            variant="outline"
            onClick={() => {
              setFailure("");
              setStatus("正在重新初始化场景…");
              setAttempt((n) => n + 1);
            }}
          >
            重新加载视口
          </Button>
        </p>
      )}
    </div>
  );
}
function aspectRatio(value: string | undefined, fallback: number) {
  if (!value || value === "adaptive") return fallback;
  const [w, h] = value.split(":").map(Number);
  return w > 0 && h > 0 ? w / h : fallback;
}
