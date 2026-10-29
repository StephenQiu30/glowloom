"use client";
import { useEffect, useMemo, useRef, useState } from "react";
import { Button } from "@/components/ui/button";
import { Input } from "@/components/ui/input";
import { Label } from "@/components/ui/label";
import { Tabs, TabsList, TabsTrigger } from "@/components/ui/tabs";
import {
  Select,
  SelectTrigger,
  SelectValue,
  SelectContent,
  SelectItem,
} from "@/components/ui/select";
import { DIRECTOR_BUILTIN_MOTION_PRESETS } from "./director-motion-catalog";
import {
  addDirectorMotionSource,
  directorMotionSourceName,
  type DirectorMotionSource,
} from "./director-motion-library-model";
import { placeDirectorMotionClip } from "./director-motion-model";
import type { DirectorModelInfo } from "./director-motion";
import type { DirectorObject, DirectorTransform } from "./director-model";
export type DirectorMotionPreview = {
  object: DirectorObject;
  time: number;
} | null;
export type DirectorMotionLibraryProps = {
  object: DirectorObject;
  info?: DirectorModelInfo;
  dynamic: boolean;
  time: number;
  duration: number;
  disabled: boolean;
  onChange: (patch: Partial<DirectorObject>) => void;
  onPreview: (value: DirectorMotionPreview) => void;
  onCapturePose: () => Record<string, DirectorTransform>;
};
export function DirectorMotionLibrary(props: DirectorMotionLibraryProps) {
  const { object, info, dynamic, time, duration, disabled, onChange } = props;
  const [tab, setTab] = useState("preset"),
    [query, setQuery] = useState(""),
    [error, setError] = useState(""),
    [selected, setSelected] = useState(""),
    [previewTime, setPreviewTime] = useState(0),
    [playing, setPlaying] = useState(false),
    [previewing, setPreviewing] = useState(false),
    [previewBaseline, setPreviewBaseline] = useState(""),
    [busy, setBusy] = useState(false),
    [file, setFile] = useState<File | null>(null),
    [names, setNames] = useState<string[]>([]),
    [pending, setPending] = useState<DirectorMotionSource | null>(null),
    [poseName, setPoseName] = useState("我的姿势");
  const current = useRef(props),
    request = useRef<AbortController | null>(null),
    pendingBaseline = useRef(""),
    previewCache = useRef<{
      baseline: string;
      name: string;
      object: DirectorObject;
    } | null>(null);
  const baseline = useMemo(() => JSON.stringify(object), [object]);
  useEffect(() => {
    current.current = props;
  });
  useEffect(
    () => () => {
      request.current?.abort();
      current.current.onPreview(null);
    },
    [baseline],
  );
  const available =
    info?.objectID === object.id && info.assetID === (object.asset_id ?? "")
      ? info
      : undefined;
  const rows = (available?.animations ?? []).map((animation) => ({
    ...animation,
    label:
      object.motion_library?.find(
        (source) => directorMotionSourceName(source.id) === animation.name,
      )?.name ??
      DIRECTOR_BUILTIN_MOTION_PRESETS.find(
        (item) => item.name === animation.name,
      )?.label ??
      animation.name,
    mine: animation.name.startsWith("user:"),
  }));
  const visible = rows.filter(
    (row) =>
      (tab === "mine" ? row.mine : !row.mine) &&
      `${row.name} ${row.label}`.toLowerCase().includes(query.toLowerCase()),
  );
  const picked = rows.find((row) => row.name === selected);
  const readOnly = disabled || object.locked === true;
  function stop() {
    setPlaying(false);
    setPreviewing(false);
    props.onPreview(null);
  }
  function preview(name: string, t: number) {
    const row = rows.find((item) => item.name === name);
    if (!row) return;
    let copy =
      previewCache.current?.baseline === baseline &&
      previewCache.current.name === name
        ? previewCache.current.object
        : undefined;
    if (!copy) {
      copy = structuredClone(object);
      copy.keyframes = [];
      copy.bone_tracks = [];
      copy.bone_overrides = {};
      copy.motion_clips = [
        {
          id: crypto.randomUUID(),
          name: row.label,
          source_animation: row.name,
          start: 0,
          duration: row.duration,
          playback_rate: 1,
          loop: false,
        },
      ];
      previewCache.current = { baseline, name, object: copy };
    }
    setPreviewBaseline(baseline);
    setSelected(name);
    setPreviewTime(t);
    setPreviewing(true);
    props.onPreview({
      object: copy,
      time: Math.min(t, Math.max(0, row.duration - 1e-7)),
    });
  }
  useEffect(() => {
    if (!playing || !picked || previewBaseline !== baseline) return;
    let raf = 0;
    let start: number | undefined;
    const tick = (now: number) => {
      start ??= now - previewTime * 1000;
      const t = (now - start) / 1000;
      if (t >= picked.duration) {
        setPlaying(false);
        return;
      }
      preview(picked.name, t);
      raf = requestAnimationFrame(tick);
    };
    raf = requestAnimationFrame(tick);
    return () => cancelAnimationFrame(raf);
    // The start time is fixed for this playback session; slider edits pause first.
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [playing, selected, baseline]);
  function apply(name: string) {
    if (readOnly || !dynamic || !available) return;
    const row = rows.find((item) => item.name === name);
    if (!row) return;
    try {
      const id = crypto.randomUUID();
      onChange({
        motion_clips: placeDirectorMotionClip(
          object.motion_clips ?? [],
          {
            id,
            name: row.label,
            source_animation: name,
            start: time,
            duration: Math.min(row.duration, duration - time),
            playback_rate: 1,
            loop: false,
          },
          duration,
        ),
        active_motion_clip_id: id,
      });
      stop();
      setError("");
    } catch (cause) {
      setError(cause instanceof Error ? cause.message : "动作无法应用");
    }
  }
  async function choose(chosen: File, animation?: string) {
    request.current?.abort();
    const controller = new AbortController();
    request.current = controller;
    const before = baseline;
    setBusy(true);
    setFile(chosen);
    if (!animation) setNames([]);
    setPending(null);
    setError("");
    try {
      const { importDirectorMotion } = await import("./director-motion-import");
      const imported = await importDirectorMotion(
        chosen,
        available?.bones ?? [],
        controller.signal,
        animation,
        (availableNames) => {
          if (
            !controller.signal.aborted &&
            JSON.stringify(current.current.object) === before
          )
            setNames(availableNames);
        },
      );
      controller.signal.throwIfAborted();
      if (
        JSON.stringify(current.current.object) !== before ||
        current.current.disabled
      )
        throw new Error("角色或权限已变化，请重新导入");
      setFile(chosen);
      setNames(imported.names);
      pendingBaseline.current = before;
      setPending(imported.source);
    } catch (cause) {
      if (!controller.signal.aborted)
        setError(cause instanceof Error ? cause.message : "动作导入失败");
    } finally {
      if (request.current === controller) {
        request.current = null;
        setBusy(false);
      }
    }
  }
  async function savePose() {
    if (readOnly || !available?.rest) return;
    try {
      const before = baseline;
      const pose = props.onCapturePose();
      const { directorPoseSource } =
        await import("./director-motion-library-runtime");
      if (
        JSON.stringify(current.current.object) !== before ||
        current.current.disabled
      )
        throw new Error("角色或权限已变化");
      const source = directorPoseSource(poseName, pose, available.rest);
      onChange(addDirectorMotionSource(object, source, available.bones));
      setTab("mine");
      stop();
      setError("");
    } catch (cause) {
      setError(cause instanceof Error ? cause.message : "姿势无法保存");
    }
  }
  return (
    <div className="space-y-3">
      <h3>动作库</h3>
      <Tabs value={tab} onValueChange={setTab}>
        <TabsList>
          <TabsTrigger value="preset">预设</TabsTrigger>
          <TabsTrigger value="mine">我的</TabsTrigger>
        </TabsList>
      </Tabs>
      <Input
        aria-label="搜索动作"
        placeholder="搜索动作"
        value={query}
        onChange={(event) => setQuery(event.target.value)}
      />
      {!available ? (
        <p role="status">等待角色骨架与动作加载</p>
      ) : (
        <>
          <p className="text-xs text-muted-foreground">
            {visible.length} 个动作
          </p>
          <div className="max-h-64 space-y-2 overflow-y-auto">
            {visible.map((row) => (
              <div key={row.name} className="space-y-2 rounded border p-2">
                <span className="block truncate" title={row.label}>
                  {row.label}
                </span>
                <div className="flex flex-wrap gap-2">
                  <Button
                    size="sm"
                    variant="ghost"
                    aria-label={`预览${row.label}`}
                    onClick={() => {
                      setPlaying(false);
                      preview(row.name, 0);
                    }}
                  >
                    预览
                  </Button>
                  <Button
                    size="sm"
                    disabled={readOnly || !dynamic}
                    aria-label={`应用${row.label}`}
                    onClick={() => apply(row.name)}
                  >
                    应用
                  </Button>
                  {row.mine && (
                    <Button
                      size="sm"
                      variant="ghost"
                      disabled={
                        readOnly ||
                        (object.motion_clips ?? []).some(
                          (clip) => clip.source_animation === row.name,
                        )
                      }
                      aria-label={`删除${row.label}`}
                      onClick={() => {
                        if (readOnly) return;
                        onChange({
                          motion_library: (object.motion_library ?? []).filter(
                            (source) =>
                              directorMotionSourceName(source.id) !== row.name,
                          ),
                        });
                        stop();
                      }}
                    >
                      删除
                    </Button>
                  )}
                </div>
              </div>
            ))}
            {!visible.length && <p>暂无匹配动作</p>}
          </div>
        </>
      )}
      {picked && previewing && previewBaseline === baseline && (
        <div className="space-y-2 rounded border p-2">
          <span className="block truncate" title={picked.label}>
            {picked.label}
          </span>
          <Label>
            预览时间
            <Input
              aria-label="动作预览时间"
              type="number"
              min={0}
              max={picked.duration}
              step={0.0333}
              value={Number(previewTime.toFixed(3))}
              onChange={(event) => {
                const t = Number(event.target.value);
                if (Number.isFinite(t) && t >= 0 && t <= picked.duration) {
                  setPlaying(false);
                  preview(picked.name, t);
                }
              }}
            />
          </Label>
          <div className="flex gap-2">
            <Button size="sm" onClick={() => setPlaying(!playing)}>
              {playing ? "暂停预览" : "播放预览"}
            </Button>
            <Button size="sm" variant="outline" onClick={stop}>
              取消预览
            </Button>
            {!dynamic && (
              <Button
                size="sm"
                disabled={readOnly}
                onClick={() => {
                  try {
                    const frozen_bones = props.onCapturePose();
                    onChange({
                      frozen_bones,
                      frozen_nodes: undefined,
                      bone_overrides: {},
                      pose: "neutral",
                    });
                    stop();
                  } catch (cause) {
                    setError(
                      cause instanceof Error ? cause.message : "取帧失败",
                    );
                  }
                }}
              >
                使用当前帧为姿势
              </Button>
            )}
          </div>
        </div>
      )}
      <div className="space-y-2">
        <Label>
          导入独立动作
          <Input
            aria-label="导入独立动作"
            type="file"
            accept=".glb,.gltf,.fbx,.bvh,.json"
            disabled={readOnly || busy || !available}
            onChange={(event) => {
              const chosen = event.target.files?.[0];
              event.target.value = "";
              if (chosen) void choose(chosen);
            }}
          />
        </Label>
        <p className="text-xs text-muted-foreground">
          GLB、glTF、FBX、BVH或动作JSON；骨骼须兼容，FBX/BVH按厘米读取。
        </p>
        {busy && (
          <Button
            size="sm"
            variant="outline"
            onClick={() => {
              request.current?.abort();
              request.current = null;
              setBusy(false);
            }}
          >
            取消导入
          </Button>
        )}
        {names.length > 1 && (
          <Select
            value={pending?.name ?? ""}
            disabled={busy}
            onValueChange={(name) => {
              if (file) void choose(file, name);
            }}
          >
            <SelectTrigger aria-label="文件内动作">
              <SelectValue placeholder="选择动作" />
            </SelectTrigger>
            <SelectContent>
              {names.map((name) => (
                <SelectItem key={name} value={name}>
                  {name}
                </SelectItem>
              ))}
            </SelectContent>
          </Select>
        )}
        {pending && (
          <Button
            size="sm"
            disabled={readOnly}
            aria-label={`保存${pending.name}到我的动作`}
            title={pending.name}
            onClick={() => {
              try {
                if (pendingBaseline.current !== baseline)
                  throw new Error("角色已变化，请重新导入");
                onChange(
                  addDirectorMotionSource(
                    object,
                    pending,
                    available?.bones ?? [],
                  ),
                );
                setPending(null);
                setFile(null);
                setNames([]);
                setTab("mine");
              } catch (cause) {
                setError(
                  cause instanceof Error ? cause.message : "无法保存动作",
                );
              }
            }}
          >
            保存到我的动作
          </Button>
        )}
      </div>
      <div className="flex gap-2">
        <Input
          aria-label="姿势名称"
          value={poseName}
          onChange={(event) => setPoseName(event.target.value)}
          maxLength={128}
        />
        <Button
          size="sm"
          disabled={readOnly || !available?.rest || !poseName.trim()}
          onClick={() => void savePose()}
        >
          保存姿势为片段
        </Button>
      </div>
      <p className="text-xs text-muted-foreground">
        文字生成动作尚未接入可输出骨骼动画的服务。
      </p>
      {error && (
        <p role="alert" className="text-destructive">
          {error}
        </p>
      )}
    </div>
  );
}
