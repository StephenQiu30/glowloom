"use client";

import { useQuery } from "@tanstack/react-query";
import { Checkbox } from "@/components/ui/checkbox";
import { Field, FieldLabel } from "@/components/ui/field";
import { Input } from "@/components/ui/input";
import { getMediaPreview } from "./queries";
import type { TimelineClip } from "./timeline";

export function TimelineCropControl({
  clip,
  projectId,
  assetId,
  disabled,
  onChange,
}: {
  clip: TimelineClip;
  projectId: string;
  assetId: string | undefined;
  disabled: boolean;
  onChange: (crop: TimelineClip["crop"]) => void;
}) {
  const preview = useQuery({
    queryKey: ["canvas", "timeline-preview", projectId, assetId],
    enabled: Boolean(assetId),
    queryFn: async ({ signal }) => {
      const result = await getMediaPreview(projectId, assetId!, signal);
      if (
        result.asset.id !== assetId ||
        result.asset.project_id !== projectId ||
        result.asset.kind !== clip.kind
      )
        throw new Error("裁切素材身份不匹配。");
      return result;
    },
    staleTime: 60000,
    gcTime: 60000,
    retry: false,
  });
  const width = preview.data?.asset.width;
  const height = preview.data?.asset.height;
  const canCrop = width && height && width >= 2 && height >= 2;
  return (
    <fieldset className="grid gap-3 rounded border p-3 sm:col-span-4 sm:grid-cols-4">
      <legend className="px-1 text-xs">空间裁切（原件像素）</legend>
      <label className="flex items-center gap-2 text-sm sm:col-span-4">
        <Checkbox
          checked={Boolean(clip.crop)}
          disabled={disabled || !canCrop}
          onCheckedChange={(checked) => {
            if (checked === true && width && height)
              onChange({
                x: 0,
                y: 0,
                width: clip.kind === "video" ? width - (width % 2) : width,
                height: clip.kind === "video" ? height - (height % 2) : height,
              });
            else onChange(null);
          }}
          aria-label="启用片段空间裁切"
        />
        启用裁切
        {canCrop ? ` · 原件 ${width} × ${height}` : " · 正在读取原件尺寸"}
      </label>
      {preview.error || !assetId ? (
        <p role="alert" className="text-xs text-destructive sm:col-span-4">
          无法读取裁切素材尺寸，请重新选择有效素材。
        </p>
      ) : null}
      {clip.crop
        ? (
            [
              ["x", "左边距"],
              ["y", "上边距"],
              ["width", "宽度"],
              ["height", "高度"],
            ] as const
          ).map(([key, label]) => (
            <Field key={key}>
              <FieldLabel htmlFor={`timeline-crop-${key}`}>
                {label}（像素）
              </FieldLabel>
              <Input
                id={`timeline-crop-${key}`}
                type="number"
                min={key === "x" || key === "y" ? 0 : 2}
                max={key === "x" || key === "width" ? width : height}
                step={
                  clip.kind === "video" && (key === "width" || key === "height")
                    ? 2
                    : 1
                }
                value={clip.crop![key]}
                disabled={disabled}
                onChange={(event) =>
                  onChange({ ...clip.crop!, [key]: Number(event.target.value) })
                }
              />
            </Field>
          ))
        : null}
      {clip.crop && clip.kind === "video" ? (
        <p className="text-xs text-muted-foreground sm:col-span-4">
          视频宽高须为偶数。预览与导出均先裁切原件，再等比适配输出画幅。
        </p>
      ) : null}
    </fieldset>
  );
}
