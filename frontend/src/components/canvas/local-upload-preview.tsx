"use client";
import { useEffect, useRef, useState } from "react";
import { Button } from "@/components/ui/button";
import dynamic from "next/dynamic";
const ModelPreview = dynamic(
  () =>
    import("@/components/media/library-model-preview").then(
      (module) => module.LibraryModelPreview,
    ),
  { ssr: false },
);

/** Local preview leases exist only while the user opens this file. */
export function LocalUploadPreview({ file }: { file: File }) {
  const [open, setOpen] = useState(false);
  const extension = file.name.split(".").at(-1)?.toLowerCase() ?? "";
  const kind = ["mp4", "mov", "webm"].includes(extension)
    ? "video"
    : ["wav", "mp3", "m4a"].includes(extension)
      ? "audio"
      : ["jpg", "jpeg", "png", "webp", "gif"].includes(extension)
        ? "image"
        : ["glb", "gltf"].includes(extension)
          ? "model"
          : undefined;
  if (!kind) return null;
  return (
    <div>
      <Button
        type="button"
        size="sm"
        variant="ghost"
        aria-expanded={open}
        onClick={() => setOpen(!open)}
      >
        {open ? "收起本地预览" : "预览本地文件"}
      </Button>
      {open &&
        (kind === "model" ? (
          <LocalModelSource file={file} />
        ) : (
          <PreviewSource file={file} kind={kind} />
        ))}
    </div>
  );
}
function LocalModelSource({ file }: { file: File }) {
  const [failed, setFailed] = useState(false);
  if (failed)
    return (
      <p role="alert" className="text-sm text-destructive">
        本地模型无法预览，请检查完整原件与嵌入资源后重试。
      </p>
    );
  return (
    <section aria-label={`${file.name} 本地预览`}>
      <ModelPreview
        file={file}
        byteSize={file.size}
        mimeType={
          /\.gltf$/i.test(file.name) ? "model/gltf+json" : "model/gltf-binary"
        }
        onError={() => setFailed(true)}
      />
    </section>
  );
}
function PreviewSource({
  file,
  kind,
}: {
  file: File;
  kind: "image" | "video" | "audio";
}) {
  const elementRef = useRef<
    HTMLVideoElement | HTMLAudioElement | HTMLImageElement | null
  >(null);
  const [failed, setFailed] = useState(false);
  useEffect(() => {
    if (failed) return;
    const element = elementRef.current;
    if (!element) return;
    const current = URL.createObjectURL(file);
    element.setAttribute("src", current);
    return () => {
      if (element instanceof HTMLMediaElement) element.pause();
      element.removeAttribute("src");
      if (element instanceof HTMLMediaElement) element.load();
      URL.revokeObjectURL(current);
    };
  }, [file, kind, failed]);
  if (failed)
    return (
      <p role="alert" className="text-sm text-destructive">
        本地文件无法预览，请检查格式与内容后重试。
      </p>
    );
  const label = `${file.name} 本地预览`;
  return kind === "video" ? (
    <video
      ref={(element) => {
        elementRef.current = element;
      }}
      aria-label={label}
      controls
      muted
      preload="metadata"
      className="max-h-60 w-full rounded border bg-black"
      onError={() => setFailed(true)}
    />
  ) : kind === "audio" ? (
    <audio
      ref={(element) => {
        elementRef.current = element;
      }}
      aria-label={label}
      controls
      preload="metadata"
      className="w-full"
      onError={() => setFailed(true)}
    />
  ) : (
    // This browser-owned Blob URL cannot be fetched or cached by the Next image server.
    // eslint-disable-next-line @next/next/no-img-element
    <img
      ref={(element) => {
        elementRef.current = element;
      }}
      alt={label}
      className="max-h-60 w-full rounded border object-contain"
      onError={() => setFailed(true)}
    />
  );
}
