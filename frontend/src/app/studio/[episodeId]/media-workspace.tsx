"use client";

import { FileUp, LoaderCircle, RefreshCw, ShieldCheck } from "lucide-react";
import { type FormEvent, useState } from "react";

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

export function MediaWorkspace({
  media,
  busy,
  onUpload,
  onRetry,
}: {
  media: API.MediaVersionResponse[];
  busy: boolean;
  onUpload: (file: File, kind: API.UploadDeclaration["kind"]) => Promise<boolean>;
  onRetry: (version: API.MediaVersionResponse) => Promise<void>;
}) {
  const [selectedFile, setSelectedFile] = useState<File | null>(null);
  const [kind, setKind] = useState<API.UploadDeclaration["kind"]>("image");

  async function submit(event: FormEvent<HTMLFormElement>) {
    event.preventDefault();
    if (!selectedFile) return;
    const form = event.currentTarget;
    if (!(await onUpload(selectedFile, kind))) return;
    setSelectedFile(null);
    form.reset();
  }

  return (
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
                onChange={(event) => setKind(event.target.value as API.UploadDeclaration["kind"])}
              >
                {Object.entries(mediaKindLabels).map(([value, label]) => (
                  <option key={value} value={value}>{label}</option>
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
              {busy ? <LoaderCircle className="animate-spin" aria-hidden="true" /> : <FileUp aria-hidden="true" />}
              上传并开始探测
            </Button>
          </form>
        </CardContent>
      </Card>

      <Card>
        <CardHeader>
          <CardTitle>媒体版本</CardTitle>
          <CardDescription>{media.length} 个 Workspace 私有版本</CardDescription>
        </CardHeader>
        <CardContent className="grid gap-3">
          {media.length === 0 ? (
            <div className="rounded-xl border border-dashed border-slate-200 px-6 py-12 text-center">
              <p className="font-medium">还没有媒体</p>
              <p className="mt-1 text-sm text-slate-500">上传角色图、场景图或声音样本后会显示在这里。</p>
            </div>
          ) : (
            media.map((version) => (
              <article className="flex flex-wrap items-center gap-4 rounded-xl border border-slate-200 p-4" key={version.id}>
                <div className="min-w-0 flex-1">
                  <div className="flex flex-wrap items-center gap-2">
                    <h3 className="truncate font-medium">{version.filename}</h3>
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
                    v{version.version_no} · {version.mime_type} · {(version.size_bytes / 1024).toFixed(1)} KB
                    {version.width && version.height ? ` · ${version.width}×${version.height}` : ""}
                  </p>
                  {version.probe_error_summary ? (
                    <p className="mt-2 text-sm text-rose-700">{version.probe_error_summary}</p>
                  ) : null}
                </div>
                {version.probe_status === "failed" ? (
                  <Button disabled={busy} size="sm" variant="outline" onClick={() => onRetry(version)}>
                    <RefreshCw aria-hidden="true" />重试探测
                  </Button>
                ) : null}
              </article>
            ))
          )}
        </CardContent>
      </Card>
    </div>
  );
}
