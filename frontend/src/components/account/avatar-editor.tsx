"use client";
import { FeedbackToast } from "@/components/feedback/feedback-toast";
import { Checkbox } from "@/components/ui/checkbox";
import { FieldLabel } from "@/components/ui/field";
import { useEffect, useEffectEvent, useRef, useState } from "react";
import { uploadMyAvatar } from "@/api/media";
import { authErrorMessage } from "@/components/auth/contracts";
import { RequestError } from "@/lib/request";
import { Avatar, AvatarImage, AvatarFallback } from "@/components/ui/avatar";
import { Button } from "@/components/ui/button";
import { Input } from "@/components/ui/input";
type Preview = { url?: string; expires_at?: string };
export function BoundAvatar({
  load,
  assetID,
  label = "头像",
  size = "default",
}: {
  load: (signal: AbortSignal) => Promise<Preview>;
  assetID?: string;
  label?: string;
  size?: "compact" | "default";
}) {
  const [preview, setPreview] = useState<{
    assetID: string;
    url: string;
    error: string;
  } | null>(null);
  const current = preview?.assetID === assetID ? preview : null;
  const url = current?.url ?? "";
  const error = current?.error ?? "";
  const [retry, setRetry] = useState(0);
  const host = useRef<HTMLDivElement>(null);
  const readPreview = useEffectEvent(load);
  useEffect(() => {
    if (!assetID) return;
    const boundAssetID = assetID;
    const c = new AbortController();
    let timer: ReturnType<typeof setTimeout> | undefined;
    let visible = document.visibilityState !== "hidden";
    let reading = false;
    async function read() {
      if (reading || !visible || c.signal.aborted) return;
      reading = true;
      clearTimeout(timer);
      try {
        const p = await readPreview(c.signal);
        if (!c.signal.aborted && visible) {
          if (!p.url) throw new Error("头像预览不可用");
          setPreview({ assetID: boundAssetID, url: p.url, error: "" });
          timer = setTimeout(
            () => {
              if (visible) void read();
            },
            Math.max(
              5000,
              new Date(p.expires_at ?? "").getTime() - Date.now() - 10000,
            ) || 45000,
          );
        }
      } catch (e) {
        if (!c.signal.aborted) {
          setPreview(
            e instanceof RequestError && e.status === 404
              ? null
              : { assetID: boundAssetID, url: "", error: authErrorMessage(e) },
          );
        }
      } finally {
        reading = false;
      }
    }
    const observe =
      typeof IntersectionObserver !== "undefined"
        ? new IntersectionObserver((entries) => {
            visible =
              (entries[0]?.isIntersecting ?? false) &&
              document.visibilityState !== "hidden";
            if (!visible) {
              clearTimeout(timer);
              setPreview(null);
            } else void read();
          })
        : null;
    if (observe && host.current) observe.observe(host.current);
    else void read();
    function visibility() {
      visible = document.visibilityState !== "hidden";
      if (visible) void read();
      else {
        clearTimeout(timer);
        setPreview(null);
      }
    }
    document.addEventListener("visibilitychange", visibility);
    return () => {
      c.abort();
      clearTimeout(timer);
      observe?.disconnect();
      document.removeEventListener("visibilitychange", visibility);
    };
  }, [assetID, retry]);
  return (
    <div ref={host}>
      <Avatar className={size === "compact" ? "size-12" : "size-14"}>
        <AvatarImage src={url || undefined} alt={label} />
        <AvatarFallback>
          {label === "头像" ? "织" : Array.from(label.trim())[0] || "织"}
        </AvatarFallback>
      </Avatar>
      {error ? (
        <FeedbackToast
          message={error}
          action={{
            label: "重新读取头像",
            onClick: () => {
              setPreview(null);
              setRetry((n) => n + 1);
            },
          }}
        />
      ) : null}
    </div>
  );
}
async function cropSquare(file: File) {
  const bitmap = await createImageBitmap(file);
  try {
    if (bitmap.width > 4096 || bitmap.height > 4096)
      throw new RequestError("invalid_avatar_dimensions", 422);
    const size = Math.min(bitmap.width, bitmap.height);
    const canvas = document.createElement("canvas");
    canvas.width = size;
    canvas.height = size;
    const ctx = canvas.getContext("2d");
    if (!ctx) throw new Error("无法裁剪图片");
    ctx.drawImage(
      bitmap,
      (bitmap.width - size) / 2,
      (bitmap.height - size) / 2,
      size,
      size,
      0,
      0,
      size,
      size,
    );
    const blob = await new Promise<Blob>((resolve, reject) =>
      canvas.toBlob(
        (v) => (v ? resolve(v) : reject(new Error("裁剪失败"))),
        "image/png",
      ),
    );
    return new File([blob], "avatar.png", { type: "image/png" });
  } finally {
    bitmap.close();
  }
}
export function AvatarEditor({
  assetID,
  load,
  bind,
  onConflict,
  compact = false,
  label = "头像",
  onBusyChange,
}: {
  assetID?: string;
  load: (signal: AbortSignal) => Promise<Preview>;
  bind: (asset: string | null, signal: AbortSignal) => Promise<void>;
  onConflict?: () => Promise<void> | void;
  compact?: boolean;
  label?: string;
  onBusyChange?: (busy: boolean) => void;
}) {
  const input = useRef<HTMLInputElement>(null);
  const [file, setFile] = useState<File | null>(null);
  const [local, setLocal] = useState("");
  const [crop, setCrop] = useState(false);
  const [pending, setPending] = useState(false);
  const [error, setError] = useState("");
  const [notice, setNotice] = useState("");
  const flight = useRef<AbortController | null>(null);
  const lock = useRef(false);
  useEffect(
    () => () => {
      if (local) URL.revokeObjectURL(local);
    },
    [local],
  );
  useEffect(() => () => flight.current?.abort(), []);
  async function save(remove = false) {
    if (lock.current) return;
    lock.current = true;
    setPending(true);
    onBusyChange?.(true);
    setError("");
    const c = new AbortController();
    flight.current = c;
    try {
      let id: string | null = null;
      if (!remove) {
        if (!file) return;
        const outgoingFile = crop ? await cropSquare(file) : file;
        if (c.signal.aborted) return;
        const uploaded = await uploadMyAvatar({}, outgoingFile, {
          signal: c.signal,
        });
        if (c.signal.aborted) return;
        if (!uploaded.id) throw new Error("头像上传回执不完整");
        id = uploaded.id;
      }
      if (c.signal.aborted) return;
      await bind(id, c.signal);
      if (!c.signal.aborted) {
        setFile(null);
        setLocal("");
        setCrop(false);
        setNotice(remove ? "已恢复默认头像。" : "头像已保存并重新读取。");
      }
    } catch (e) {
      if (!c.signal.aborted) {
        setError(authErrorMessage(e));
        if (e instanceof RequestError && e.status === 409) await onConflict?.();
      }
    } finally {
      lock.current = false;
      if (!c.signal.aborted) {
        setPending(false);
        onBusyChange?.(false);
      }
    }
  }
  return (
    <section className="grid gap-3" aria-label="编辑头像">
      <div className="flex items-center gap-3">
        <BoundAvatar
          key={assetID}
          assetID={assetID}
          label={label}
          load={load}
          size={compact ? "compact" : "default"}
        />
        {compact && (
          <Button
            type="button"
            variant="secondary"
            disabled={pending}
            onClick={() => input.current?.click()}
          >
            更换头像
          </Button>
        )}
      </div>
      <FieldLabel className={compact ? "sr-only" : "grid gap-2"}>
        选择头像（JPEG、PNG、WebP，最多 5 MB）
        <Input
          ref={input}
          type="file"
          accept="image/jpeg,image/png,image/webp"
          disabled={pending}
          onChange={(e) => {
            const next = e.target.files?.[0];
            setError("");
            setNotice("");
            setFile(null);
            setLocal("");
            setCrop(false);
            e.target.value = "";
            if (
              next &&
              !["image/jpeg", "image/png", "image/webp"].includes(next.type)
            ) {
              setError("请选择 JPEG、PNG 或 WebP 图片。");
              return;
            }
            if (next && next.size > 5 * 1024 * 1024) {
              setError("图片不能超过 5 MB。");
              return;
            }
            setFile(next ?? null);
            setLocal(next ? URL.createObjectURL(next) : "");
          }}
        />
      </FieldLabel>
      {file ? (
        <>
          <Avatar className="size-24">
            <AvatarImage
              src={local}
              alt="待上传的原图"
              className={crop ? "rounded-none" : "object-contain"}
            />
            <AvatarFallback>预览</AvatarFallback>
          </Avatar>
          <FieldLabel className="flex items-center gap-2">
            <Checkbox
              checked={crop}
              onCheckedChange={(value) => setCrop(value === true)}
              disabled={pending}
            />
            将图片居中裁剪为正方形
          </FieldLabel>
          <div className="flex flex-wrap justify-end gap-3">
            <Button
              type="button"
              disabled={pending}
              onClick={() => void save()}
            >
              上传并保存头像
            </Button>
            <Button
              type="button"
              variant="outline"
              disabled={pending}
              onClick={() => {
                setFile(null);
                setLocal("");
                setCrop(false);
              }}
            >
              取消选择
            </Button>
          </div>
        </>
      ) : null}
      {assetID ? (
        <Button
          type="button"
          variant="ghost"
          className="w-fit"
          disabled={pending}
          onClick={() => void save(true)}
        >
          恢复默认头像
        </Button>
      ) : null}
      {pending ? <p role="status">正在保存头像…</p> : null}
      {error ? <FeedbackToast message={error} /> : null}
      {notice ? <FeedbackToast message={notice} tone="info" /> : null}
    </section>
  );
}
