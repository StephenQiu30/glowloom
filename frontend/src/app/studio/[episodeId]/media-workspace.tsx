"use client";

import {
  Archive,
  ArrowRightLeft,
  FileUp,
  HardDrive,
  LoaderCircle,
  Plus,
  RefreshCw,
  RotateCcw,
  ShieldCheck,
} from "lucide-react";
import { type FormEvent, useMemo, useState } from "react";

import { Alert, AlertDescription, AlertTitle } from "@/components/ui/alert";
import { Badge } from "@/components/ui/badge";
import { Button } from "@/components/ui/button";
import {
  Card,
  CardContent,
  CardDescription,
  CardHeader,
  CardTitle,
} from "@/components/ui/card";
import {
  Dialog,
  DialogContent,
  DialogDescription,
  DialogFooter,
  DialogHeader,
  DialogTitle,
} from "@/components/ui/dialog";
import { Label } from "@/components/ui/label";

import { mediaKindFromFile } from "./episode-studio-model";

const mediaKindLabels: Record<API.UploadDeclaration["kind"], string> = {
  image: "图片",
  video: "视频",
  audio: "音频",
  subtitle: "字幕",
  delivery: "交付文件",
};

const probeLabels: Record<API.MediaVersionResponse["probe_status"], string> = {
  pending: "探测中",
  ready: "可用",
  failed: "探测失败",
  quarantined: "已隔离",
};

const locationLabels: Record<API.MediaLocationResponse["status"], string> = {
  verified: "已校验",
  active: "当前读取",
  retiring: "回滚保护中",
  retired: "已退役",
  quarantined: "已隔离",
};

type MediaObjectView = {
  current: API.MediaVersionResponse;
  versions: API.MediaVersionResponse[];
};

function groupMediaObjects(media: API.MediaVersionResponse[]): MediaObjectView[] {
  const grouped = new Map<string, API.MediaVersionResponse[]>();
  for (const version of media) {
    const versions = grouped.get(version.media_object_id) ?? [];
    versions.push(version);
    grouped.set(version.media_object_id, versions);
  }
  return [...grouped.values()].map((versions) => {
    versions.sort((left, right) => right.version_no - left.version_no);
    const currentId = versions[0]?.media_object_current_version_id;
    return {
      current: versions.find((version) => version.id === currentId) ?? versions[0],
      versions,
    };
  });
}

function inputAccept(kind: API.UploadDeclaration["kind"]): string | undefined {
  if (kind === "image" || kind === "video" || kind === "audio") return `${kind}/*`;
  if (kind === "subtitle") return ".srt,.vtt,text/vtt,application/x-subrip";
  return undefined;
}

export function MediaWorkspace({
  media,
  busy,
  locations = [],
  locationBusy = false,
  locationVersionId = null,
  onUpload,
  onAppendVersion,
  onRetry,
  onSetCurrent,
  onToggleArchived,
  onOpenLocations,
  onCloseLocations,
  onLocationMigration,
  onLocationRollback,
}: {
  media: API.MediaVersionResponse[];
  busy: boolean;
  locations?: API.MediaLocationResponse[];
  locationBusy?: boolean;
  locationVersionId?: string | null;
  onUpload: (file: File, kind: API.UploadDeclaration["kind"]) => Promise<boolean>;
  onAppendVersion: (
    current: API.MediaVersionResponse,
    file: File,
  ) => Promise<boolean>;
  onRetry: (version: API.MediaVersionResponse) => Promise<void>;
  onSetCurrent: (version: API.MediaVersionResponse) => Promise<void>;
  onToggleArchived: (current: API.MediaVersionResponse) => Promise<void>;
  onOpenLocations?: (version: API.MediaVersionResponse) => void;
  onCloseLocations?: () => void;
  onLocationMigration?: (
    version: API.MediaVersionResponse,
    activeLocationId: string,
  ) => Promise<void>;
  onLocationRollback?: (
    version: API.MediaVersionResponse,
    targetLocationId: string,
    activeLocationId: string,
  ) => Promise<void>;
}) {
  const [selectedFile, setSelectedFile] = useState<File | null>(null);
  const [kind, setKind] = useState<API.UploadDeclaration["kind"]>("image");
  const [appendTarget, setAppendTarget] = useState<API.MediaVersionResponse | null>(null);
  const [appendFile, setAppendFile] = useState<File | null>(null);
  const objects = useMemo(() => groupMediaObjects(media), [media]);
  const locationVersion = media.find((version) => version.id === locationVersionId);
  const activeLocation = locations.find((location) => location.status === "active");

  async function submit(event: FormEvent<HTMLFormElement>) {
    event.preventDefault();
    if (!selectedFile) return;
    const form = event.currentTarget;
    if (!(await onUpload(selectedFile, kind))) return;
    setSelectedFile(null);
    form.reset();
  }

  async function submitVersion(event: FormEvent<HTMLFormElement>) {
    event.preventDefault();
    if (!appendTarget || !appendFile) return;
    if (!(await onAppendVersion(appendTarget, appendFile))) return;
    setAppendTarget(null);
    setAppendFile(null);
  }

  return (
    <>
      <div className="grid gap-6 xl:grid-cols-[380px_minmax(0,1fr)]">
        <Card className="h-fit">
          <CardHeader>
            <CardTitle>私有上传</CardTitle>
            <CardDescription>
              文件通过短期签名 URL 直接写入本机 MinIO，完成后由服务端创建不可变媒体版本。
            </CardDescription>
          </CardHeader>
          <CardContent>
            <form className="grid gap-5" onSubmit={submit}>
              <div className="grid gap-2">
                <Label htmlFor="mediaFile">选择媒体文件</Label>
                <input
                  className="block w-full rounded-lg border border-slate-200 bg-white p-2 text-sm file:mr-3 file:rounded-md file:border-0 file:bg-slate-100 file:px-3 file:py-1.5 file:text-sm file:font-medium"
                  id="mediaFile"
                  type="file"
                  required
                  onChange={(event) => {
                    const file = event.target.files?.[0] ?? null;
                    setSelectedFile(file);
                    if (file) setKind(mediaKindFromFile(file));
                  }}
                />
              </div>
              <div className="grid gap-2">
                <Label htmlFor="mediaKind">媒体类型</Label>
                <select
                  className="h-9 rounded-lg border border-slate-200 bg-white px-3 text-sm"
                  id="mediaKind"
                  value={kind}
                  onChange={(event) =>
                    setKind(event.target.value as API.UploadDeclaration["kind"])
                  }
                >
                  {Object.entries(mediaKindLabels).map(([value, label]) => (
                    <option key={value} value={value}>
                      {label}
                    </option>
                  ))}
                </select>
              </div>
              {selectedFile ? (
                <Alert className="border-cyan-100 bg-cyan-50 text-[#087f91]">
                  <ShieldCheck aria-hidden="true" />
                  <AlertTitle>{selectedFile.name}</AlertTitle>
                  <AlertDescription className="text-[#087f91]/80">
                    {(selectedFile.size / 1024 / 1024).toFixed(2)} MB · 上传前在浏览器计算 SHA-256
                  </AlertDescription>
                </Alert>
              ) : null}
              <Button disabled={busy || !selectedFile} type="submit">
                {busy ? (
                  <LoaderCircle className="animate-spin" aria-hidden="true" />
                ) : (
                  <FileUp aria-hidden="true" />
                )}
                上传并开始探测
              </Button>
            </form>
          </CardContent>
        </Card>

        <Card>
          <CardHeader>
            <CardTitle>媒体库</CardTitle>
            <CardDescription>
              {objects.length} 个媒体对象 · {media.length} 个 Workspace 私有版本
            </CardDescription>
          </CardHeader>
          <CardContent className="grid gap-4">
            {objects.length === 0 ? (
              <div className="rounded-xl border border-dashed border-slate-200 px-6 py-12 text-center">
                <p className="font-medium">还没有媒体</p>
                <p className="mt-1 text-sm text-slate-500">
                  上传角色图、场景图或声音样本后会显示在这里。
                </p>
              </div>
            ) : (
              objects.map(({ current, versions }) => {
                const archived = current.media_object_status === "archived";
                return (
                  <article
                    className="rounded-xl border border-slate-200 p-4"
                    key={current.media_object_id}
                  >
                    <div className="flex flex-wrap items-start justify-between gap-3">
                      <div className="min-w-0">
                        <div className="flex flex-wrap items-center gap-2">
                          <h3 className="truncate font-medium">{current.filename}</h3>
                          <Badge variant="secondary">
                            当前版本 v{current.version_no}
                          </Badge>
                          {archived ? <Badge variant="outline">已归档</Badge> : null}
                        </div>
                        <p className="mt-1 text-xs text-slate-500">
                          {mediaKindLabels[current.media_object_kind]} · {versions.length} 个不可变版本
                        </p>
                      </div>
                      <div className="flex flex-wrap gap-2">
                        {!archived ? (
                          <Button
                            aria-label="追加媒体版本"
                            disabled={busy}
                            size="sm"
                            variant="outline"
                            onClick={() => {
                              setAppendTarget(current);
                              setAppendFile(null);
                            }}
                          >
                            <Plus aria-hidden="true" />追加版本
                          </Button>
                        ) : null}
                        <Button
                          aria-label={archived ? "恢复媒体" : "归档媒体"}
                          disabled={busy}
                          size="sm"
                          variant="outline"
                          onClick={() => void onToggleArchived(current)}
                        >
                          {archived ? (
                            <RotateCcw aria-hidden="true" />
                          ) : (
                            <Archive aria-hidden="true" />
                          )}
                          {archived ? "恢复" : "归档"}
                        </Button>
                      </div>
                    </div>

                    <div className="mt-4 grid gap-2">
                      {versions.map((version) => {
                        const isCurrent = version.id === current.id;
                        return (
                          <div
                            className="flex flex-wrap items-center gap-3 rounded-lg bg-slate-50 px-3 py-3"
                            key={version.id}
                          >
                            <div className="min-w-0 flex-1">
                              <div className="flex flex-wrap items-center gap-2">
                                <p className="truncate text-sm font-medium">
                                  v{version.version_no} · {version.filename}
                                </p>
                                <Badge
                                  className={
                                    version.probe_status === "ready"
                                      ? "border-emerald-200 bg-emerald-50 text-emerald-700"
                                      : version.probe_status === "pending"
                                        ? "border-cyan-200 bg-cyan-50 text-[#087f91]"
                                        : "border-rose-200 bg-rose-50 text-rose-700"
                                  }
                                  variant="outline"
                                >
                                  {probeLabels[version.probe_status]}
                                </Badge>
                              </div>
                              <p className="mt-1 text-xs text-slate-500">
                                {version.mime_type} · {(version.size_bytes / 1024).toFixed(1)} KB
                                {version.width && version.height
                                  ? ` · ${version.width}×${version.height}`
                                  : ""}
                              </p>
                              {version.probe_error_summary ? (
                                <p className="mt-2 text-sm text-rose-700">
                                  {version.probe_error_summary}
                                </p>
                              ) : null}
                            </div>
                            {!isCurrent && !archived ? (
                              <Button
                                aria-label={`设为当前媒体版本 v${version.version_no}`}
                                disabled={busy}
                                size="sm"
                                variant="outline"
                                onClick={() => void onSetCurrent(version)}
                              >
                                设为当前
                              </Button>
                            ) : null}
                            {version.probe_status === "failed" ? (
                              <Button
                                disabled={busy}
                                size="sm"
                                variant="outline"
                                onClick={() => void onRetry(version)}
                              >
                                <RefreshCw aria-hidden="true" />重试探测
                              </Button>
                            ) : null}
                            {onOpenLocations ? (
                              <Button
                                aria-label={`管理媒体版本 v${version.version_no} 的存储位置`}
                                disabled={busy}
                                size="sm"
                                variant="outline"
                                onClick={() => onOpenLocations(version)}
                              >
                                <HardDrive aria-hidden="true" />存储位置
                              </Button>
                            ) : null}
                          </div>
                        );
                      })}
                    </div>
                  </article>
                );
              })
            )}
          </CardContent>
        </Card>
      </div>

      <Dialog
        open={Boolean(appendTarget)}
        onOpenChange={(open) => {
          if (!open) {
            setAppendTarget(null);
            setAppendFile(null);
          }
        }}
      >
        <DialogContent>
          <DialogHeader>
            <DialogTitle>追加媒体版本</DialogTitle>
            <DialogDescription>
              新文件会创建不可变版本并设为 current；已有版本、引用和探测结果不会被改写。
            </DialogDescription>
          </DialogHeader>
          <form className="grid gap-5" onSubmit={submitVersion}>
            <div className="grid gap-2">
              <Label htmlFor="appendMediaFile">选择新的媒体文件</Label>
              <input
                accept={appendTarget ? inputAccept(appendTarget.media_object_kind) : undefined}
                className="block w-full rounded-lg border border-slate-200 bg-white p-2 text-sm file:mr-3 file:rounded-md file:border-0 file:bg-slate-100 file:px-3 file:py-1.5 file:text-sm file:font-medium"
                id="appendMediaFile"
                required
                type="file"
                onChange={(event) => setAppendFile(event.target.files?.[0] ?? null)}
              />
              <p className="text-xs text-slate-500">
                必须保持为{appendTarget ? mediaKindLabels[appendTarget.media_object_kind] : "原"}类型；上传完成后自动创建探测任务。
              </p>
            </div>
            <DialogFooter>
              <Button
                type="button"
                variant="outline"
                onClick={() => {
                  setAppendTarget(null);
                  setAppendFile(null);
                }}
              >
                取消
              </Button>
              <Button disabled={busy || !appendFile} type="submit">
                {busy ? <LoaderCircle className="animate-spin" aria-hidden="true" /> : null}
                上传为新版本
              </Button>
            </DialogFooter>
          </form>
        </DialogContent>
      </Dialog>

      <Dialog
        open={Boolean(locationVersion)}
        onOpenChange={(open) => {
          if (!open) onCloseLocations?.();
        }}
      >
        <DialogContent>
          <DialogHeader>
            <DialogTitle>存储位置治理</DialogTitle>
            <DialogDescription>
              {locationVersion?.filename ?? "媒体版本"} 的业务 ID 与内容 hash 不会变化；只有校验通过的位置才能成为当前读取位置。
            </DialogDescription>
          </DialogHeader>

          <div className="grid gap-3">
            {locationBusy && locations.length === 0 ? (
              <div className="flex items-center justify-center gap-2 rounded-xl border border-dashed border-slate-200 px-4 py-8 text-sm text-slate-500">
                <LoaderCircle className="animate-spin" aria-hidden="true" />
                正在读取位置状态
              </div>
            ) : locations.length === 0 ? (
              <Alert variant="destructive">
                <AlertTitle>位置状态不可用</AlertTitle>
                <AlertDescription>
                  当前没有可展示的位置事实，请稍后刷新或检查任务中心。
                </AlertDescription>
              </Alert>
            ) : (
              locations.map((location, index) => {
                const rollbackAvailable = location.rollback_available;
                return (
                  <div
                    className="rounded-xl border border-slate-200 bg-slate-50 px-4 py-3"
                    key={location.id}
                  >
                    <div className="flex flex-wrap items-center justify-between gap-3">
                      <div>
                        <div className="flex items-center gap-2">
                          <p className="text-sm font-medium">位置 {index + 1}</p>
                          <Badge
                            className={
                              location.status === "active"
                                ? "border-emerald-200 bg-emerald-50 text-emerald-700"
                                : location.status === "retiring"
                                  ? "border-amber-200 bg-amber-50 text-amber-700"
                                  : undefined
                            }
                            variant="outline"
                          >
                            {locationLabels[location.status]}
                          </Badge>
                        </div>
                        <p className="mt-1 text-xs text-slate-500">
                          {location.retire_after
                            ? `保护至 ${new Date(location.retire_after).toLocaleString("zh-CN")}`
                            : location.verified_at
                              ? `校验于 ${new Date(location.verified_at).toLocaleString("zh-CN")}`
                              : "等待完整性校验"}
                        </p>
                      </div>
                      {rollbackAvailable &&
                      activeLocation &&
                      locationVersion &&
                      onLocationRollback ? (
                        <Button
                          disabled={locationBusy}
                          size="sm"
                          variant="outline"
                          onClick={() =>
                            void onLocationRollback(
                              locationVersion,
                              location.id,
                              activeLocation.id,
                            )
                          }
                        >
                          <RotateCcw aria-hidden="true" />回滚到此位置
                        </Button>
                      ) : null}
                    </div>
                  </div>
                );
              })
            )}
          </div>

          <DialogFooter>
            <Button type="button" variant="outline" onClick={onCloseLocations}>
              关闭
            </Button>
            {locationVersion && activeLocation && onLocationMigration ? (
              <Button
                disabled={locationBusy}
                onClick={() =>
                  void onLocationMigration(locationVersion, activeLocation.id)
                }
              >
                {locationBusy ? (
                  <LoaderCircle className="animate-spin" aria-hidden="true" />
                ) : (
                  <ArrowRightLeft aria-hidden="true" />
                )}
                迁移当前版本
              </Button>
            ) : null}
          </DialogFooter>
        </DialogContent>
      </Dialog>
    </>
  );
}
