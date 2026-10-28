"use client";

import {
  FileText,
  Heart,
  ImageIcon,
  MoreHorizontal,
  Music,
  Video,
} from "lucide-react";
import { ProjectMediaPreview } from "@/components/media/project-media";
import { taskLabels } from "@/components/generation/contracts";
import { Button } from "@/components/ui/button";
import { Checkbox } from "@/components/ui/checkbox";
import {
  DropdownMenu,
  DropdownMenuContent,
  DropdownMenuGroup,
  DropdownMenuItem,
  DropdownMenuTrigger,
} from "@/components/ui/dropdown-menu";
import type { AssetKind } from "./asset-query";
import {
  hasOriginalDocument,
  isVideoDubbing,
  type HistoryCardTarget,
} from "./history-results";

export function historyCardTitle(card: HistoryCardTarget) {
  const title = card.task.prompt_preview || card.task.model_name || "生成记录";
  return card.output && card.task.outputs.length > 1
    ? `${title} · 结果${card.task.outputs.findIndex((output) => output.id === card.output!.id) + 1}`
    : title;
}

export function HistoryResultCard({
  card,
  kind,
  batch,
  checked,
  disabled,
  onOpen,
  onToggle,
  onFavorite,
  onDelete,
  onDownload,
  onPublish,
  onReuse,
}: {
  card: HistoryCardTarget;
  kind: AssetKind;
  batch: boolean;
  checked: boolean;
  disabled: boolean;
  onOpen: () => void;
  onToggle: () => void;
  onFavorite: () => void;
  onDelete: () => void;
  onDownload: () => void;
  onPublish: () => void;
  onReuse: () => void;
}) {
  const output = card.output ?? card.outputs[0];
  const Icon =
    kind === "image"
      ? ImageIcon
      : kind === "video"
        ? Video
        : kind === "audio"
          ? Music
          : FileText;
  const title = historyCardTitle(card);
  const media =
    output?.media_asset_id &&
    output.moderation_status === "passed" &&
    ["image", "video"].includes(output.kind ?? "");
  const metadata = [output?.resolution, output?.ratio, card.task.model_name]
    .filter(Boolean)
    .join(" · ");
  return (
    <article
      className="group relative min-w-0 [contain-intrinsic-size:auto_16rem] [content-visibility:auto]"
      aria-label={title}
    >
      <Button
        variant="ghost"
        disabled={disabled}
        aria-pressed={batch ? checked : undefined}
        className="h-auto w-full min-w-0 flex-col items-stretch gap-2 overflow-hidden p-0 text-left font-normal"
        onClick={batch ? onToggle : onOpen}
      >
        <span className="relative flex aspect-video w-full items-center justify-center overflow-hidden rounded-lg bg-muted">
          {media ? (
            <ProjectMediaPreview
              projectID={card.task.project_id}
              id={output.media_asset_id!}
              kind={output.kind}
              compact
              interactive={false}
            />
          ) : (
            <Icon className="size-8 text-muted-foreground" aria-hidden="true" />
          )}
        </span>
        <span className="line-clamp-2 w-full text-sm break-words whitespace-normal">
          {title}
        </span>
        <span className="w-full truncate text-xs text-muted-foreground">
          {[taskLabels[card.task.status], metadata].filter(Boolean).join(" · ")}
        </span>
        <span className="w-full truncate text-xs text-muted-foreground">
          {new Date(card.task.create_time).toLocaleString("zh-CN")}
        </span>
      </Button>
      {batch ? (
        <Checkbox
          className="absolute top-3 left-3 after:-inset-3.5"
          checked={checked}
          disabled={disabled}
          aria-label={`选择${title}`}
          onCheckedChange={onToggle}
        />
      ) : (
        <div className="absolute top-2 right-2 flex gap-1 rounded-lg bg-popover/90 p-1 text-popover-foreground opacity-0 transition-opacity group-focus-within:opacity-100 group-hover:opacity-100 motion-reduce:transition-none max-md:opacity-100">
          <Button
            size="icon"
            variant="ghost"
            className="max-md:size-11"
            disabled={disabled || !card.canFavorite}
            aria-pressed={card.favorite}
            aria-label={`${card.favorite ? "取消收藏" : "收藏"}${title}`}
            onClick={onFavorite}
          >
            <Heart
              aria-hidden="true"
              className={card.favorite ? "fill-current" : undefined}
            />
          </Button>
          <DropdownMenu>
            <DropdownMenuTrigger asChild>
              <Button
                size="icon"
                variant="ghost"
                className="max-md:size-11"
                disabled={disabled}
                aria-label={`更多 · ${title}`}
              >
                <MoreHorizontal aria-hidden="true" />
              </Button>
            </DropdownMenuTrigger>
            <DropdownMenuContent align="end">
              <DropdownMenuGroup>
                <DropdownMenuItem onSelect={onOpen}>查看详情</DropdownMenuItem>
                <DropdownMenuItem onSelect={onReuse}>再次创作</DropdownMenuItem>
                <DropdownMenuItem
                  disabled={!card.canFavorite}
                  onSelect={onFavorite}
                >
                  {card.favorite ? "取消收藏" : "收藏"}
                </DropdownMenuItem>
                <DropdownMenuItem
                  disabled={!card.canDownload}
                  onSelect={onDownload}
                >
                  {hasOriginalDocument(card) ? "下载原件" : "下载"}
                </DropdownMenuItem>
                {(kind === "image" ||
                  (kind === "video" && !isVideoDubbing(card))) && (
                  <DropdownMenuItem
                    disabled={!card.canPublish}
                    onSelect={onPublish}
                  >
                    发布
                  </DropdownMenuItem>
                )}
                <DropdownMenuItem onSelect={onDelete}>删除</DropdownMenuItem>
              </DropdownMenuGroup>
            </DropdownMenuContent>
          </DropdownMenu>
        </div>
      )}
    </article>
  );
}
