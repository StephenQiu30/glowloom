"use client";

import { useEffect, useRef, useState } from "react";
import { ArrowLeft, FileImage, Upload, Video } from "lucide-react";
import { cn } from "cn";
import { previewLibraryMediaAsset } from "@/api/media";
import { useOptionalSession } from "@/components/auth/session";
import { useLoginDialog } from "@/components/layout/basic-layout";
import {
  mediaError,
  requirePreview,
  validateUpload,
  type LibraryItem,
} from "@/components/media/library";
import { Alert, AlertDescription } from "@/components/ui/alert";
import { Button } from "@/components/ui/button";
import {
  Dialog,
  DialogContent,
  DialogDescription,
  DialogHeader,
  DialogTitle,
} from "@/components/ui/dialog";
import { Input } from "@/components/ui/input";
import { getScopeGeneration, getSessionGeneration } from "@/lib/request";
import { ToolAssetPicker } from "./tool-asset-picker";
import type { ImportTool, MediaKind } from "./catalog";

const formats: Record<MediaKind, readonly string[]> = {
  image: [".png", ".jpg", ".jpeg", ".webp"],
  video: [".mp4", ".webm", ".mov"],
};

type Source = { kind: MediaKind; title: string; url: string; local?: true };

export function ToolImportDialog({
  tool,
  onClose,
  onCloseAutoFocus,
}: {
  tool: ImportTool;
  onClose: () => void;
  onCloseAutoFocus: () => void;
}) {
  const actorID = useOptionalSession()?.actor.id;
  const login = useLoginDialog();
  const [picking, setPicking] = useState(false);
  const [source, setSource] = useState<Source | null>(null);
  const [error, setError] = useState("");
  const input = useRef<HTMLInputElement>(null);
  const previewRequest = useRef<AbortController | null>(null);
  const localURL = source?.local ? source.url : undefined;
  useEffect(
    () => () => {
      if (localURL) URL.revokeObjectURL(localURL);
    },
    [localURL],
  );
  useEffect(() => () => previewRequest.current?.abort(), []);

  function readLocal(files: FileList | null) {
    if (!files?.length) return;
    if (files.length !== 1) {
      setError("请一次选择一个素材。");
      return;
    }
    const file = files[0];
    const name = file.name.toLowerCase();
    const kind = tool.media.find(
      (kind) =>
        formats[kind].some((extension) => name.endsWith(extension)) &&
        (!file.type ||
          file.type === "application/octet-stream" ||
          file.type.startsWith(kind + "/")),
    );
    const invalid = kind ? validateUpload(file) : "文件类型与此工具不匹配。";
    if (invalid || !kind) {
      setError(invalid);
      return;
    }
    setError("");
    setSource({
      kind,
      title: file.name,
      url: URL.createObjectURL(file),
      local: true,
    });
  }

  async function chooseAsset(item: LibraryItem, kind: MediaKind) {
    const controller = new AbortController();
    previewRequest.current?.abort();
    previewRequest.current = controller;
    const scope = getScopeGeneration(),
      session = getSessionGeneration();
    try {
      const id = item.asset_id;
      if (!id || !item.media || !tool.media.includes(kind))
        throw new Error("invalid_asset");
      const result = await previewLibraryMediaAsset(
        { item_id: item.id, scope: "personal" },
        { signal: controller.signal },
      );
      if (
        controller.signal.aborted ||
        scope !== getScopeGeneration() ||
        session !== getSessionGeneration()
      )
        return;
      if (
        result.asset?.kind !== kind ||
        result.asset.revision !== item.media.revision
      )
        throw new Error("stale_asset");
      setSource({ kind, title: item.title, url: requirePreview(result, id) });
      setError("");
      setPicking(false);
    } catch (reason) {
      if (!controller.signal.aborted) throw new Error(mediaError(reason));
    }
  }

  function backToImport() {
    previewRequest.current?.abort();
    setPicking(false);
  }

  return (
    <Dialog
      open
      onOpenChange={(open) => {
        if (!open) onClose();
      }}
    >
      <DialogContent
        className={cn(
          "max-h-dialog overflow-y-auto",
          picking ? "sm:max-w-2xl" : "sm:max-w-[27.5rem]",
        )}
        onCloseAutoFocus={(event) => {
          event.preventDefault();
          onCloseAutoFocus();
        }}
      >
        <DialogHeader>
          <DialogTitle>{picking ? "资产库" : tool.name}</DialogTitle>
          <DialogDescription>
            {picking ? "选择一个与当前工具匹配的素材。" : tool.description}
          </DialogDescription>
        </DialogHeader>
        {picking && actorID ? (
          <>
            <Button
              variant="ghost"
              size="sm"
              onClick={backToImport}
              className="justify-self-start"
            >
              <ArrowLeft data-icon="inline-start" /> 返回{tool.name}
            </Button>
            <ToolAssetPicker
              media={tool.media}
              actorID={actorID}
              onConfirm={chooseAsset}
            />
          </>
        ) : (
          <>
            {source ? (
              <div className="flex flex-col gap-2">
                {source.kind === "image" ? (
                  /* An authorized, expiring media URL or a local blob is rendered directly. */
                  // eslint-disable-next-line @next/next/no-img-element
                  <img
                    src={source.url}
                    alt={source.title}
                    className="max-h-48 w-full rounded-md bg-muted object-contain"
                    onError={() => setError("无法预览此图片，请更换素材。")}
                  />
                ) : (
                  <video
                    src={source.url}
                    controls
                    playsInline
                    preload="metadata"
                    aria-label={source.title}
                    className="max-h-48 w-full rounded-md bg-muted"
                    onError={() => setError("无法预览此视频，请更换素材。")}
                  />
                )}
                <div className="flex min-w-0 items-center justify-between gap-2">
                  <span className="truncate text-sm">{source.title}</span>
                  <Button
                    variant="ghost"
                    size="sm"
                    onClick={() => {
                      setSource(null);
                      setError("");
                    }}
                  >
                    更换素材
                  </Button>
                </div>
              </div>
            ) : (
              <div
                className="flex min-h-36 flex-col items-center justify-center gap-3 rounded-lg border border-dashed bg-background p-4"
                onDragOver={(event) => event.preventDefault()}
                onDrop={(event) => {
                  event.preventDefault();
                  readLocal(event.dataTransfer.files);
                }}
              >
                <p className="text-sm font-medium">将文件拖至此处添加</p>
                <div className="flex flex-wrap justify-center gap-2">
                  <Button size="sm" onClick={() => input.current?.click()}>
                    <Upload data-icon="inline-start" />
                    从本地上传
                  </Button>
                  <Button
                    size="sm"
                    variant="secondary"
                    onClick={() => {
                      if (actorID) setPicking(true);
                      else {
                        onClose();
                        login();
                      }
                    }}
                  >
                    {tool.media[0] === "image" ? (
                      <FileImage data-icon="inline-start" />
                    ) : (
                      <Video data-icon="inline-start" />
                    )}
                    从资产库选择
                  </Button>
                </div>
                <Input
                  ref={input}
                  type="file"
                  hidden
                  aria-label="从本地上传"
                  accept={tool.media.flatMap((kind) => formats[kind]).join(",")}
                  onChange={(event) => {
                    readLocal(event.target.files);
                    event.target.value = "";
                  }}
                />
              </div>
            )}
            {error && (
              <Alert variant="destructive">
                <AlertDescription>{error}</AlertDescription>
              </Alert>
            )}
            <Alert>
              <AlertDescription>{tool.unavailable}</AlertDescription>
            </Alert>
          </>
        )}
      </DialogContent>
    </Dialog>
  );
}
