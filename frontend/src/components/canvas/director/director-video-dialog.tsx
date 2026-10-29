"use client";
import { useEffect, useRef, useState } from "react";
import { Film, Download } from "lucide-react";
import { uploadMediaAsset } from "@/api/media";
import { Progress } from "@/components/ui/progress";
import { Button } from "@/components/ui/button";
import { Input } from "@/components/ui/input";
import { Label } from "@/components/ui/label";
import { Checkbox } from "@/components/ui/checkbox";
import {
  Dialog,
  DialogContent,
  DialogTitle,
  DialogDescription,
} from "@/components/ui/dialog";
import {
  Select,
  SelectContent,
  SelectItem,
  SelectTrigger,
  SelectValue,
} from "@/components/ui/select";
import { directorFingerprint, type Director } from "./director-model";
import {
  activeDirectorScene,
  migrateDirectorDocument,
  replaceDirectorScene,
} from "./director-workspaces";
import { bindDirectorOutput } from "./director-capture";
import type {
  DirectorVideoExporter,
  DirectorVideoOptions,
} from "./director-video-output";
import type { CanvasYjsLiveSession } from "../yjs-live-session";
type Pending = {
  file: File;
  url: string;
  baseline: API.DirectorConfig;
  epoch: string;
  id: string;
  key: string;
  created: string;
  shotID: string;
  asset?: API.AssetSummary;
  next?: API.DirectorConfig;
};
function Choice({
  label,
  value,
  items,
  onChange,
  disabled,
}: {
  label: string;
  value: string;
  items: [string, string][];
  onChange: (v: string) => void;
  disabled: boolean;
}) {
  return (
    <Label>
      {label}
      <Select value={value} onValueChange={onChange} disabled={disabled}>
        <SelectTrigger aria-label={label}>
          <SelectValue />
        </SelectTrigger>
        <SelectContent>
          {items.map(([id, name]) => (
            <SelectItem key={id} value={id}>
              {name}
            </SelectItem>
          ))}
        </SelectContent>
      </Select>
    </Label>
  );
}
export function DirectorVideoDialog({
  client,
  nodeID,
  projectID,
  scene,
  exporter,
  disabled,
  onBusy,
  onCommit,
}: {
  client: CanvasYjsLiveSession;
  nodeID: string;
  projectID: string;
  scene: Director;
  exporter: DirectorVideoExporter | null;
  disabled: boolean;
  onCommit?: (
    next: API.DirectorConfig,
    baseline: API.DirectorConfig,
    epoch: string,
  ) => boolean | undefined;
  onBusy: (busy: boolean) => void;
}) {
  const shot = scene.shots.find((item) => item.id === scene.active_shot_id)!;
  const [open, setOpen] = useState(false),
    [options, setOptions] = useState<DirectorVideoOptions>({
      camera: shot.camera_id,
      duration: shot.duration,
      fps: shot.fps,
      longEdge: 1920,
      aspect:
        scene.aspect_ratio && scene.aspect_ratio !== "adaptive"
          ? scene.aspect_ratio
          : "16:9",
      quality: "high",
      clay: false,
    }),
    [busy, setBusy] = useState(false),
    [progress, setProgress] = useState(0),
    [error, setError] = useState(""),
    [pending, setPending] = useState<Pending | null>(null),
    [reviewed, setReviewed] = useState(false),
    [loaded, setLoaded] = useState(false);
  const controller = useRef<AbortController | null>(null),
    alive = useRef(true),
    active = useRef(false);
  useEffect(() => {
    alive.current = true;
    return () => {
      alive.current = false;
      controller.current?.abort();
    };
  }, []);
  const url = pending?.url;
  useEffect(
    () => () => {
      if (url) URL.revokeObjectURL(url);
    },
    [url],
  );
  function guard(baseline: API.DirectorConfig, epoch: string) {
    const view = client.getSnapshot();
    if (
      !alive.current ||
      client.getDocumentEpoch() !== epoch ||
      !view.writable ||
      view.migration ||
      directorFingerprint(
        view.document?.nodes.find((n) => n.id === nodeID)?.serviceData?.config
          ?.director,
      ) !== directorFingerprint(baseline)
    )
      throw new Error("场景或画布版本已变化，请重新导出");
  }
  async function run(publish: boolean) {
    if (active.current || disabled || !exporter) return;
    active.current = true;
    setBusy(true);
    onBusy(true);
    setError("");
    const request = new AbortController();
    controller.current = request;
    try {
      if (publish) {
        if (!pending || !reviewed || !loaded)
          throw new Error("请先查看并确认视频");
        if (pending.next) {
          guard(pending.next, pending.epoch);
          await client.waitForSaved(() => guard(pending.next!, pending.epoch));
          setPending(null);
          return;
        }
        guard(pending.baseline, pending.epoch);
        let asset = pending.asset;
        if (!asset) {
          const receipt = await uploadMediaAsset(
            { pid: projectID },
            { local_review_confirmed: true },
            pending.file,
            {
              signal: request.signal,
              headers: { "Idempotency-Key": pending.key },
            },
          );
          asset = receipt.asset;
          if (
            !asset?.id ||
            asset.project_id !== projectID ||
            asset.kind !== "video"
          )
            throw new Error("视频上传回执无效");
          if (alive.current) setPending({ ...pending, asset });
        }
        guard(pending.baseline, pending.epoch);
        const document = migrateDirectorDocument(pending.baseline);
        const next = replaceDirectorScene(
          document,
          bindDirectorOutput(
            activeDirectorScene(document),
            pending.shotID,
            {
              id: pending.id,
              asset_id: asset.id!,
              name: pending.file.name,
              created_at: pending.created,
              kind: "video",
            },
            asset,
            projectID,
          ),
        );
        if (onCommit) {
          if (!onCommit(next, pending.baseline, pending.epoch))
            throw new Error("镜头素材未写入场景，请处理恢复稿");
        } else
          client.changeDirector(nodeID, next, pending.baseline, pending.epoch);
        if (alive.current) setPending({ ...pending, asset, next });
        await client.waitForSaved(() => guard(next, pending.epoch));
        if (alive.current) setPending(null);
      } else {
        const baseline = client
            .getSnapshot()
            .document?.nodes.find((n) => n.id === nodeID)?.serviceData
            ?.config?.director,
          epoch = client.getDocumentEpoch();
        if (
          !baseline ||
          !epoch ||
          client.retainedDirector(nodeID) ||
          directorFingerprint(
            activeDirectorScene(migrateDirectorDocument(baseline)),
          ) !== directorFingerprint(scene)
        )
          throw new Error("请先保存当前场景");
        await client.waitForSaved(() => guard(baseline, epoch));
        setProgress(0);
        const blob = await exporter(options, request.signal, (value) => {
          guard(baseline, epoch);
          if (alive.current) setProgress(value);
        });
        guard(baseline, epoch);
        request.signal.throwIfAborted();
        const file = new File([blob], `${shot.name.slice(0, 100)}.webm`, {
          type: blob.type,
        });
        setPending({
          file,
          url: URL.createObjectURL(blob),
          baseline,
          epoch,
          id: crypto.randomUUID(),
          key: crypto.randomUUID(),
          created: new Date().toISOString(),
          shotID: shot.id,
        });
        setReviewed(false);
        setLoaded(false);
      }
    } catch (cause) {
      if (alive.current)
        setError(
          request.signal.aborted
            ? "已取消导出"
            : cause instanceof Error
              ? cause.message
              : "导出失败",
        );
    } finally {
      active.current = false;
      controller.current = null;
      if (alive.current) {
        setBusy(false);
        onBusy(false);
      }
    }
  }
  return (
    <>
      <Button
        variant="secondary"
        size="sm"
        disabled={disabled || !exporter || scene.mode !== "dynamic"}
        onClick={() => {
          if (!pending) {
            setOptions((previous) => ({
              ...previous,
              camera: shot.camera_id,
              duration: shot.duration,
              fps: shot.fps,
              aspect:
                scene.aspect_ratio && scene.aspect_ratio !== "adaptive"
                  ? scene.aspect_ratio
                  : "16:9",
            }));
            setError("");
          }
          setOpen(true);
        }}
      >
        <Film />
        导出视频
      </Button>
      <Dialog
        open={open}
        onOpenChange={(value) => {
          if (!busy) setOpen(value);
        }}
      >
        <DialogContent className="sm:max-w-xl" data-canvas-no-zoom>
          <DialogTitle>导出视频</DialogTitle>
          <DialogDescription>
            按指定帧率输出机位画面，查看成片后可保存到镜头素材。
          </DialogDescription>
          {pending ? (
            <>
              <video
                className="max-h-72 w-full"
                src={pending.url}
                controls
                onLoadedData={() => setLoaded(true)}
                onError={() => setError("视频无法播放，请重新导出")}
              />
              <Label>
                <Checkbox
                  checked={reviewed}
                  disabled={!loaded || busy}
                  onCheckedChange={(value) => setReviewed(value === true)}
                />
                已查看视频并确认保存
              </Label>
              <div className="flex gap-2">
                <Button variant="outline" asChild>
                  <a download={pending.file.name} href={pending.url}>
                    <Download />
                    下载 WebM
                  </a>
                </Button>
                <Button
                  disabled={busy || disabled || !reviewed || !loaded}
                  onClick={() => void run(true)}
                >
                  保存到镜头素材
                </Button>
                <Button
                  variant="ghost"
                  disabled={busy}
                  onClick={() => setPending(null)}
                >
                  重新导出
                </Button>
              </div>
            </>
          ) : (
            <>
              <div className="grid grid-cols-2 gap-4">
                <Choice
                  label="机位来源"
                  value={options.camera}
                  disabled={busy}
                  items={[
                    ["sequence", "按分镜顺序"],
                    ...scene.cameras.map((camera): [string, string] => [
                      camera.id,
                      camera.name,
                    ]),
                  ]}
                  onChange={(camera) =>
                    setOptions((previous) => ({
                      ...previous,
                      camera,
                      duration:
                        camera === "sequence"
                          ? scene.shots.reduce(
                              (total, item) => total + item.duration,
                              0,
                            )
                          : shot.duration,
                    }))
                  }
                />
                <Choice
                  label="画幅"
                  value={options.aspect}
                  disabled={busy}
                  items={["1:1", "4:3", "3:4", "16:9", "9:16", "2.39:1"].map(
                    (r) => [r, r],
                  )}
                  onChange={(aspect) => setOptions({ ...options, aspect })}
                />
                <Choice
                  label="长边像素"
                  value={String(options.longEdge)}
                  disabled={busy}
                  items={[
                    ["1280", "1280"],
                    ["1920", "1920"],
                  ]}
                  onChange={(v) =>
                    setOptions({ ...options, longEdge: Number(v) })
                  }
                />
                <Choice
                  label="帧率"
                  value={String(options.fps)}
                  disabled={busy}
                  items={[24, 25, 30, 50, 60].map((v) => [
                    String(v),
                    `${v} fps`,
                  ])}
                  onChange={(v) => setOptions({ ...options, fps: Number(v) })}
                />
                <Label>
                  时长（秒）
                  <Input
                    type="number"
                    min={0.1}
                    max={3600}
                    step={0.1}
                    value={options.duration}
                    disabled={busy}
                    onChange={(e) =>
                      setOptions({
                        ...options,
                        duration: Number(e.target.value),
                      })
                    }
                  />
                </Label>
                <Choice
                  label="清晰度"
                  value={options.quality}
                  disabled={busy}
                  items={[
                    ["standard", "标准"],
                    ["high", "高清"],
                  ]}
                  onChange={(quality) =>
                    setOptions({
                      ...options,
                      quality: quality as "standard" | "high",
                    })
                  }
                />
              </div>
              <Label>
                <Checkbox
                  checked={options.clay}
                  disabled={busy}
                  onCheckedChange={(value) =>
                    setOptions({ ...options, clay: value === true })
                  }
                />
                简化材质
              </Label>
              {busy ? (
                <div>
                  <Progress
                    className="w-full"
                    value={progress * 100}
                    aria-label="视频导出进度"
                  />
                  <p>{Math.round(progress * 100)}%</p>
                  <Button
                    variant="outline"
                    onClick={() => controller.current?.abort()}
                  >
                    取消导出
                  </Button>
                </div>
              ) : (
                <Button disabled={disabled} onClick={() => void run(false)}>
                  开始导出
                </Button>
              )}
            </>
          )}
          {error && (
            <p role="alert" className="text-destructive">
              {error}
            </p>
          )}
        </DialogContent>
      </Dialog>
    </>
  );
}
