"use client";

import { useState } from "react";
import { z } from "zod";
import { useInfiniteQuery } from "@tanstack/react-query";
import Link from "next/link";
import { Button } from "@/components/ui/button";
import { listMediaAssets } from "@/components/canvas/queries";
import type { GenerationConfig } from "@/components/canvas/generation-config";

const role = z.object({
  max_count: z.number().int().nonnegative(),
  types: z.array(z.enum(["image", "video", "audio"])),
  max_ms: z.number().int().positive().optional(),
  max_bytes: z.number().int().positive().optional(),
});
export function generationReferenceRoles(
  inputRoles: string[],
  limits: Record<string, z.core.util.JSONType>,
) {
  const parsed = z.record(z.string(), z.json()).safeParse(limits.roles);
  if (!parsed.success) return [];
  return inputRoles.flatMap((name) => {
    const value = role.safeParse(parsed.data[name]);
    return name !== "prompt" && value.success && value.data.max_count > 0
      ? [{ name, ...value.data }]
      : [];
  });
}
export function ReferenceInputs({
  projectId,
  inputRoles,
  limits,
  inputs,
  onChange,
  disabled,
}: {
  projectId: string;
  inputRoles: string[];
  limits: Record<string, z.core.util.JSONType>;
  inputs: GenerationConfig["inputs"];
  onChange: (value: GenerationConfig["inputs"]) => void;
  disabled: boolean;
}) {
  const roles = generationReferenceRoles(inputRoles, limits);
  const [chosenRole, setChosenRole] = useState("");
  const selectedRole =
    roles.find((role) => role.name === chosenRole) ?? roles[0];
  const media = useInfiniteQuery({
    queryKey: ["media", "form-references", projectId],
    initialPageParam: undefined as string | undefined,
    queryFn: ({ pageParam, signal }) =>
      listMediaAssets(projectId, pageParam, signal),
    getNextPageParam: (page) => page.next_cursor ?? undefined,
    enabled: roles.length > 0,
    retry: false,
  });
  const assets = media.data?.pages.flatMap((page) => page.items) ?? [];
  const allowed = assets.filter(
    (asset) =>
      selectedRole?.types.includes(asset.kind as "image" | "video" | "audio") &&
      (!selectedRole.max_bytes || asset.byte_size <= selectedRole.max_bytes) &&
      (!selectedRole.max_ms ||
        (!!asset.duration_ms && asset.duration_ms <= selectedRole.max_ms)),
  );
  const maxed =
    !selectedRole ||
    inputs.length >= 255 ||
    inputs.filter((item) => item.role === selectedRole.name).length >=
      selectedRole.max_count;
  function move(index: number, delta: number) {
    const next = [...inputs],
      target = index + delta;
    if (target < 0 || target >= next.length) return;
    [next[index], next[target]] = [next[target], next[index]];
    onChange(next);
  }
  if (!roles.length && !inputs.length) return null;
  return (
    <section className="space-y-3" aria-label="参考素材">
      <div className="flex items-center justify-between">
        <h2 className="text-sm font-medium">参考素材</h2>
        <Button variant="link" className="h-auto p-0 text-xs" asChild>
          <Link href={`/assets?project_id=${projectId}`}>上传或管理素材</Link>
        </Button>
      </div>
      {inputs.map((input, index) => (
        <div
          key={`${input.mediaAssetId}:${input.role}:${index}`}
          className="flex flex-wrap items-center gap-2 rounded-lg bg-muted/40 p-3 text-sm"
        >
          <span className="min-w-0 flex-1 truncate">
            参考 {index + 1} ·{" "}
            {assets.find((asset) => asset.id === input.mediaAssetId)
              ?.file_name ?? input.mediaAssetId}
          </span>
          <select
            aria-label={`参考 ${index + 1} 用途`}
            value={input.role}
            disabled={disabled || !roles.length}
            className="h-8 max-w-full rounded border bg-background px-2 text-xs"
            onChange={(event) =>
              onChange(
                inputs.map((value, position) =>
                  position === index
                    ? { ...value, role: event.target.value }
                    : value,
                ),
              )
            }
          >
            {!roles.some((role) => role.name === input.role) && (
              <option value={input.role}>{input.role} · 待确认用途</option>
            )}
            {roles.map((role) => (
              <option key={role.name} value={role.name}>
                {role.name}
              </option>
            ))}
          </select>
          <Button
            variant="ghost"
            size="sm"
            disabled={disabled || index === 0}
            aria-label={`上移参考 ${index + 1}`}
            onClick={() => move(index, -1)}
          >
            ↑
          </Button>
          <Button
            variant="ghost"
            size="sm"
            disabled={disabled || index === inputs.length - 1}
            aria-label={`下移参考 ${index + 1}`}
            onClick={() => move(index, 1)}
          >
            ↓
          </Button>
          <Button
            variant="ghost"
            size="sm"
            disabled={disabled}
            onClick={() =>
              onChange(inputs.filter((_, position) => position !== index))
            }
          >
            移除
          </Button>
        </div>
      ))}
      {roles.length > 0 && (
        <div className="flex flex-wrap gap-2">
          <select
            aria-label="参考用途"
            className="h-10 rounded-lg border bg-background px-3 text-sm"
            value={selectedRole?.name ?? ""}
            onChange={(event) => setChosenRole(event.target.value)}
            disabled={disabled}
          >
            {roles.map((role) => (
              <option key={role.name} value={role.name}>
                {role.name} · 最多 {role.max_count} 项
              </option>
            ))}
          </select>
          <select
            aria-label="添加参考素材"
            className="h-10 min-w-48 flex-1 rounded-lg border bg-background px-3 text-sm"
            value=""
            onChange={(event) => {
              if (event.target.value && selectedRole)
                onChange([
                  ...inputs,
                  { role: selectedRole.name, mediaAssetId: event.target.value },
                ]);
            }}
            disabled={disabled || media.isPending || maxed}
          >
            <option value="">
              {maxed ? "已达到此用途数量上限" : "添加项目素材"}
            </option>
            {allowed.map((asset) => (
              <option key={asset.id} value={asset.id}>
                {asset.file_name}
              </option>
            ))}
          </select>
        </div>
      )}
      {media.isError && (
        <p role="alert" className="text-sm">
          参考素材读取失败。
          <Button variant="link" onClick={() => void media.refetch()}>
            重新读取
          </Button>
        </p>
      )}
      {media.hasNextPage && (
        <Button
          variant="ghost"
          size="sm"
          disabled={media.isFetchingNextPage}
          onClick={() => void media.fetchNextPage()}
        >
          加载更多素材
        </Button>
      )}
      <p className="text-xs leading-5 text-muted-foreground">
        引用按显示顺序提交。模型的格式、时长和数量限制将在报价时再次核对。
      </p>
    </section>
  );
}
