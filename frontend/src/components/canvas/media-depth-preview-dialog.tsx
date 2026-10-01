"use client";
import { useEffect, useId, useRef, useState } from "react";
import { useForm, Controller, useWatch } from "react-hook-form";
import { zodResolver } from "@hookform/resolvers/zod";
import { z } from "zod";
import { useQuery } from "@tanstack/react-query";
import { Button } from "@/components/ui/button";
import { Checkbox } from "@/components/ui/checkbox";
import { Field, FieldError, FieldLabel } from "@/components/ui/field";
import {
  Dialog,
  DialogContent,
  DialogDescription,
  DialogFooter,
  DialogHeader,
  DialogTitle,
} from "@/components/ui/dialog";
import {
  depthActions,
  type DepthJob,
  type DepthPreview,
} from "./media-depth-model";
import type { MediaAsset } from "./queries";
import { previewDepth } from "./media-depth-queries";
import { useReleaseMediaSource } from "./nodes/media-lifecycle";
const confirmation = z.object({
  confirmed: z
    .boolean()
    .refine((value) => value, "请实际查看结果后明确确认人工审核。"),
});
export function MediaDepthPreviewDialog({
  job,
  locked,
  onClose,
  onReview,
  onAdopt,
  onVerifyOriginal,
  verifying = false,
  onRecoverStorage,
}: {
  job: DepthJob;
  locked: boolean;
  onClose: () => void;
  onReview: (preview: DepthPreview) => Promise<void> | void;
  onAdopt?: (job: DepthJob, asset: MediaAsset) => Promise<boolean>;
  onVerifyOriginal?: () => Promise<void>;
  verifying?: boolean;
  onRecoverStorage?: () => void;
}) {
  const inputId = useId(),
    video = useRef<HTMLVideoElement>(null),
    originalRequest = useRef<HTMLButtonElement>(null),
    storageRecovery = useRef<HTMLButtonElement>(null);
  const [decoded, setDecoded] = useState(false),
    [expired, setExpired] = useState(false),
    [busy, setBusy] = useState(false),
    [error, setError] = useState("");
  const form = useForm<{ confirmed: boolean }>({
    resolver: zodResolver(confirmation),
    defaultValues: { confirmed: false },
  });
  const confirmed = useWatch({ control: form.control, name: "confirmed" });
  const canVerifyOriginal = Boolean(onVerifyOriginal),
    canRecoverStorage = Boolean(onRecoverStorage);
  useEffect(() => {
    if (verifying || busy) return;
    if (canVerifyOriginal) originalRequest.current?.focus();
    else if (canRecoverStorage) storageRecovery.current?.focus();
  }, [busy, canRecoverStorage, canVerifyOriginal, verifying]);
  const preview = useQuery({
    queryKey: [
      "canvas",
      "depth-preview",
      job.project_id,
      job.id,
      job.revision,
      job.sha256,
    ],
    queryFn: ({ signal }) => previewDepth(job, signal),
    retry: false,
    staleTime: 0,
    gcTime: 0,
    refetchOnWindowFocus: false,
  });
  useReleaseMediaSource(video, expired ? "" : (preview.data?.url ?? ""));
  useEffect(() => {
    if (!preview.data) return;
    const timer = window.setTimeout(
      () => setExpired(true),
      Math.min(
        2147483647,
        Math.max(0, Date.parse(preview.data.expires_at) - Date.now() - 5000),
      ),
    );
    return () => window.clearTimeout(timer);
  }, [preview.data]);
  const unavailable =
      locked || busy || expired || !decoded || !preview.data || Boolean(error),
    actions = depthActions(job);
  function metadata() {
    const element = video.current;
    if (!element || !preview.data) return;
    const asset = preview.data.asset;
    if (
      element.videoWidth !== 1920 ||
      element.videoHeight !== 1080 ||
      !Number.isFinite(element.duration) ||
      Math.abs(element.duration * 1000 - asset.duration_ms) > 100
    ) {
      setDecoded(false);
      setError("视频尺寸或时长与核验结果不一致，当前不能审核。");
      return;
    }
    setDecoded(true);
  }
  async function review() {
    if (unavailable || !actions.review || !preview.data) return;
    setBusy(true);
    try {
      await onReview(preview.data);
    } catch (failure) {
      setError(failure instanceof Error ? failure.message : "审核未确认。");
    } finally {
      setBusy(false);
    }
  }
  async function adopt() {
    if (unavailable || !onAdopt || !preview.data || !actions.adopt) return;
    setBusy(true);
    try {
      if (await onAdopt(job, preview.data.asset)) onClose();
      else setError("画布尚未确认保存，请先处理画布错误，再核验是否已采纳。");
    } catch (failure) {
      setError(failure instanceof Error ? failure.message : "采纳未确认。");
    } finally {
      setBusy(false);
    }
  }
  return (
    <Dialog
      open
      onOpenChange={(open) => {
        if (!open && !locked && !busy) onClose();
      }}
    >
      <DialogContent
        className="max-h-[90dvh] min-w-0 overflow-y-auto sm:max-w-4xl"
        showCloseButton={false}
        onEscapeKeyDown={(event) => {
          if (locked || busy) event.preventDefault();
        }}
        onInteractOutside={(event) => {
          if (locked || busy) event.preventDefault();
        }}
      >
        <DialogHeader>
          <DialogTitle>视频深度结果人工审核</DialogTitle>
          <DialogDescription>
            实际查看完整视频，确认相对深度输出可用。审核只对应当前结果 SHA-256
            和修订，原视频保持不变。
          </DialogDescription>
        </DialogHeader>
        {preview.isPending ? (
          <p role="status">读取确切结果的私有预览…</p>
        ) : null}
        {preview.error ? (
          <div role="alert">
            <p>{preview.error.message}</p>
            <Button variant="ghost" onClick={() => void preview.refetch()}>
              重新读取预览
            </Button>
          </div>
        ) : null}
        {preview.data ? (
          <>
            <video
              ref={video}
              aria-label="确切深度结果预览"
              className="max-h-[50dvh] w-full rounded-lg bg-black"
              src={expired ? undefined : preview.data.url}
              controls
              playsInline
              preload="metadata"
              crossOrigin="anonymous"
              onLoadedMetadata={metadata}
              onCanPlay={metadata}
              onError={() => {
                setDecoded(false);
                setError("深度视频无法实际解码，请重新读取结果预览。");
              }}
            />
            <p className="text-xs break-all text-muted-foreground">
              修订 {preview.data.revision} · SHA-256 {preview.data.sha256}
            </p>
          </>
        ) : null}
        {expired ? (
          <p role="alert">私有预览授权已过期，请关闭后重新读取结果。</p>
        ) : null}
        {error ? <p role="alert">{error}</p> : null}
        {locked ? (
          <p role="status">
            操作尚未确认，保留原请求。请等待当前请求，或人工核验原请求。
          </p>
        ) : null}
        {onVerifyOriginal ? (
          <Button
            ref={originalRequest}
            disabled={verifying || busy}
            onClick={() => void onVerifyOriginal()}
          >
            核验原审核请求
          </Button>
        ) : null}
        {onRecoverStorage ? (
          <Button
            ref={storageRecovery}
            variant="outline"
            disabled={verifying || busy}
            onClick={onRecoverStorage}
          >
            重新检查浏览器存储
          </Button>
        ) : null}
        {actions.review ? (
          <form
            onSubmit={(event) => void form.handleSubmit(review)(event)}
            className="space-y-4"
          >
            <Controller
              control={form.control}
              name="confirmed"
              render={({ field, fieldState }) => (
                <Field data-invalid={fieldState.invalid}>
                  <div className="flex items-start gap-3">
                    <Checkbox
                      id={inputId}
                      name={field.name}
                      ref={field.ref}
                      checked={field.value}
                      onCheckedChange={(value) =>
                        field.onChange(value === true)
                      }
                      onBlur={field.onBlur}
                      disabled={unavailable}
                      aria-invalid={fieldState.invalid}
                    />
                    <FieldLabel htmlFor={inputId}>
                      我已实际查看完整结果，确认当前 SHA-256
                      对应视频通过人工审核
                    </FieldLabel>
                  </div>
                  <FieldError errors={[fieldState.error]} />
                </Field>
              )}
            />
            <Button type="submit" disabled={unavailable || !confirmed}>
              确认人工审核通过
            </Button>
          </form>
        ) : null}
        <DialogFooter>
          <Button variant="ghost" disabled={locked || busy} onClick={onClose}>
            关闭深度预览
          </Button>
          {actions.adopt && onAdopt ? (
            <Button disabled={unavailable} onClick={() => void adopt()}>
              采纳到来源画布
            </Button>
          ) : null}
        </DialogFooter>
      </DialogContent>
    </Dialog>
  );
}
