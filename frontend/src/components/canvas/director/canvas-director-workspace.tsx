"use client";
import {
  useCallback,
  useEffect,
  useMemo,
  useRef,
  useState,
  useSyncExternalStore,
} from "react";
import dynamic from "next/dynamic";
import {
  CircleHelp,
  Camera,
  Lock,
  Unlock,
  Eye,
  EyeOff,
  Move,
  RotateCw,
  Scaling,
  MousePointer2,
  Plus,
  Trash2,
  Play,
  Pause,
  X,
  Maximize2,
  Focus,
  Save,
  Undo2,
  Redo2,
  Download,
  FolderOpen,
  Crosshair,
  Columns2,
  PanelLeft,
  PanelRight,
} from "lucide-react";
import { Button } from "@/components/ui/button";
import { Input } from "@/components/ui/input";
import { Label } from "@/components/ui/label";
import { Slider } from "@/components/ui/slider";
import { Tabs, TabsList, TabsTrigger } from "@/components/ui/tabs";
import {
  Select,
  SelectContent,
  SelectItem,
  SelectTrigger,
  SelectValue,
} from "@/components/ui/select";
import {
  Dialog,
  DialogContent,
  DialogTitle,
  DialogDescription,
} from "@/components/ui/dialog";
import type { CanvasYjsLiveSession } from "../yjs-live-session";
import {
  DIRECTOR_COLORS,
  directorEntity,
  directorFingerprint,
  directorTransform,
  editDirector,
  type Director,
  type DirectorEdit,
  type DirectorSelection,
  type DirectorTransform,
  type DirectorObject,
} from "./director-model";
import {
  activeDirectorScene,
  migrateDirectorDocument,
  replaceDirectorScene,
  switchDirectorWorkspace,
  type DirectorMode,
  type DirectorPoses,
  type DirectorNodePoses,
} from "./director-workspaces";
import {
  sampleDirectorTransform,
  sampleDirectorCamera,
  keyDirectorCamera,
} from "./director-animation";
import {
  applyDirectorEnvironment,
  DIRECTOR_ENVIRONMENTS,
} from "./director-environment";
import {
  directorHelpSeen,
  markDirectorHelpSeen,
} from "./director-view-preferences";
import { useIsMobile } from "@/hooks/use-mobile";
import { getMediaPreview } from "@/api/media";
import { DirectorAssetLibrary } from "./director-asset-library";
import { saveAs } from "file-saver";
import {
  useDirectorRecovery,
  parseDirectorRecoveryFile,
} from "./director-recovery";
import {
  directorLightAngles,
  directorLightRotation,
} from "./director-light-angles";
import { DirectorBodyEditor } from "./director-body-editor";
import { DirectorImportDialog } from "./director-import-dialog";
import type { DirectorTool } from "./director-scene";
import { DirectorCapture, DirectorGallery } from "./director-capture";
import {
  DIRECTOR_CAMERA_PRESETS,
  directorCameraPreset,
} from "./director-cameras";
import { DirectorTimeline } from "./director-timeline";
import { DirectorHistory } from "./director-history";
import { DirectorVideoDialog } from "./director-video-dialog";
import type { DirectorVideoExporter } from "./director-video-output";
import { DirectorPathEditor } from "./director-path-editor";
import {
  DirectorMotionLibrary,
  type DirectorMotionPreview,
} from "./director-motion-library";
import { DirectorPoseEditor } from "./director-pose-editor";
import { DirectorMotionEditor } from "./director-motion-editor";
import type { DirectorModelInfo } from "./director-motion";
import { DirectorViewport } from "./director-viewport";
import { createDirectorActor } from "./director-assets";
import "./director.css";
const ProjectAssetDialog = dynamic(
  () =>
    import("@/components/media/project-asset-dialog").then(
      (m) => m.ProjectAssetDialog,
    ),
  { loading: () => <div role="status">正在打开素材库…</div> },
);
type Props = {
  client: CanvasYjsLiveSession;
  nodeID: string;
  canvasID: string;
  projectID: string;
  compact?: boolean;
  onExpand?: () => void;
  onClose?: () => void;
};
function NumberField({
  label,
  value,
  min,
  max,
  step = 0.1,
  onChange,
  disabled = false,
}: {
  label: string;
  value: number;
  min?: number;
  max?: number;
  step?: number;
  onChange: (v: number) => void;
  disabled?: boolean;
}) {
  const [editing, setEditing] = useState(false),
    [draft, setDraft] = useState("");
  const commit = useRef<((v: number) => void) | null>(null);
  return (
    <Label className="director-field">
      {label}
      <Input
        aria-label={label}
        type="number"
        step={step}
        min={min}
        max={max}
        value={editing ? draft : value}
        disabled={disabled}
        onFocus={() => {
          commit.current = onChange;
          setDraft(String(value));
          setEditing(true);
        }}
        onChange={(e) => setDraft(e.target.value)}
        onBlur={() => {
          setEditing(false);
          const value = Number(draft);
          if (commit.current && draft.trim() && Number.isFinite(value))
            commit.current(value);
          commit.current = null;
        }}
        onKeyDown={(e) => {
          if (e.key === "Enter") e.currentTarget.blur();
          if (e.key === "Escape") {
            commit.current = null;
            e.currentTarget.blur();
          }
        }}
      />
    </Label>
  );
}
function Choice({
  label,
  value,
  items,
  onChange,
  disabled = false,
}: {
  label: string;
  value: string;
  items: [string, string][];
  onChange: (value: string) => void;
  disabled?: boolean;
}) {
  return (
    <Label className="director-field">
      {label}
      <Select value={value} onValueChange={onChange} disabled={disabled}>
        <SelectTrigger aria-label={label}>
          <SelectValue />
        </SelectTrigger>
        <SelectContent
          data-canvas-no-zoom
          onMouseDown={(e) => e.stopPropagation()}
        >
          {items.map(([v, title]) => (
            <SelectItem key={v} value={v}>
              {title}
            </SelectItem>
          ))}
        </SelectContent>
      </Select>
    </Label>
  );
}
export function CanvasDirectorWorkspace(props: Props) {
  const [expanded, setExpanded] = useState(false),
    key = `${props.projectID}/${props.canvasID}/${props.nodeID}`;
  if (!props.compact) return <DirectorWorkspace key={key} {...props} />;
  return (
    <>
      <DirectorCard
        key={key}
        {...props}
        onExpand={() => (props.onExpand ? props.onExpand() : setExpanded(true))}
      />
      <Dialog open={expanded} onOpenChange={setExpanded}>
        <DialogContent
          data-canvas-no-zoom
          onMouseDown={(e) => e.stopPropagation()}
          className="canvas-director-dialog top-0 left-0 max-h-none translate-x-0 translate-y-0"
          showCloseButton={false}
        >
          <DialogTitle className="sr-only">导演台</DialogTitle>
          <DialogDescription className="sr-only">
            三维场景、机位与动态预演
          </DialogDescription>
          <DirectorWorkspace
            key={key}
            {...props}
            compact={false}
            onClose={() => setExpanded(false)}
          />
        </DialogContent>
      </Dialog>
    </>
  );
}
function DirectorCard({ client, nodeID, onExpand }: Props) {
  const view = useSyncExternalStore(
    client.subscribe,
    client.getSnapshot,
    client.getSnapshot,
  );
  const raw = view.document?.nodes.find((node) => node.id === nodeID)
    ?.serviceData?.config?.director;
  const parsed = useMemo(() => {
    try {
      const document = migrateDirectorDocument(raw);
      return { scene: activeDirectorScene(document), document, error: "" };
    } catch (cause) {
      return {
        scene: null,
        error: cause instanceof Error ? cause.message : "场景无法读取",
      };
    }
  }, [raw]);
  const scene = parsed.scene;
  return (
    <section
      className="canvas-director director-card"
      aria-label="导演台节点"
      data-canvas-shortcuts-ignore
      data-canvas-no-zoom
      onMouseDown={(event) => event.stopPropagation()}
      onPointerDown={(event) => event.stopPropagation()}
      onDoubleClick={(event) => event.stopPropagation()}
    >
      <Camera className="director-card-icon" aria-hidden />
      <strong>3D导演台</strong>
      <p>
        {scene?.objects.length
          ? `${scene.objects.length} 个物体 · ${scene.cameras.length} 个机位 · ${scene.shots.length} 个镜头`
          : "搭建三维场景，自由调整机位与灯光"}
      </p>
      <p>{scene?.mode === "dynamic" ? "动态预演" : "静态预演"}</p>
      {parsed.error && <p role="alert">{parsed.error}</p>}
      <Button size="sm" onClick={onExpand} disabled={!scene}>
        <Maximize2 />
        进入导演台
      </Button>
    </section>
  );
}
function DirectorWorkspace({
  client,
  nodeID,
  canvasID,
  projectID,
  compact = false,
  onExpand,
  onClose,
}: Props) {
  const view = useSyncExternalStore(
      client.subscribe,
      client.getSnapshot,
      client.getSnapshot,
    ),
    raw = view.document?.nodes.find((n) => n.id === nodeID)?.serviceData?.config
      ?.director;
  const parsed = useMemo(() => {
    try {
      const document = migrateDirectorDocument(raw);
      return { scene: activeDirectorScene(document), document, error: "" };
    } catch (error) {
      return {
        scene: null,
        error: error instanceof Error ? error.message : "导演场景无法读取",
      };
    }
  }, [raw]);
  const mobile = useIsMobile();
  const [assetsExpanded, setAssetsExpanded] = useState<boolean | null>(null);
  const [propertiesExpanded, setPropertiesExpanded] = useState<boolean | null>(
    null,
  );
  const showAssets = assetsExpanded ?? !mobile;
  const showProperties = propertiesExpanded ?? !mobile;
  const [history] = useState(() => new DirectorHistory());
  const [historyVersion, setHistoryVersion] = useState(0);
  void historyVersion;
  const [exportBusy, setExportBusy] = useState(false);
  const [videoExporter, setVideoExporter] =
    useState<DirectorVideoExporter | null>(null);
  const recovery = useDirectorRecovery(client, canvasID, nodeID);
  const { retained, setRetained } = recovery;
  const [recoverConfirm, setRecoverConfirm] = useState(false);
  const [exitConfirm, setExitConfirm] = useState(false);
  const saveGeneration = useRef(0);
  useEffect(
    () => () => {
      saveGeneration.current++;
    },
    [client],
  );
  useEffect(() => {
    if (!retained && !view.dirty) return;
    const preventLoss = (event: BeforeUnloadEvent) => {
      event.preventDefault();
      event.returnValue = "";
    };
    window.addEventListener("beforeunload", preventLoss);
    return () => window.removeEventListener("beforeunload", preventLoss);
  }, [retained, view.dirty]);
  const scene = parsed.scene,
    readOnly = !view.writable || view.migration || exportBusy;
  const [error, setError] = useState(""),
    [selected, setSelected] = useState<DirectorSelection | null>(null),
    [targetCameraID, setTargetCameraID] = useState<string | null>(null),
    [tool, setTool] = useState<DirectorTool>("translate"),
    [time, setTime] = useState(0),
    [playing, setPlaying] = useState(false),
    [loop, setLoop] = useState(true),
    [focus, setFocus] = useState(0),
    [tab, setTab] = useState("actors"),
    [picker, setPicker] = useState<{
      kind: "model" | "image";
      objectKind: "actor" | "model" | "billboard" | "panorama";
      epoch: string | null;
      mode: DirectorMode;
      projectID: string;
    } | null>(null);
  const [modelInfo, setModelInfo] = useState<Record<string, DirectorModelInfo>>(
    {},
  );
  const [capture, setCapture] = useState<(() => Promise<Blob>) | null>(null);
  const [placement, setPlacement] = useState<{
    object: DirectorObject;
    document: API.DirectorConfig;
    epoch: string | null;
    scene: Director;
    projectID: string;
    remaining?: DirectorObject[];
  } | null>(null);
  const placedObjectIDs = useRef(new Set<string>());
  const [path, setPath] = useState<{
    scene: Director;
    selection: DirectorSelection;
    document: API.DirectorConfig;
    epoch: string | null;
  } | null>(null);
  const [previewScene, setPreviewScene] = useState<Director | null>(null);
  const [motionPreview, setMotionPreview] =
    useState<DirectorMotionPreview>(null);
  const [assetRefresh, setAssetRefresh] = useState(0);
  const previewObject = motionPreview?.object;
  const motionPreviewScene = useMemo(
    () =>
      scene && previewObject
        ? {
            ...scene,
            objects: scene.objects.map((object) =>
              object.id === previewObject.id ? previewObject : object,
            ),
          }
        : scene,
    [scene, previewObject],
  );
  const [views, setViews] = useState({
    main: "director",
    preview: "active",
    visible: true,
    width: 320,
    names: true,
    paths: true,
    height: false,
    split: false,
  });
  const [help, setHelp] = useState(false);
  useEffect(() => {
    if (compact) return;
    if (directorHelpSeen()) return;
    const timer = window.setTimeout(() => setHelp(true), 0);
    return () => window.clearTimeout(timer);
  }, [compact]);
  function closeHelp() {
    setHelp(false);
    markDirectorHelpSeen();
  }
  const [assetSearch, setAssetSearch] = useState("");
  const [canvasBackground, setCanvasBackground] = useState<{
    kind: "billboard" | "panorama";
    epoch: string | null;
    mode: DirectorMode;
    projectID: string;
  } | null>(null);
  const [localImport, setLocalImport] = useState<{
    epoch: string | null;
    mode: DirectorMode;
    kind: "actor" | "model";
    objectID: string;
    projectID: string;
  } | null>(null);
  const [switchMode, setSwitchMode] = useState<DirectorMode | null>(null);
  const directorView = useRef<
    (() => { position: number[]; target: number[]; fov: number }) | null
  >(null);
  const groundSelection = useRef<
    ((selection: DirectorSelection) => DirectorTransform | null) | null
  >(null);
  const poses = useRef<(() => DirectorPoses) | null>(null);
  const nodePoses = useRef<(() => DirectorNodePoses) | null>(null);
  const gesture = useRef<{
    scene: Director;
    document: API.DirectorConfig;
    epoch: string | null;
  } | null>(null);
  const placementScene = useMemo(
    () =>
      placement
        ? {
            ...placement.scene,
            objects: [...placement.scene.objects, placement.object],
          }
        : null,
    [placement],
  );
  const shot = scene?.shots.find((s) => s.id === scene.active_shot_id),
    duration = shot?.duration ?? 20,
    fps = shot?.fps ?? 30,
    currentTime = Math.min(time, duration);
  const playhead = useRef(0);
  useEffect(() => {
    playhead.current = time;
  }, [time]);
  useEffect(() => {
    if (!playing || scene?.mode !== "dynamic") return;
    let frame = 0,
      previous = performance.now();
    const tick = (now: number) => {
      const elapsed = (now - previous) / 1000;
      if (elapsed >= 1 / fps) {
        previous = now;
        const next = playhead.current + elapsed;
        playhead.current =
          next >= duration ? (loop ? next % duration : duration) : next;
        setTime(playhead.current);
        if (next >= duration && !loop) {
          setPlaying(false);
          return;
        }
      }
      frame = requestAnimationFrame(tick);
    };
    frame = requestAnimationFrame(tick);
    return () => cancelAnimationFrame(frame);
  }, [playing, scene?.mode, duration, fps, loop]);
  const onError = useCallback((message: string) => {
    setError(message);
    setPlaying(false);
  }, []);
  function saveDocument(
    next: API.DirectorConfig,
    baseline = raw,
    epoch = client.getDocumentEpoch(),
    record = true,
  ) {
    if (!baseline) return;
    const generation = ++saveGeneration.current;
    const restoresRetained =
      retained && directorFingerprint(next) === directorFingerprint(retained);
    try {
      client.changeDirector(nodeID, next, baseline, epoch);
      if (record && epoch)
        history.record(
          migrateDirectorDocument(baseline),
          migrateDirectorDocument(next),
          epoch,
        );
      setHistoryVersion((v) => v + 1);
      void client
        .waitForSaved(() => {
          if (client.getDocumentEpoch() !== epoch)
            throw new Error("画布版本已变化");
        })
        .then(async (confirmed) => {
          if (saveGeneration.current !== generation) return;
          if (
            directorFingerprint(
              confirmed.nodes?.find((node) => node.id === nodeID)?.config
                ?.director,
            ) !== directorFingerprint(next)
          )
            throw new Error("已确认场景与本次编辑不同，编辑已保留");
          if (restoresRetained) await recovery.discard();
          setError("");
        })
        .catch((cause) => {
          if (saveGeneration.current !== generation) return;
          recovery.retain(next, baseline, epoch);
          setError(cause instanceof Error ? cause.message : "场景保存未确认");
        });
      setError("");
      return true;
    } catch (cause) {
      recovery.retain(next, baseline, epoch);
      setError(cause instanceof Error ? cause.message : "场景未保存");
      return false;
    }
  }
  function save(
    next: Director,
    baseline = raw,
    epoch = client.getDocumentEpoch(),
  ) {
    if (!baseline) return;
    try {
      return saveDocument(
        replaceDirectorScene(migrateDirectorDocument(baseline), next),
        baseline,
        epoch,
      );
    } catch (cause) {
      setError(cause instanceof Error ? cause.message : "场景无效");
      return false;
    }
  }
  function importBaseline(scope: {
    epoch: string | null;
    mode: DirectorMode;
    projectID: string;
  }) {
    const current = client.getSnapshot();
    const config = current.document?.nodes.find((node) => node.id === nodeID)
      ?.serviceData?.config?.director;
    if (
      !scope.epoch ||
      scope.epoch !== client.getDocumentEpoch() ||
      scope.projectID !== projectID ||
      readOnly ||
      !current.writable ||
      current.migration ||
      !config ||
      migrateDirectorDocument(config).mode !== scope.mode
    )
      throw new Error("画布版本、工作区或权限已变化，请重新选择素材");
    return config;
  }
  function applyHistory(action: "undo" | "redo") {
    if (!raw || readOnly) return;
    try {
      const epoch = client.getDocumentEpoch();
      if (!epoch) throw new Error("画布当前不可编辑");
      const change = history.prepare(
        migrateDirectorDocument(raw),
        epoch,
        action,
      );
      if (saveDocument(change.next, raw, epoch, false)) {
        change.accept();
        setSelected(null);
        setPlaying(false);
      }
    } catch (cause) {
      setError(cause instanceof Error ? cause.message : "历史操作失败");
    }
  }
  function changeMode(synchronize: boolean) {
    if (!parsed.document || !switchMode || readOnly) return;
    try {
      const next = switchDirectorWorkspace(
        parsed.document,
        switchMode,
        synchronize,
        currentTime,
        undefined,
        synchronize && switchMode === "static"
          ? nodePoses.current?.()
          : undefined,
      );
      if (saveDocument(next)) {
        setSwitchMode(null);
        setPlacement(null);
        setSelected(null);
        setTime(0);
      }
    } catch (cause) {
      setError(cause instanceof Error ? cause.message : "工作区切换失败");
    }
  }
  function edit(action: DirectorEdit) {
    if (!scene || readOnly) return;
    try {
      save(editDirector(scene, action));
    } catch (cause) {
      setError(cause instanceof Error ? cause.message : "操作失败");
    }
  }
  if (!scene)
    return (
      <p role="alert" className="p-4">
        {parsed.error}
      </p>
    );
  const entity = directorEntity(scene, selected),
    camera =
      selected?.kind === "camera"
        ? scene.cameras.find((c) => c.id === selected.id)
        : null,
    light =
      selected?.kind === "light"
        ? scene.lights.find((l) => l.id === selected.id)
        : null,
    object =
      selected?.kind === "object"
        ? scene.objects.find((o) => o.id === selected.id)
        : null;
  const cameraView =
    camera && scene.mode === "dynamic"
      ? sampleDirectorCamera(camera, currentTime)
      : camera;
  function cameraOptics(patch: {
    fov?: number;
    target?: number[];
    roll?: number;
  }) {
    if (!camera || !selected || !scene) return;
    edit({
      type: "entity",
      selection: selected,
      patch:
        scene.mode === "dynamic"
          ? keyDirectorCamera(
              camera,
              Math.round(currentTime * fps) / fps,
              patch,
            )
          : patch,
    });
  }
  const animated = object ?? camera,
    locked = readOnly || !!entity?.locked || !!motionPreview;
  const evaluated =
    animated && scene.mode === "dynamic"
      ? sampleDirectorTransform(
          animated.transform,
          animated.keyframes,
          currentTime,
        )
      : entity?.transform;
  function transform(
    value: DirectorTransform,
    selection = selected,
    baseline = scene,
    epoch = client.getDocumentEpoch(),
    baselineDocument = raw,
    channel: "position" | "rotation" | "scale" = tool === "rotate"
      ? "rotation"
      : tool === "scale"
        ? "scale"
        : "position",
  ) {
    if (!selection || !baseline) return;
    try {
      let next = editDirector(
        baseline,
        baseline.mode === "dynamic" && selection.kind !== "light"
          ? {
              type: "keyframe",
              channel,
              selection,
              time: Math.round(currentTime * fps) / fps,
              transform: value,
            }
          : {
              type: "entity",
              selection,
              patch: {
                transform: value,
                ...(selection.kind === "camera" && tool === "rotate"
                  ? { look_at_mode: "rotation" }
                  : {}),
              },
            },
      );
      if (
        selection.kind === "camera" &&
        tool === "rotate" &&
        baseline.mode === "dynamic"
      )
        next = editDirector(next, {
          type: "entity",
          selection,
          patch: { look_at_mode: "rotation" },
        });
      save(next, baselineDocument, epoch);
    } catch (cause) {
      setError(cause instanceof Error ? cause.message : "变换无效");
    }
  }
  const select = (value: DirectorSelection | null) => {
    setSelected(value);
    if (mobile && value) {
      setPropertiesExpanded(true);
      setAssetsExpanded(false);
    }
    setMotionPreview(null);
    setTargetCameraID(null);
    setPlaying(false);
  };
  const create = (action: DirectorEdit) => {
    try {
      const next = editDirector(scene, action);
      if (action.type === "add-object" && raw) {
        setPlacement({
          object: next.objects.at(-1)!,
          document: raw,
          epoch: client.getDocumentEpoch(),
          scene,
          projectID,
        });
        setPlaying(false);
        return;
      }
      save(next);
      if (action.type === "add-object")
        select({ kind: "object", id: next.objects.at(-1)!.id });
      if (action.type === "add-camera")
        select({ kind: "camera", id: next.cameras.at(-1)!.id });
      if (action.type === "add-light")
        select({ kind: "light", id: next.lights.at(-1)!.id });
    } catch (cause) {
      setError(cause instanceof Error ? cause.message : "添加失败");
    }
  };
  function placeProjectAssets(
    assets: API.AssetSummary[],
    scope: NonNullable<typeof picker>,
  ) {
    const baseline = importBaseline(scope);
    const next = activeDirectorScene(migrateDirectorDocument(baseline));
    if (!assets.length) throw new Error("请选择素材");
    if (scope.objectKind === "panorama" && assets.length !== 1)
      throw new Error("请选择一张全景图片");
    for (const asset of assets) {
      if (
        asset.project_id !== scope.projectID ||
        asset.kind !== scope.kind ||
        !asset.id
      )
        throw new Error("素材归属或类型不匹配");
    }
    if (scope.objectKind === "panorama") {
      next.panorama = {
        asset_id: assets[0].id,
        name: assets[0].file_name,
        rotation: 0,
      };
      if (!save(next, baseline, scope.epoch))
        throw new Error("全景背景未保存，请重试");
    } else {
      const kind = scope.objectKind;
      const objects: DirectorObject[] = assets.map((asset) => ({
        id: crypto.randomUUID(),
        name: asset.file_name?.slice(0, 128) || "素材",
        kind,
        asset_id: asset.id,
        transform: directorTransform(),
        color: DIRECTOR_COLORS.white,
        visible: true,
        cast_shadow: true,
        receive_shadow: true,
        keyframes: [],
      }));
      setPlacement({
        object: objects[0],
        remaining: objects.slice(1),
        scene: next,
        document: baseline,
        epoch: scope.epoch,
        projectID: scope.projectID,
      });
      setPlaying(false);
    }
  }
  const openAssetPicker = (
    kind: "model" | "image",
    objectKind: "actor" | "model" | "billboard" | "panorama",
  ) => {
    setPicker({
      kind,
      objectKind,
      epoch: client.getDocumentEpoch(),
      mode: scene.mode ?? "static",
      projectID,
    });
  };
  return (
    <section
      className={`canvas-director ${compact ? "director-compact" : ""}`}
      aria-label="导演台工作区"
      onKeyDown={(event) => {
        if (
          event.nativeEvent.isComposing ||
          event.ctrlKey ||
          event.metaKey ||
          event.altKey ||
          (event.target as HTMLElement).closest(
            "input,textarea,button,[contenteditable=true],[role=slider],[role=combobox],[role=menu]",
          )
        )
          return;
        if (event.key === "Escape" && placement) {
          setPlacement(null);
          event.preventDefault();
          event.stopPropagation();
          return;
        }
        if (
          event.key.toLowerCase() === "g" &&
          selected &&
          !locked &&
          evaluated
        ) {
          const grounded = groundSelection.current?.(selected);
          if (grounded) transform(grounded);
          event.preventDefault();
          return;
        }
        const modes = {
          "0": "select",
          "1": "translate",
          "2": "rotate",
          "3": "scale",
        } as const;
        if (event.key in modes) {
          setTool(modes[event.key as keyof typeof modes]);
          event.preventDefault();
        }
      }}
      data-canvas-shortcuts-ignore
      data-canvas-no-zoom
      onMouseDown={(e) => e.stopPropagation()}
      onPointerDown={(e) => e.stopPropagation()}
      onDoubleClick={(e) => e.stopPropagation()}
    >
      <header className="director-header">
        <span>3D导演台</span>
        <Button
          variant="ghost"
          size="icon-sm"
          aria-label="导演视角操作说明"
          onClick={() => setHelp(true)}
        >
          <CircleHelp />
        </Button>
        <span role="status" className="text-xs text-foreground/40">
          {retained ? "编辑未保存" : view.phase}
        </span>
        {!compact && (
          <Tabs
            value={scene.mode ?? "static"}
            onValueChange={(mode) => {
              setPlaying(false);
              if (mode !== scene.mode) setSwitchMode(mode as DirectorMode);
            }}
          >
            <TabsList>
              <TabsTrigger value="static" disabled={readOnly}>
                静态预演
              </TabsTrigger>
              <TabsTrigger value="dynamic" disabled={readOnly}>
                动态预演
              </TabsTrigger>
            </TabsList>
          </Tabs>
        )}
        <div className="flex-1" />
        {scene.mode === "static" && (
          <>
            <Button
              variant="ghost"
              size="icon-sm"
              aria-label="撤销"
              disabled={readOnly || !history.available("static", "undo")}
              onClick={() => applyHistory("undo")}
            >
              <Undo2 />
            </Button>
            <Button
              variant="ghost"
              size="icon-sm"
              aria-label="重做"
              disabled={readOnly || !history.available("static", "redo")}
              onClick={() => applyHistory("redo")}
            >
              <Redo2 />
            </Button>
          </>
        )}
        {!compact && (
          <>
            <Button
              variant="ghost"
              size="icon-sm"
              aria-label={showAssets ? "收起资产面板" : "展开资产面板"}
              aria-expanded={showAssets}
              onClick={() => {
                setAssetsExpanded(!showAssets);
                if (mobile && !showAssets) setPropertiesExpanded(false);
              }}
            >
              <PanelLeft />
            </Button>
            <Button
              variant="ghost"
              size="icon-sm"
              aria-label={showProperties ? "收起属性面板" : "展开属性面板"}
              aria-expanded={showProperties}
              onClick={() => {
                setPropertiesExpanded(!showProperties);
                if (mobile && !showProperties) setAssetsExpanded(false);
              }}
            >
              <PanelRight />
            </Button>
          </>
        )}
        {!compact && (
          <>
            <DirectorCapture
              onCommit={saveDocument}
              client={client}
              nodeID={nodeID}
              projectID={projectID}
              scene={scene}
              capture={capture}
              disabled={
                readOnly ||
                !!motionPreview ||
                !!previewScene ||
                !!path ||
                !!placement
              }
              onPause={() => setPlaying(false)}
              onError={onError}
            />
            <DirectorVideoDialog
              onCommit={saveDocument}
              client={client}
              nodeID={nodeID}
              projectID={projectID}
              scene={scene}
              exporter={videoExporter}
              disabled={
                readOnly ||
                !!motionPreview ||
                !!previewScene ||
                !!path ||
                !!placement
              }
              onBusy={(busy) => {
                setExportBusy(busy);
                if (busy) setPlaying(false);
              }}
            />
            <DirectorGallery
              key={`${scene.mode}:${scene.active_shot_id}`}
              client={client}
              nodeID={nodeID}
              scene={scene}
              projectID={projectID}
              disabled={readOnly}
              onChange={(next) => save(next)}
            />
          </>
        )}

        {!compact && (
          <Label className="inline-flex cursor-pointer items-center gap-1 text-xs">
            <FolderOpen size={14} />
            恢复文件
            <Input
              className="sr-only"
              type="file"
              accept="application/json,.json"
              disabled={readOnly}
              onChange={(event) => {
                const file = event.target.files?.[0];
                event.target.value = "";
                if (!file) return;
                void file
                  .text()
                  .then((text) => {
                    setRetained(
                      parseDirectorRecoveryFile(text, canvasID, nodeID),
                    );
                    setRecoverConfirm(true);
                  })
                  .catch((cause) =>
                    setError(
                      cause instanceof Error ? cause.message : "恢复文件无效",
                    ),
                  );
              }}
            />
          </Label>
        )}
        {!compact && (
          <Button
            variant="ghost"
            size="sm"
            disabled={readOnly}
            onClick={() => {
              if (client.retainedDirector(nodeID)) {
                setError("请先处理保留的场景编辑");
                return;
              }
              const epoch = client.getDocumentEpoch();
              void client
                .waitForSaved(() => {
                  if (client.getDocumentEpoch() !== epoch)
                    throw new Error("画布版本已变化");
                })
                .catch((e) => setError(e.message));
            }}
          >
            <Save />
            保存
          </Button>
        )}
        <Button
          variant="ghost"
          size="icon-sm"
          aria-label={compact ? "全屏编辑导演台" : "返回画布"}
          onClick={
            compact
              ? onExpand
              : () => {
                  if (retained || view.dirty) setExitConfirm(true);
                  else onClose?.();
                }
          }
        >
          {compact ? <Maximize2 /> : <X />}
        </Button>
      </header>
      {error && (
        <p role="alert" className="px-3 text-xs text-destructive">
          {error}
        </p>
      )}
      {recovery.status && (
        <p role="status" className="px-3 text-xs">
          {recovery.status}
        </p>
      )}
      {retained && (
        <div className="flex items-center gap-2 px-3 text-xs">
          <Button
            size="sm"
            disabled={readOnly}
            onClick={() => setRecoverConfirm(true)}
          >
            恢复未提交编辑
          </Button>
          <Button
            size="sm"
            variant="ghost"
            onClick={() =>
              saveAs(
                new Blob(
                  [
                    JSON.stringify({
                      version: 1,
                      canvas_id: canvasID,
                      node_id: nodeID,
                      director: retained,
                    }),
                  ],
                  { type: "application/json" },
                ),
                "导演台恢复文件.json",
              )
            }
          >
            <Download />
            下载恢复文件
          </Button>
          <Button
            size="sm"
            variant="ghost"
            disabled={readOnly}
            onClick={() =>
              void recovery
                .discard()
                .catch(() => setError("恢复稿删除未确认，保留当前内容"))
            }
          >
            放弃恢复稿
          </Button>
        </div>
      )}
      <Dialog open={recoverConfirm} onOpenChange={setRecoverConfirm}>
        <DialogContent>
          <DialogTitle>恢复未提交编辑</DialogTitle>
          <DialogDescription>
            将替换当前导演台的静态和动态工作区。其他画布节点不受影响，当前协作者的新编辑不会自动合并。
          </DialogDescription>
          <Button
            disabled={readOnly || !retained}
            onClick={() => {
              if (retained && saveDocument(retained)) setRecoverConfirm(false);
            }}
          >
            确认恢复
          </Button>
          <Button variant="ghost" onClick={() => setRecoverConfirm(false)}>
            继续查看当前场景
          </Button>
        </DialogContent>
      </Dialog>
      <Dialog open={exitConfirm} onOpenChange={setExitConfirm}>
        <DialogContent>
          <DialogTitle>编辑尚未保存确认</DialogTitle>
          <DialogDescription>
            {retained ? recovery.status : "请等待服务端确认后再离开。"}
          </DialogDescription>
          <Button onClick={() => setExitConfirm(false)}>继续编辑</Button>
          <Button
            variant="outline"
            disabled={!!retained}
            onClick={() => {
              void client
                .waitForSaved()
                .then(() => {
                  setExitConfirm(false);
                  onClose?.();
                })
                .catch(() => setError("保存尚未确认，请继续处理"));
            }}
          >
            保存并返回画布
          </Button>
          <Button
            variant="ghost"
            onClick={() => {
              setExitConfirm(false);
              onClose?.();
            }}
          >
            保留编辑并返回画布
          </Button>
        </DialogContent>
      </Dialog>
      <div className="director-main">
        {!compact && (
          <aside className="director-assets" hidden={!showAssets}>
            <h3>资产</h3>
            <Tabs value={tab} onValueChange={setTab}>
              <TabsList>
                <TabsTrigger value="actors">角色</TabsTrigger>
                <TabsTrigger value="objects">物体</TabsTrigger>
                <TabsTrigger value="backgrounds">背景</TabsTrigger>
                <TabsTrigger value="cameras">机位</TabsTrigger>
              </TabsList>
            </Tabs>
            {tab !== "cameras" && (
              <DirectorAssetLibrary
                projectID={projectID}
                refreshKey={assetRefresh}
                tab={tab as "actors" | "objects" | "backgrounds"}
                disabled={readOnly || !!motionPreview}
                guard={() => {
                  importBaseline({
                    epoch: client.getDocumentEpoch(),
                    mode: scene.mode ?? "static",
                    projectID,
                  });
                }}
                onActor={(id) => {
                  if (raw) {
                    setPlacement({
                      object: createDirectorActor(id),
                      document: raw,
                      epoch: client.getDocumentEpoch(),
                      scene,
                      projectID,
                    });
                    setPlaying(false);
                  }
                }}
                onPrimitive={(primitive) =>
                  create({ type: "add-object", primitive })
                }
                onBuiltinModel={(preset) => {
                  if (raw) {
                    setPlacement({
                      object: {
                        id: crypto.randomUUID(),
                        name: preset.name,
                        kind: "model",
                        builtin_model: preset.id,
                        transform: directorTransform(),
                        color: DIRECTOR_COLORS.white,
                        visible: true,
                        cast_shadow: true,
                        receive_shadow: true,
                        keyframes: [],
                      },
                      document: raw,
                      epoch: client.getDocumentEpoch(),
                      scene,
                      projectID,
                    });
                    setPlaying(false);
                  }
                }}
                onLocalImport={(kind) =>
                  setLocalImport({
                    epoch: client.getDocumentEpoch(),
                    mode: scene.mode ?? "static",
                    kind,
                    objectID: crypto.randomUUID(),
                    projectID,
                  })
                }
                onProjectPicker={openAssetPicker}
                onProjectAssets={(assets, objectKind) =>
                  placeProjectAssets(assets, {
                    kind:
                      objectKind === "panorama" || objectKind === "billboard"
                        ? "image"
                        : "model",
                    objectKind,
                    epoch: client.getDocumentEpoch(),
                    mode: scene.mode ?? "static",
                    projectID,
                  })
                }
                onCanvasPicker={(kind) =>
                  setCanvasBackground({
                    kind,
                    epoch: client.getDocumentEpoch(),
                    mode: scene.mode ?? "static",
                    projectID,
                  })
                }
              />
            )}
            {tab === "backgrounds" && scene.panorama && (
              <>
                <NumberField
                  label="全景旋转（度）"
                  value={((scene.panorama.rotation ?? 0) * 180) / Math.PI}
                  min={-180}
                  max={180}
                  disabled={readOnly}
                  onChange={(degrees) =>
                    save({
                      ...scene,
                      panorama: {
                        ...scene.panorama,
                        rotation: (degrees * Math.PI) / 180,
                      },
                    })
                  }
                />
                <Button
                  variant="ghost"
                  disabled={readOnly}
                  onClick={() => save({ ...scene, panorama: undefined })}
                >
                  移除全景
                </Button>
              </>
            )}
            {tab === "cameras" && (
              <Input
                aria-label="搜索机位"
                placeholder="搜索机位"
                value={assetSearch}
                onChange={(event) => setAssetSearch(event.target.value)}
              />
            )}
            {tab === "cameras" && (
              <div className="director-palette">
                {DIRECTOR_CAMERA_PRESETS.filter(([, label]) =>
                  label.includes(assetSearch),
                ).map(([kind, label]) => (
                  <Button
                    key={kind}
                    variant="secondary"
                    disabled={readOnly}
                    onClick={() => {
                      try {
                        const target =
                          selected?.kind === "object"
                            ? scene.objects.find(
                                (item) => item.id === selected.id,
                              )
                            : undefined;
                        const camera = directorCameraPreset(
                          kind,
                          target
                            ? {
                                id: target.id,
                                position: sampleDirectorTransform(
                                  target.transform,
                                  target.keyframes,
                                  currentTime,
                                ).position,
                              }
                            : undefined,
                          directorView.current?.(),
                        );
                        if (
                          save({
                            ...scene,
                            cameras: [...scene.cameras, camera],
                          })
                        )
                          select({ kind: "camera", id: camera.id });
                      } catch (cause) {
                        setError(
                          cause instanceof Error
                            ? cause.message
                            : "机位创建失败",
                        );
                      }
                    }}
                  >
                    <Camera />
                    {label}
                  </Button>
                ))}
              </div>
            )}
          </aside>
        )}
        <div className="director-stage">
          {!compact && (
            <div
              className="director-stage-tools"
              role="toolbar"
              aria-label="场景工具"
            >
              {(
                [
                  ["select", MousePointer2, "选择"],
                  ["translate", Move, "移动"],
                  ["rotate", RotateCw, "旋转"],
                  ["scale", Scaling, "缩放"],
                ] as const
              ).map(([mode, Icon, label]) => (
                <Button
                  key={mode}
                  variant={tool === mode ? "secondary" : "ghost"}
                  size="icon-sm"
                  aria-label={label}
                  aria-pressed={tool === mode}
                  onClick={() => setTool(mode)}
                >
                  <Icon />
                </Button>
              ))}
              <Button
                variant="ghost"
                size="icon-sm"
                aria-label="聚焦选中对象"
                onClick={() => setFocus((v) => v + 1)}
              >
                <Focus />
              </Button>
              <Button
                variant="ghost"
                size="sm"
                disabled={!selected || locked}
                onClick={() => {
                  const grounded =
                    selected && groundSelection.current?.(selected);
                  if (grounded) transform(grounded);
                }}
              >
                落地
              </Button>
            </div>
          )}
          {motionPreview && (
            <div role="status" className="director-placement-banner">
              动作预览尚未应用；结束预览后可拍摄或导出
            </div>
          )}
          {placement && (
            <div className="director-placement-banner" role="status">
              点击地面放置 {placement.object.name}
              {!!placement.remaining?.length &&
                `（另有 ${placement.remaining.length} 个待放置）`}
              <Button
                variant="ghost"
                size="sm"
                onClick={() => setPlacement(null)}
              >
                取消
              </Button>
            </div>
          )}
          <DirectorViewport
            views={views}
            previewObjectID={motionPreview?.object.id ?? null}
            targetCameraID={
              selected?.kind === "camera" && targetCameraID === selected.id
                ? targetCameraID
                : null
            }
            onTargetSelect={(id) => {
              select({ kind: "camera", id });
              setTargetCameraID(id);
            }}
            onTargetTransform={(id, target) => {
              const original = gesture.current;
              gesture.current = null;
              if (!original) return;
              const originalCamera = original.scene.cameras.find(
                (camera) => camera.id === id,
              );
              if (!originalCamera || originalCamera.locked || readOnly) return;
              try {
                const patch =
                  original.scene.mode === "dynamic"
                    ? keyDirectorCamera(
                        originalCamera,
                        Math.round(currentTime * fps) / fps,
                        { target },
                      )
                    : { target };
                save(
                  editDirector(original.scene, {
                    type: "entity",
                    selection: { kind: "camera", id },
                    patch: { ...patch, look_at_mode: "coordinates" },
                  }),
                  original.document,
                  original.epoch,
                );
              } catch (cause) {
                setError(
                  cause instanceof Error ? cause.message : "注视点无法保存",
                );
              }
            }}
            scene={
              placementScene ?? previewScene ?? motionPreviewScene ?? scene
            }
            placement={placement?.object.id}
            onPlace={(position) => {
              if (
                !placement ||
                placedObjectIDs.current.has(placement.object.id)
              )
                return;
              try {
                importBaseline({
                  epoch: placement.epoch,
                  mode: placement.scene.mode ?? "static",
                  projectID: placement.projectID,
                });
              } catch (cause) {
                setError(
                  cause instanceof Error ? cause.message : "无法放置素材",
                );
                return;
              }
              const object = {
                ...placement.object,
                transform: { ...placement.object.transform, position },
              };
              if (
                save(
                  {
                    ...placement.scene,
                    objects: [...placement.scene.objects, object],
                  },
                  placement.document,
                  placement.epoch,
                )
              ) {
                placedObjectIDs.current.add(object.id);
                const [following, ...remaining] = placement.remaining ?? [];
                const baseline = client
                  .getSnapshot()
                  .document?.nodes.find((node) => node.id === nodeID)
                  ?.serviceData?.config?.director;
                setPlacement(
                  following && baseline
                    ? {
                        ...placement,
                        object: following,
                        remaining,
                        document: baseline,
                        scene: activeDirectorScene(
                          migrateDirectorDocument(baseline),
                        ),
                      }
                    : null,
                );
                select({ kind: "object", id: object.id });
              }
            }}
            time={
              motionPreview && selected?.id === motionPreview.object.id
                ? motionPreview.time
                : currentTime
            }
            selected={selected}
            tool={tool}
            writable={
              !readOnly && !playing && !path && !placement && !motionPreview
            }
            projectID={projectID}
            compact={compact}
            focus={focus}
            onSelect={select}
            onGestureCancel={() => {
              gesture.current = null;
            }}
            onGestureStart={() => {
              if (raw)
                gesture.current = {
                  scene,
                  document: raw,
                  epoch: client.getDocumentEpoch(),
                };
              setPlaying(false);
            }}
            onTransform={(selection, value) => {
              const original = gesture.current;
              gesture.current = null;
              if (original)
                transform(
                  value,
                  selection,
                  original.scene,
                  original.epoch,
                  original.document,
                );
            }}
            onError={onError}
            onModelInfo={(info) =>
              setModelInfo((current) => ({ ...current, [info.objectID]: info }))
            }
            onGroundReady={(ground) => {
              groundSelection.current = ground;
            }}
            onViewReady={(view) => {
              directorView.current = view;
            }}
            onVideoReady={(exporter) => setVideoExporter(() => exporter)}
            onCaptureReady={(capture) => setCapture(() => capture)}
            onNodePosesReady={(snapshot) => {
              nodePoses.current = snapshot;
            }}
            onPosesReady={(snapshot) => {
              poses.current = snapshot;
            }}
          />
          {!compact && (
            <>
              <div className="director-view-switch">
                {(
                  [
                    ["names", "名称"],
                    ["paths", "路径"],
                    ["height", "高度参考"],
                  ] as const
                ).map(([key, label]) => (
                  <Button
                    key={key}
                    size="sm"
                    variant="ghost"
                    aria-pressed={views[key]}
                    onClick={() => setViews({ ...views, [key]: !views[key] })}
                  >
                    {label}
                  </Button>
                ))}
                <Choice
                  label="视角"
                  value={views.main}
                  items={[
                    ["director", "导演视角"],
                    ...scene.cameras.map((camera): [string, string] => [
                      camera.id,
                      camera.name,
                    ]),
                  ]}
                  onChange={(main) => setViews({ ...views, main })}
                />
              </div>
              <div className="director-preview-controls">
                <Button
                  variant={views.split ? "secondary" : "ghost"}
                  size="sm"
                  aria-pressed={views.split}
                  onClick={() =>
                    setViews({ ...views, split: !views.split, visible: true })
                  }
                >
                  <Columns2 />
                  {views.split ? "退出分屏" : "分屏"}
                </Button>
                <Choice
                  label="实时预览"
                  value={views.preview}
                  items={[
                    ["active", "当前镜头"],
                    ...scene.cameras.map((camera): [string, string] => [
                      camera.id,
                      camera.name,
                    ]),
                  ]}
                  onChange={(preview) => setViews({ ...views, preview })}
                />
                <Button
                  variant="ghost"
                  size="sm"
                  onClick={() =>
                    setViews({ ...views, visible: !views.visible })
                  }
                >
                  {views.visible ? "收起" : "展开"}
                </Button>
                {views.visible && (
                  <Slider
                    aria-label="预览窗口大小"
                    min={200}
                    max={600}
                    step={20}
                    value={[views.width]}
                    onValueChange={([width]) => setViews({ ...views, width })}
                  />
                )}
              </div>
            </>
          )}
        </div>
        {!compact && (
          <aside className="director-properties" hidden={!showProperties}>
            <h3>对象</h3>
            <div className="director-tree">
              {(
                [
                  ["object", scene.objects],
                  ["camera", scene.cameras],
                  ["light", scene.lights],
                ] as const
              ).flatMap(([kind, items]) =>
                items.map((item) => (
                  <div
                    key={`${kind}:${item.id}`}
                    className="director-tree-item"
                  >
                    <Button
                      variant={
                        selected?.id === item.id && selected.kind === kind
                          ? "secondary"
                          : "ghost"
                      }
                      size="sm"
                      className="min-w-0 flex-1 justify-start truncate"
                      onClick={() => select({ kind, id: item.id })}
                    >
                      {item.name}
                    </Button>
                    {kind === "camera" && (
                      <Button
                        variant={
                          targetCameraID === item.id ? "secondary" : "ghost"
                        }
                        size="icon-sm"
                        aria-label={`选择注视点 ${item.name}`}
                        onClick={() => {
                          select({ kind: "camera", id: item.id });
                          setTargetCameraID(item.id);
                        }}
                      >
                        <Crosshair />
                      </Button>
                    )}
                    <Button
                      variant="ghost"
                      size="icon-sm"
                      aria-label={`${item.locked ? "解锁" : "锁定"} ${item.name}`}
                      disabled={readOnly}
                      onClick={() =>
                        edit({
                          type: "entity",
                          selection: { kind, id: item.id },
                          patch: { locked: !item.locked },
                        })
                      }
                    >
                      {item.locked ? <Lock /> : <Unlock />}
                    </Button>
                    {kind === "object" && (
                      <Button
                        variant="ghost"
                        size="icon-sm"
                        aria-label={`${"visible" in item && item.visible ? "隐藏" : "显示"} ${item.name}`}
                        disabled={readOnly}
                        onClick={() =>
                          edit({
                            type: "entity",
                            selection: { kind, id: item.id },
                            patch: {
                              visible: !("visible" in item && item.visible),
                            },
                          })
                        }
                      >
                        {"visible" in item && item.visible ? (
                          <Eye />
                        ) : (
                          <EyeOff />
                        )}
                      </Button>
                    )}
                  </div>
                )),
              )}
            </div>
            <h3>属性</h3>
            {entity && selected && evaluated ? (
              <div className="director-fields">
                <Label className="director-field">
                  名称
                  <Input
                    aria-label="对象名称"
                    value={entity.name}
                    disabled={locked}
                    onChange={(e) =>
                      edit({
                        type: "entity",
                        selection: selected,
                        patch: { name: e.target.value },
                      })
                    }
                  />
                </Label>
                {(["position", "rotation", "scale"] as const).map((field) => (
                  <div key={field}>
                    <span className="text-xs text-foreground/45">
                      {
                        {
                          position: "位置",
                          rotation: "旋转（度）",
                          scale: "缩放",
                        }[field]
                      }
                    </span>
                    <div className="director-vector">
                      {["X", "Y", "Z"].map((axis, index) => (
                        <NumberField
                          key={axis}
                          label={`${field === "position" ? "位置（米）" : field === "rotation" ? "旋转（度）" : "缩放"} ${axis}`}
                          value={
                            evaluated[field][index] *
                            (field === "rotation" ? 180 / Math.PI : 1)
                          }
                          min={field === "scale" ? 0.01 : undefined}
                          disabled={locked}
                          onChange={(number) => {
                            const vector = [...evaluated[field]];
                            vector[index] =
                              number *
                              (field === "rotation" ? Math.PI / 180 : 1);
                            transform(
                              { ...evaluated, [field]: vector },
                              selected,
                              scene,
                              client.getDocumentEpoch(),
                              raw,
                              field,
                            );
                          }}
                        />
                      ))}
                    </div>
                  </div>
                ))}
                {object && (
                  <Label className="director-field">
                    颜色
                    <Input
                      type="color"
                      aria-label="对象颜色"
                      value={object.color}
                      disabled={locked}
                      onChange={(e) =>
                        edit({
                          type: "entity",
                          selection: selected,
                          patch: { color: e.target.value },
                        })
                      }
                    />
                  </Label>
                )}
                {object && (
                  <Button
                    variant="ghost"
                    size="sm"
                    disabled={locked}
                    onClick={() =>
                      edit({
                        type: "entity",
                        selection: selected!,
                        patch: {
                          color:
                            object.kind === "primitive"
                              ? DIRECTOR_COLORS.primitive
                              : DIRECTOR_COLORS.white,
                        },
                      })
                    }
                  >
                    恢复原色
                  </Button>
                )}
                {object && (
                  <NumberField
                    label="不透明度"
                    value={object.opacity ?? 1}
                    min={0}
                    max={1}
                    step={0.05}
                    disabled={locked}
                    onChange={(opacity) =>
                      edit({
                        type: "entity",
                        selection: selected,
                        patch: { opacity },
                      })
                    }
                  />
                )}
                {object?.builtin_actor && (
                  <DirectorBodyEditor
                    object={object}
                    disabled={locked || scene.mode === "dynamic"}
                    onChange={(patch) =>
                      edit({ type: "entity", selection: selected, patch })
                    }
                  />
                )}
                {object?.kind === "actor" && (
                  <Choice
                    label="姿势"
                    value={object.pose ?? "stand"}
                    disabled={locked || scene.mode === "dynamic"}
                    items={[
                      ["neutral", "默认姿势"],
                      ["stand", "站立"],
                      ["t_pose", "T 字姿势"],
                      ["sit", "坐姿"],
                      ["squat", "蹲姿"],
                      ["kneel_single", "单膝跪"],
                      ["kneel_double", "双膝跪"],
                      ["walk", "行走"],
                      ["run", "跑步"],
                      ["hands_hips", "叉腰"],
                      ["bow", "鞠躬"],
                      ["wave", "挥手"],
                      ["think", "思考"],
                      ["arms_crossed", "抱臂"],
                      ["phone", "看手机"],
                      ["lean", "倚靠"],
                      ["fight", "格斗"],
                      ["kick", "踢腿"],
                      ["throw", "投掷"],
                      ["push", "推举"],
                      ["reach", "伸手"],
                    ]}
                    onChange={(pose) =>
                      edit({
                        type: "entity",
                        selection: selected,
                        patch: {
                          pose,
                          bone_overrides: {},
                          frozen_bones: {},
                          frozen_nodes: {},
                        },
                      })
                    }
                  />
                )}
                {object &&
                  (object.asset_id || object.builtin_actor) &&
                  object.kind !== "billboard" && (
                    <div className="space-y-3" key={object.id}>
                      {scene.mode !== "dynamic" && (
                        <DirectorPoseEditor
                          nodePaths={modelInfo[object.id]?.bonePaths}
                          rest={modelInfo[object.id]?.rest}
                          object={object}
                          disabled={locked}
                          onChange={(patch) =>
                            edit({ type: "entity", selection: selected, patch })
                          }
                        />
                      )}
                      <DirectorMotionLibrary
                        key={`${object.id}:${scene.mode}`}
                        object={object}
                        info={modelInfo[object.id]}
                        dynamic={scene.mode === "dynamic"}
                        time={currentTime}
                        duration={duration}
                        disabled={readOnly || !!object.locked}
                        onChange={(patch) => {
                          setMotionPreview(null);
                          edit({ type: "entity", selection: selected, patch });
                        }}
                        onPreview={setMotionPreview}
                        onCapturePose={() => poses.current?.()[object.id] ?? {}}
                      />
                      <DirectorMotionEditor
                        key={object.id}
                        object={object}
                        info={modelInfo[object.id]}
                        dynamic={scene.mode === "dynamic"}
                        time={currentTime}
                        duration={duration}
                        disabled={locked}
                        onChange={(patch) =>
                          edit({ type: "entity", selection: selected, patch })
                        }
                      />
                    </div>
                  )}
                {camera && (
                  <>
                    <NumberField
                      label="视场角"
                      value={cameraView?.fov ?? camera.fov}
                      min={1}
                      max={179}
                      disabled={locked}
                      onChange={(fov) => cameraOptics({ fov })}
                    />
                    <NumberField
                      label="画面倾斜 (°)"
                      value={((cameraView?.roll ?? 0) * 180) / Math.PI}
                      min={-180}
                      max={180}
                      disabled={locked}
                      onChange={(degrees) =>
                        cameraOptics({ roll: (degrees * Math.PI) / 180 })
                      }
                    />
                    <Choice
                      label="注视方式"
                      value={camera.look_at_mode || "coordinates"}
                      disabled={locked}
                      items={[
                        ["coordinates", "坐标"],
                        ["rotation", "旋转"],
                        ["object", "对象"],
                      ]}
                      onChange={(look_at_mode) =>
                        edit({
                          type: "entity",
                          selection: selected,
                          patch: { look_at_mode },
                        })
                      }
                    />
                    {camera.look_at_mode === "object" ? (
                      <Choice
                        label="注视对象"
                        value={camera.look_at_object_id ?? "none"}
                        disabled={locked}
                        items={[
                          ["none", "请选择"],
                          ...scene.objects.map((o): [string, string] => [
                            o.id,
                            o.name,
                          ]),
                        ]}
                        onChange={(id) =>
                          edit({
                            type: "entity",
                            selection: selected,
                            patch: {
                              look_at_object_id: id === "none" ? undefined : id,
                            },
                          })
                        }
                      />
                    ) : (
                      <div className="director-vector">
                        {["X", "Y", "Z"].map((axis, i) => (
                          <NumberField
                            key={axis}
                            label={`注视 ${axis}`}
                            value={cameraView?.target[i] ?? camera.target[i]}
                            disabled={locked}
                            onChange={(v) => {
                              const target = [
                                ...(cameraView?.target ?? camera.target),
                              ];
                              target[i] = v;
                              cameraOptics({ target });
                            }}
                          />
                        ))}
                      </div>
                    )}
                    <Button
                      variant="outline"
                      disabled={readOnly}
                      onClick={() =>
                        edit({ type: "add-shot", cameraID: camera.id })
                      }
                    >
                      使用此机位新建镜头
                    </Button>
                  </>
                )}
                {light && (
                  <>
                    {(light.type === "directional" ||
                      light.type === "spot") && (
                      <>
                        <NumberField
                          label="水平角度（度）"
                          value={
                            directorLightAngles(light.transform.rotation)
                              .horizontal
                          }
                          min={-180}
                          max={180}
                          disabled={locked}
                          onChange={(horizontal) =>
                            edit({
                              type: "entity",
                              selection: selected,
                              patch: {
                                transform: {
                                  ...light.transform,
                                  rotation: directorLightRotation(
                                    horizontal,
                                    directorLightAngles(
                                      light.transform.rotation,
                                    ).elevation,
                                  ),
                                },
                              },
                            })
                          }
                        />
                        <NumberField
                          label="照射高度角（度）"
                          value={
                            directorLightAngles(light.transform.rotation)
                              .elevation
                          }
                          min={-90}
                          max={90}
                          disabled={locked}
                          onChange={(elevation) =>
                            edit({
                              type: "entity",
                              selection: selected,
                              patch: {
                                transform: {
                                  ...light.transform,
                                  rotation: directorLightRotation(
                                    directorLightAngles(
                                      light.transform.rotation,
                                    ).horizontal,
                                    elevation,
                                  ),
                                },
                              },
                            })
                          }
                        />
                      </>
                    )}
                    <Choice
                      label="灯光类型"
                      value={light.type}
                      items={[
                        ["directional", "平行光"],
                        ["point", "点光源"],
                        ["spot", "聚光灯"],
                        ["ambient", "环境光"],
                      ]}
                      disabled={locked}
                      onChange={(type) =>
                        edit({
                          type: "entity",
                          selection: selected,
                          patch: { type },
                        })
                      }
                    />
                    <NumberField
                      label="灯光强度"
                      value={light.intensity}
                      min={0}
                      max={100}
                      disabled={locked}
                      onChange={(intensity) =>
                        edit({
                          type: "entity",
                          selection: selected,
                          patch: { intensity },
                        })
                      }
                    />
                    <Label className="director-field">
                      灯光颜色
                      <Input
                        type="color"
                        value={light.color}
                        disabled={locked}
                        onChange={(e) =>
                          edit({
                            type: "entity",
                            selection: selected,
                            patch: { color: e.target.value },
                          })
                        }
                      />
                    </Label>
                  </>
                )}
                {scene.mode === "dynamic" && selected.kind !== "light" && (
                  <Button
                    variant="outline"
                    disabled={locked}
                    onClick={() => {
                      if (raw) {
                        setPlaying(false);
                        setPath({
                          scene,
                          selection: selected,
                          document: raw,
                          epoch: client.getDocumentEpoch(),
                        });
                      }
                    }}
                  >
                    {selected.kind === "camera" ? "添加运镜" : "绘制移动路径"}
                  </Button>
                )}
                <Button
                  variant="outline"
                  disabled={locked}
                  onClick={() => {
                    edit({ type: "remove", selection: selected });
                  }}
                >
                  <Trash2 />
                  删除对象
                </Button>
              </div>
            ) : (
              <div className="director-fields">
                <Choice
                  label="添加光源"
                  value="none"
                  items={[
                    ["none", "选择光源"],
                    ["directional", "平行光"],
                    ["point", "点光源"],
                    ["spot", "聚光灯"],
                    ["ambient", "环境光"],
                  ]}
                  disabled={readOnly}
                  onChange={(light) => {
                    if (light !== "none") create({ type: "add-light", light });
                  }}
                />
                <Choice
                  label="环境预设"
                  value="custom"
                  items={[
                    ["custom", "自定义"],
                    ...DIRECTOR_ENVIRONMENTS.map(
                      ([value, label]): [string, string] => [value, label],
                    ),
                  ]}
                  disabled={readOnly}
                  onChange={(preset) => {
                    if (preset !== "custom")
                      save(applyDirectorEnvironment(scene, preset));
                  }}
                />
                <p className="text-xs text-muted-foreground">
                  应用预设会替换主光、补光和轮廓光。
                </p>
                <Label className="director-field">
                  背景颜色
                  <Input
                    aria-label="背景颜色"
                    type="color"
                    value={scene.background}
                    disabled={readOnly}
                    onChange={(e) =>
                      save({ ...scene, background: e.target.value })
                    }
                  />
                </Label>
                <NumberField
                  label="曝光"
                  value={scene.exposure ?? 1.05}
                  min={0.1}
                  max={5}
                  step={0.05}
                  disabled={readOnly}
                  onChange={(exposure) => save({ ...scene, exposure })}
                />
                <NumberField
                  label="环境强度"
                  value={scene.environment_intensity}
                  min={0}
                  max={10}
                  disabled={readOnly}
                  onChange={(environment_intensity) =>
                    save({ ...scene, environment_intensity })
                  }
                />
                <Label className="director-field">
                  地面颜色
                  <Input
                    type="color"
                    aria-label="地面颜色"
                    value={scene.ground?.color ?? DIRECTOR_COLORS.ground}
                    disabled={readOnly}
                    onChange={(e) =>
                      save({
                        ...scene,
                        ground: {
                          visible: true,
                          opacity: 1,
                          height: 0,
                          ...scene.ground,
                          color: e.target.value,
                        },
                      })
                    }
                  />
                </Label>
                <NumberField
                  label="地面透明度"
                  value={scene.ground?.opacity ?? 1}
                  min={0}
                  max={1}
                  disabled={readOnly}
                  onChange={(opacity) =>
                    save({
                      ...scene,
                      ground: {
                        visible: true,
                        height: 0,
                        ...scene.ground,
                        opacity,
                      },
                    })
                  }
                />
                <Button
                  variant="outline"
                  disabled={readOnly}
                  onClick={() =>
                    save({
                      ...scene,
                      ground: {
                        height: 0,
                        opacity: 1,
                        ...scene.ground,
                        visible: !scene.ground?.visible,
                      },
                    })
                  }
                >
                  {scene.ground?.visible ? "隐藏地面" : "显示地面"}
                </Button>
                <Button
                  variant="outline"
                  disabled={readOnly}
                  onClick={() =>
                    save({ ...scene, grid_visible: !scene.grid_visible })
                  }
                >
                  {scene.grid_visible ? "隐藏网格" : "显示网格"}
                </Button>
                <Button
                  variant="outline"
                  disabled={readOnly}
                  onClick={() =>
                    save({ ...scene, grid_snap: !scene.grid_snap })
                  }
                >
                  {scene.grid_snap ? "关闭吸附" : "开启吸附"}
                </Button>
                <Choice
                  label="画幅"
                  value={scene.aspect_ratio || "16:9"}
                  items={[
                    "adaptive",
                    "21:9",
                    "16:9",
                    "4:3",
                    "1:1",
                    "3:4",
                    "9:16",
                    "2.39:1",
                  ].map((ratio) => [
                    ratio,
                    ratio === "adaptive" ? "自适应" : ratio,
                  ])}
                  disabled={readOnly}
                  onChange={(aspect_ratio) => save({ ...scene, aspect_ratio })}
                />
              </div>
            )}
          </aside>
        )}
      </div>
      {!compact && scene.mode === "dynamic" && (
        <div className="director-timeline">
          <div className="director-playback">
            <Button
              variant="ghost"
              size="icon-sm"
              aria-label="撤销"
              disabled={
                readOnly || !history.available(scene.mode ?? "static", "undo")
              }
              onClick={() => applyHistory("undo")}
            >
              <Undo2 />
            </Button>
            <Button
              variant="ghost"
              size="icon-sm"
              aria-label="重做"
              disabled={
                readOnly || !history.available(scene.mode ?? "static", "redo")
              }
              onClick={() => applyHistory("redo")}
            >
              <Redo2 />
            </Button>
            <Choice
              label="镜头"
              value={scene.active_shot_id}
              items={scene.shots.map((s) => [s.id, s.name])}
              disabled={readOnly}
              onChange={(active_shot_id) => {
                setPlaying(false);
                setTime(0);
                save({ ...scene, active_shot_id });
              }}
            />
            {shot && (
              <>
                <Choice
                  label="分镜机位"
                  value={shot.camera_id}
                  items={scene.cameras.map((camera) => [
                    camera.id,
                    camera.name,
                  ])}
                  disabled={readOnly}
                  onChange={(camera_id) =>
                    edit({ type: "shot", id: shot.id, patch: { camera_id } })
                  }
                />
                {([-1, 1] as const).map((direction) => (
                  <Button
                    key={direction}
                    variant="ghost"
                    size="sm"
                    disabled={
                      readOnly ||
                      scene.shots.indexOf(shot) + direction < 0 ||
                      scene.shots.indexOf(shot) + direction >=
                        scene.shots.length
                    }
                    onClick={() => {
                      const shots = [...scene.shots],
                        index = shots.indexOf(shot);
                      [shots[index], shots[index + direction]] = [
                        shots[index + direction],
                        shots[index],
                      ];
                      save({ ...scene, shots });
                    }}
                  >
                    {direction < 0 ? "前移分镜" : "后移分镜"}
                  </Button>
                ))}
                <NumberField
                  label="时长（秒）"
                  value={shot.duration}
                  min={0.1}
                  max={3600}
                  disabled={readOnly}
                  onChange={(duration) =>
                    edit({ type: "shot", id: shot.id, patch: { duration } })
                  }
                />
                <Choice
                  label="帧率"
                  value={String(shot.fps)}
                  items={[24, 25, 30, 50, 60].map((fps) => [
                    String(fps),
                    `${fps}fps`,
                  ])}
                  disabled={readOnly}
                  onChange={(fps) =>
                    edit({
                      type: "shot",
                      id: shot.id,
                      patch: { fps: Number(fps) },
                    })
                  }
                />
                <Button
                  variant="ghost"
                  size="icon-sm"
                  aria-label="删除当前镜头"
                  disabled={readOnly || scene.shots.length < 2}
                  onClick={() => edit({ type: "remove-shot", id: shot.id })}
                >
                  <Trash2 />
                </Button>
              </>
            )}
            <div className="flex-1" />
            <Button
              variant="ghost"
              size="sm"
              aria-pressed={loop}
              onClick={() => setLoop(!loop)}
            >
              循环
            </Button>
            <Button
              variant="secondary"
              size="icon-sm"
              aria-label={playing ? "暂停预演" : "播放预演"}
              disabled={scene.mode !== "dynamic"}
              onClick={() => {
                if (currentTime >= duration) setTime(0);
                setPlaying(!playing);
              }}
            >
              {playing ? <Pause /> : <Play />}
            </Button>
            <span className="text-xs tabular-nums">
              {currentTime.toFixed(2)} / {duration.toFixed(2)}
            </span>
          </div>
          <Slider
            aria-label="预演时间"
            min={0}
            max={duration}
            step={1 / fps}
            value={[currentTime]}
            onValueChange={([time]) => {
              setTime(time);
              setPlaying(false);
            }}
          />
          {scene.mode === "dynamic" && (
            <div className="director-keyframes">
              <Button
                size="sm"
                variant="outline"
                disabled={!animated || locked}
                onClick={() =>
                  selected &&
                  evaluated &&
                  edit({
                    type: "keyframe",
                    selection: selected,
                    time: Math.round(currentTime * fps) / fps,
                    transform: evaluated,
                  })
                }
              >
                <Plus />
                记录关键帧
              </Button>
              <DirectorTimeline
                scene={scene}
                time={currentTime}
                duration={duration}
                fps={fps}
                disabled={readOnly}
                onTime={(time) => {
                  setTime(time);
                  setPlaying(false);
                }}
                onSelect={select}
                onChange={(next) => save(next)}
              />
            </div>
          )}
        </div>
      )}
      <Dialog
        open={help}
        onOpenChange={(open) => (open ? setHelp(true) : closeHelp())}
      >
        <DialogContent>
          <DialogTitle>导演视角操作</DialogTitle>
          <DialogDescription>
            点击场景后使用键盘移动和旋转视角。
          </DialogDescription>
          <p>Q 下降 · W 前进 · E 上升 · A 向左 · S 后退 · D 向右</p>
          <p>I 上转 · J 左转 · K 下转 · L 右转 · Shift 加速</p>
          <p>0 选择 · 1 移动 · 2 旋转 · 3 缩放 · G 落地 · Esc 取消拖拽</p>
          <Button onClick={closeHelp}>知道了</Button>
        </DialogContent>
      </Dialog>
      <Dialog
        open={!!switchMode}
        onOpenChange={(open) => {
          if (!open) setSwitchMode(null);
        }}
      >
        <DialogContent data-canvas-no-zoom>
          <DialogTitle>
            {switchMode === "dynamic" ? "切换到动态预演" : "切换到静态预演"}
          </DialogTitle>
          <DialogDescription>
            {switchMode === "dynamic"
              ? "是否同步静态场景？同步将保留动态时间轴，角色恢复默认姿势。"
              : "是否将当前帧冻结并同步到静态场景？动态时间轴将保留。"}
          </DialogDescription>
          <div className="flex justify-end gap-2">
            <Button variant="ghost" onClick={() => setSwitchMode(null)}>
              取消
            </Button>
            <Button
              variant="outline"
              disabled={readOnly}
              onClick={() => changeMode(false)}
            >
              不同步
            </Button>
            <Button disabled={readOnly} onClick={() => changeMode(true)}>
              同步
            </Button>
          </div>
        </DialogContent>
      </Dialog>
      {path && (
        <DirectorPathEditor
          scene={path.scene}
          selection={path.selection}
          time={currentTime}
          disabled={readOnly}
          onPreview={(next, start) => {
            setPreviewScene(next);
            setTime(start);
            setPlaying(true);
          }}
          onApply={(next) => save(next, path.document, path.epoch)}
          onClose={() => {
            setPath(null);
            setPreviewScene(null);
            setPlaying(false);
          }}
        />
      )}
      {localImport && (
        <DirectorImportDialog
          projectID={projectID}
          disabled={readOnly}
          onClose={() => setLocalImport(null)}
          guard={() => {
            importBaseline(localImport);
          }}
          onAdd={async (asset) => {
            const baseline = importBaseline(localImport);
            setAssetRefresh((value) => value + 1);
            if (
              asset.project_id !== localImport.projectID ||
              asset.kind !== "model" ||
              !asset.id
            )
              throw new Error("模型归属不匹配");
            const next = activeDirectorScene(migrateDirectorDocument(baseline));
            setPlacement({
              object: {
                id: localImport.objectID,
                name: asset.file_name?.slice(0, 128) || "模型",
                kind: localImport.kind,
                asset_id: asset.id,
                transform: directorTransform(),
                color: DIRECTOR_COLORS.white,
                visible: true,
                cast_shadow: true,
                receive_shadow: true,
                keyframes: [],
              },
              document: baseline,
              scene: next,
              epoch: localImport.epoch,
              projectID: localImport.projectID,
            });
            setPlaying(false);
          }}
        />
      )}
      {canvasBackground && (
        <Dialog
          open
          onOpenChange={(open) => {
            if (!open) setCanvasBackground(null);
          }}
        >
          <DialogContent>
            <DialogTitle>从画布选择背景</DialogTitle>
            <DialogDescription>
              选择已保存的图片节点，
              {canvasBackground.kind === "panorama"
                ? "设为全景背景"
                : "作为平面背景放置"}
              。
            </DialogDescription>
            {(view.document?.nodes ?? []).filter(
              (node) =>
                node.type === "image" && node.serviceData?.media_ref?.asset_id,
            ).length === 0 && <p className="text-sm">画布暂无可用图片</p>}
            {(view.document?.nodes ?? [])
              .filter(
                (node) =>
                  node.type === "image" &&
                  node.serviceData?.media_ref?.asset_id,
              )
              .map((node) => (
                <Button
                  key={node.id}
                  variant="outline"
                  disabled={readOnly}
                  onClick={() => {
                    const scope = canvasBackground;
                    const assetID = node.serviceData?.media_ref?.asset_id;
                    if (!assetID) return;
                    void getMediaPreview({ pid: projectID, asset_id: assetID })
                      .then((result) => {
                        importBaseline(scope);
                        const current = client
                          .getSnapshot()
                          .document?.nodes.find(
                            (candidate) => candidate.id === node.id,
                          );
                        if (
                          current?.serviceData?.media_ref?.asset_id !==
                            assetID ||
                          !result.asset
                        )
                          throw new Error("画布图片已变化，请重新选择");
                        placeProjectAssets([result.asset], {
                          ...scope,
                          kind: "image",
                          objectKind: scope.kind,
                        });
                        setCanvasBackground(null);
                      })
                      .catch((cause) =>
                        setError(
                          cause instanceof Error
                            ? cause.message
                            : "背景读取失败",
                        ),
                      );
                  }}
                >
                  {node.title || "图片"}
                </Button>
              ))}
          </DialogContent>
        </Dialog>
      )}
      {picker && (
        <ProjectAssetDialog
          projectID={projectID}
          allowedKinds={[picker.kind]}
          initialKind={picker.kind}
          canvasPresentation
          confirmLabel={
            picker.objectKind === "panorama" ? "设为全景背景" : "选择并放置"
          }
          onClose={() => setPicker(null)}
          onAdd={async (assets) => {
            placeProjectAssets(assets, picker);
            setPicker(null);
          }}
        />
      )}
    </section>
  );
}
