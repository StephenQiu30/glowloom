"use client";
import { useEffect, useRef, useState } from "react";
import Image from "next/image";
import { Camera, ImageIcon, Star, Trash2, Download, Plus } from "lucide-react";
import {
  getMediaPreview,
  downloadMediaOriginal,
  uploadMediaAsset,
} from "@/api/media";
import { Button } from "@/components/ui/button";
import { Checkbox } from "@/components/ui/checkbox";
import { Label } from "@/components/ui/label";
import {
  Dialog,
  DialogContent,
  DialogDescription,
  DialogHeader,
  DialogTitle,
} from "@/components/ui/dialog";
import { ProjectMediaPreview } from "@/components/media/project-media";
import { isProjectID } from "@/components/projects/project";
import type { CanvasYjsLiveSession } from "../yjs-live-session";
import {
  readDirector,
  directorFingerprint,
  type Director,
} from "./director-model";
import {
  activeDirectorScene,
  migrateDirectorDocument,
  replaceDirectorScene,
} from "./director-workspaces";
import { addDirectorOutputs } from "./director-output-nodes";
// Saved gallery names may still carry the pre-transcode WebM extension.
// The downloaded response MIME describes the actual bytes, without renaming the shot.
const downloadExtensions: Record<string, string> = {
  "image/png": "png",
  "image/jpeg": "jpg",
  "image/webp": "webp",
  "image/gif": "gif",
  "video/mp4": "mp4",
  "video/webm": "webm",
  "video/quicktime": "mov",
};
function directorDownloadName(name: string, mime: string) {
  const extension = downloadExtensions[mime.split(";")[0].trim().toLowerCase()];
  return extension
    ? `${name.replace(/\.(?:png|jpe?g|webp|gif|mp4|webm|mov)$/i, "")}.${extension}`
    : name;
}
type Capture = {
  id: string;
  key: string;
  file: File;
  url: string;
  baseline: API.DirectorConfig;
  epoch: string;
  shotID: string;
  name: string;
  createdAt: string;
  asset?: API.AssetSummary;
  next?: API.DirectorConfig;
};
export function bindDirectorOutput(
  scene: Director,
  shotID: string,
  output: Required<Omit<API.DirectorScreenshot, "kind">> & { kind?: string },
  asset: API.AssetSummary,
  projectID: string,
): Director {
  if (
    !isProjectID(asset.id) ||
    asset.project_id !== projectID ||
    asset.kind !== (output.kind || "image") ||
    output.asset_id !== asset.id
  )
    throw new Error("图片上传回执不匹配");
  const next = readDirector(scene),
    shot = next.shots.find((s) => s.id === shotID);
  if (!shot) throw new Error("拍摄镜头已不存在");
  if (
    (shot.screenshots?.length ?? 0) >= 64 ||
    next.shots.reduce((total, s) => total + (s.screenshots?.length ?? 0), 0) >=
      512
  )
    throw new Error("镜头素材数量已达上限");
  if (
    next.shots.some((s) => s.screenshots?.some((item) => item.id === output.id))
  )
    throw new Error("拍摄结果已存在");
  shot.screenshots = [...(shot.screenshots ?? []), output];
  return readDirector(next);
}
export const bindDirectorCapture = bindDirectorOutput;
export function DirectorCapture({
  client,
  nodeID,
  projectID,
  scene,
  capture,
  disabled,
  onPause,
  onCommit,
  onError,
}: {
  client: CanvasYjsLiveSession;
  nodeID: string;
  projectID: string;
  scene: Director;
  capture: (() => Promise<Blob>) | null;
  disabled: boolean;
  onCommit?: (
    next: API.DirectorConfig,
    baseline: API.DirectorConfig,
    epoch: string,
  ) => boolean | undefined;
  onPause: () => void;
  onError: (message: string) => void;
}) {
  const [pending, setPending] = useState<Capture | null>(null),
    [busy, setBusy] = useState(false),
    [reviewed, setReviewed] = useState(false),
    [loaded, setLoaded] = useState(false),
    [error, setError] = useState("");
  const active = useRef(false),
    alive = useRef(true),
    requests = useRef(new Set<AbortController>());
  useEffect(() => {
    alive.current = true;
    const controllers = requests.current;
    return () => {
      alive.current = false;
      for (const controller of controllers) controller.abort();
    };
  }, []);
  const pendingURL = pending?.url;
  useEffect(() => {
    if (pendingURL) return () => URL.revokeObjectURL(pendingURL);
  }, [pendingURL]);
  function guard(baseline: API.DirectorConfig, epoch: string) {
    if (
      !alive.current ||
      client.getDocumentEpoch() !== epoch ||
      !client.getSnapshot().writable ||
      directorFingerprint(
        client.getSnapshot().document?.nodes.find((n) => n.id === nodeID)
          ?.serviceData?.config?.director,
      ) !== directorFingerprint(baseline)
    )
      throw new Error("场景或画布版本已变化，请重新拍摄");
  }
  async function shoot() {
    if (active.current || disabled || !capture) return;
    active.current = true;
    setBusy(true);
    setError("");
    onPause();
    try {
      const epoch = client.getDocumentEpoch();
      if (!epoch || client.retainedDirector(nodeID))
        throw new Error("请先处理尚未保存的场景编辑");
      const baseline = client
        .getSnapshot()
        .document?.nodes.find((n) => n.id === nodeID)?.serviceData
        ?.config?.director;
      if (
        !baseline ||
        directorFingerprint(
          activeDirectorScene(migrateDirectorDocument(baseline)),
        ) !== directorFingerprint(scene)
      )
        throw new Error("场景已变化，请重新拍摄");
      await client.waitForSaved(() => guard(baseline, epoch));
      guard(baseline, epoch);
      const blob = await capture();
      guard(baseline, epoch);
      if (blob.type !== "image/png" || !blob.size)
        throw new Error("机位图片无效");
      const shot = scene.shots.find((s) => s.id === scene.active_shot_id)!;
      const name = [...`${shot.name} 拍摄`].slice(0, 128).join("");
      setPending({
        id: crypto.randomUUID(),
        key: crypto.randomUUID(),
        file: new File([blob], `${name}.png`, { type: "image/png" }),
        url: URL.createObjectURL(blob),
        baseline,
        epoch,
        shotID: shot.id,
        name,
        createdAt: new Date().toISOString(),
      });
      setReviewed(false);
      setLoaded(false);
    } catch (cause) {
      onError(cause instanceof Error ? cause.message : "拍摄失败");
    } finally {
      active.current = false;
      if (alive.current) setBusy(false);
    }
  }
  async function publish() {
    if (active.current || !pending || !reviewed || !loaded) return;
    active.current = true;
    setBusy(true);
    setError("");
    const controller = new AbortController();
    requests.current.add(controller);
    try {
      if (pending.next) {
        guard(pending.next, pending.epoch);
        await client.waitForSaved(() => guard(pending.next!, pending.epoch));
        if (alive.current) setPending(null);
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
            signal: controller.signal,
            headers: { "Idempotency-Key": pending.key },
          },
        );
        asset = receipt.asset;
        if (
          !asset ||
          !isProjectID(asset.id) ||
          asset.project_id !== projectID ||
          asset.kind !== "image"
        )
          throw new Error("图片上传回执不匹配");
        if (alive.current)
          setPending((current) =>
            current?.id === pending.id ? { ...current, asset } : current,
          );
      }
      guard(pending.baseline, pending.epoch);
      const document = migrateDirectorDocument(pending.baseline);
      const next = replaceDirectorScene(
        document,
        bindDirectorCapture(
          activeDirectorScene(document),
          pending.shotID,
          {
            id: pending.id,
            asset_id: asset.id!,
            name: pending.name,
            created_at: pending.createdAt,
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
      if (alive.current)
        setPending((current) =>
          current?.id === pending.id ? { ...current, next } : current,
        );
      await client.waitForSaved(() => guard(next, pending.epoch));
      if (alive.current) setPending(null);
    } catch (cause) {
      if (alive.current)
        setError(cause instanceof Error ? cause.message : "拍摄结果未保存");
    } finally {
      requests.current.delete(controller);
      active.current = false;
      if (alive.current) setBusy(false);
    }
  }
  return (
    <>
      <Button
        variant="secondary"
        size="sm"
        disabled={disabled || busy || !capture}
        onClick={() => void shoot()}
      >
        <Camera />
        拍摄
      </Button>
      <Dialog
        open={!!pending}
        onOpenChange={(open) => {
          if (!open && !busy) setPending(null);
        }}
      >
        <DialogContent
          data-canvas-no-zoom
          onMouseDown={(event) => event.stopPropagation()}
          className="max-w-3xl"
        >
          <DialogHeader>
            <DialogTitle>检查机位图片</DialogTitle>
            <DialogDescription>
              确认当前机位画面、素材使用权限，且不含需要授权的真人素材后保存到镜头素材。
            </DialogDescription>
          </DialogHeader>
          {pending && (
            <Image
              src={pending.url}
              width={1920}
              height={1080}
              unoptimized
              alt="实际机位拍摄图片"
              className="max-h-[55vh] w-full object-contain"
              onLoad={(event) =>
                setLoaded(
                  event.currentTarget.naturalWidth > 0 &&
                    event.currentTarget.naturalHeight > 0,
                )
              }
              onError={() => {
                setLoaded(false);
                setError("拍摄图片无法显示");
              }}
            />
          )}
          {error && (
            <p role="alert" className="text-sm text-destructive">
              {error}
            </p>
          )}
          <Label className="flex items-center gap-2">
            <Checkbox
              checked={reviewed}
              disabled={!loaded || busy}
              onCheckedChange={(value) => setReviewed(value === true)}
            />
            我已检查画面和素材使用权限
          </Label>
          <Button
            disabled={busy || !loaded || !reviewed}
            onClick={() => void publish()}
          >
            {busy
              ? "正在保存…"
              : pending?.next
                ? "重新确认保存结果"
                : "保存到镜头素材"}
          </Button>
        </DialogContent>
      </Dialog>
    </>
  );
}
export function DirectorGallery({
  client,
  nodeID,
  scene,
  projectID,
  disabled,
  onChange,
}: {
  client?: CanvasYjsLiveSession;
  nodeID?: string;
  scene: Director;
  projectID: string;
  disabled: boolean;
  onChange: (scene: Director) => void;
}) {
  const [open, setOpen] = useState(false);
  const shot = scene.shots.find((s) => s.id === scene.active_shot_id)!;
  const [selection, setSelection] = useState<string[]>([]),
    [busy, setBusy] = useState(false),
    [error, setError] = useState("");
  const request = useRef<AbortController | null>(null),
    nodeIDs = useRef(new Map<string, string>());
  useEffect(() => () => request.current?.abort(), []);
  const chosen = (shot.screenshots ?? []).filter((image) =>
    selection.includes(image.id!),
  );
  async function act(add: boolean) {
    if (busy || !chosen.length || (add && disabled)) return;
    const controller = new AbortController();
    request.current = controller;
    setBusy(true);
    setError("");
    try {
      const baseline = client
          ?.getSnapshot()
          .document?.nodes.find((node) => node.id === nodeID)?.serviceData
          ?.config?.director,
        epoch = client?.getDocumentEpoch();
      if (add) {
        if (!client || !nodeID || !baseline || !epoch)
          throw new Error("画布当前不可写");
        const assets = await Promise.all(
          chosen.map(async (image) => {
            const preview = await getMediaPreview(
              { pid: projectID, asset_id: image.asset_id! },
              { signal: controller.signal },
            );
            if (
              !preview.asset ||
              preview.asset.id !== image.asset_id ||
              preview.asset.kind !== (image.kind || "image")
            )
              throw new Error("素材读取失败");
            if (!nodeIDs.current.has(image.id!))
              nodeIDs.current.set(image.id!, crypto.randomUUID());
            return {
              asset: preview.asset,
              nodeID: nodeIDs.current.get(image.id!)!,
            };
          }),
        );
        controller.signal.throwIfAborted();
        addDirectorOutputs(client, nodeID, projectID, baseline, epoch, assets);
        await client.waitForSaved(() => {
          if (client.getDocumentEpoch() !== epoch)
            throw new Error("画布版本已变化");
        });
        setSelection([]);
      } else
        for (const image of chosen) {
          const blob = await downloadMediaOriginal(
            { pid: projectID, asset_id: image.asset_id! },
            { signal: controller.signal, responseType: "blob" },
          );
          controller.signal.throwIfAborted();
          const url = URL.createObjectURL(blob),
            link = document.createElement("a");
          link.href = url;
          link.download = directorDownloadName(
            image.name || "镜头素材",
            blob.type,
          );
          link.click();
          setTimeout(() => URL.revokeObjectURL(url), 1000);
        }
    } catch (cause) {
      if (!controller.signal.aborted)
        setError(cause instanceof Error ? cause.message : "操作失败");
    } finally {
      if (!controller.signal.aborted) setBusy(false);
      request.current = null;
    }
  }
  return (
    <>
      <Button variant="ghost" size="sm" onClick={() => setOpen(true)}>
        <ImageIcon />
        镜头素材
      </Button>
      <Dialog open={open} onOpenChange={setOpen}>
        <DialogContent
          data-canvas-no-zoom
          onMouseDown={(event) => event.stopPropagation()}
          className="max-w-4xl"
        >
          <DialogHeader>
            <DialogTitle>{shot.name} · 镜头素材</DialogTitle>
            <DialogDescription>
              已保存的机位图片与视频，可下载或添加到画布。
            </DialogDescription>
          </DialogHeader>
          <div className="flex items-center gap-2">
            <Label>
              <Checkbox
                checked={
                  !!shot.screenshots?.length &&
                  chosen.length === shot.screenshots.length
                }
                disabled={busy || !shot.screenshots?.length}
                onCheckedChange={(value) =>
                  setSelection(
                    value ? shot.screenshots!.map((image) => image.id!) : [],
                  )
                }
              />
              全选
            </Label>
            <Button
              variant="outline"
              disabled={busy || !chosen.length}
              onClick={() => void act(false)}
            >
              <Download />
              下载
            </Button>
            <Button
              disabled={disabled || busy || !chosen.length || !client}
              onClick={() => void act(true)}
            >
              <Plus />
              添加到画布
            </Button>
          </div>
          {error && (
            <p role="alert" className="text-destructive">
              {error}
            </p>
          )}
          <div className="grid max-h-[65vh] grid-cols-2 gap-4 overflow-auto">
            {shot.screenshots?.map((image) => (
              <div key={image.id} className="space-y-2">
                <Label>
                  <Checkbox
                    aria-label={`选择 ${image.name}`}
                    checked={selection.includes(image.id!)}
                    disabled={busy}
                    onCheckedChange={(value) =>
                      setSelection(
                        value
                          ? [...selection, image.id!]
                          : selection.filter((id) => id !== image.id),
                      )
                    }
                  />
                  {image.name}
                </Label>
                <div className="aspect-video overflow-hidden rounded-md bg-[var(--canvas-director-background)]">
                  <ProjectMediaPreview
                    projectID={projectID}
                    id={image.asset_id!}
                    kind={image.kind === "video" ? "video" : "image"}
                    large
                  />
                </div>
                <div className="flex items-center gap-2">
                  <span className="min-w-0 flex-1 truncate text-xs">
                    {image.name}
                  </span>
                  <Button
                    size="icon-sm"
                    variant="ghost"
                    aria-label={`设为封面 ${image.name}`}
                    aria-pressed={scene.cover?.asset_id === image.asset_id}
                    disabled={disabled || image.kind === "video"}
                    onClick={() =>
                      onChange({
                        ...scene,
                        cover: { shot_id: shot.id, asset_id: image.asset_id },
                      })
                    }
                  >
                    <Star />
                  </Button>
                  <Button
                    size="icon-sm"
                    variant="ghost"
                    aria-label={`移除素材 ${image.name}`}
                    disabled={disabled}
                    onClick={() => {
                      const next = readDirector(scene);
                      next.shots.find((s) => s.id === shot.id)!.screenshots =
                        shot.screenshots?.filter((s) => s.id !== image.id);
                      if (
                        next.cover?.shot_id === shot.id &&
                        next.cover.asset_id === image.asset_id
                      )
                        delete next.cover;
                      onChange(next);
                    }}
                  >
                    <Trash2 />
                  </Button>
                </div>
              </div>
            ))}
          </div>
          {!shot.screenshots?.length && (
            <p className="py-10 text-center text-sm text-muted-foreground">
              还没有拍摄的镜头素材
            </p>
          )}
        </DialogContent>
      </Dialog>
    </>
  );
}
