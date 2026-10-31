import { Tag } from "lucide-react";
import { Button } from "@/components/ui/button";
import { Checkbox } from "@/components/ui/checkbox";
import {
  Popover,
  PopoverContent,
  PopoverTrigger,
} from "@/components/ui/popover";
import { canvasColorTags, type CanvasColorTag } from "../lib/canvas-color-tags";
import type { CanvasNodeData } from "../types/canvas";

export function CanvasColorTags({
  nodes,
  disabled,
  onChange,
}: {
  nodes: CanvasNodeData[];
  disabled?: boolean;
  onChange: (tag: CanvasColorTag | null) => void;
}) {
  return (
    <Popover>
      <PopoverTrigger asChild>
        <Button
          variant="ghost"
          size="icon-sm"
          aria-label="添加标签"
          disabled={disabled}
        >
          <Tag className="size-4" />
        </Button>
      </PopoverTrigger>
      <PopoverContent
        align="end"
        className="flex w-auto items-center gap-3 p-3"
        data-canvas-no-zoom
        onMouseDown={(event) => event.stopPropagation()}
        onPointerDown={(event) => event.stopPropagation()}
      >
        <div
          role="toolbar"
          aria-label="节点颜色标签"
          className="flex items-center gap-3"
        >
          <Button
            variant="ghost"
            size="sm"
            disabled={
              disabled ||
              !nodes.some((node) => node.metadata?.colorTags?.length)
            }
            onClick={() => onChange(null)}
          >
            全部清空
          </Button>
          {canvasColorTags.map((tag) => {
            const count = nodes.filter((node) =>
              node.metadata?.colorTags?.includes(tag.value),
            ).length;
            const checked = count === nodes.length && count > 0;
            return (
              <Checkbox
                key={tag.value}
                aria-label={`${checked ? "移除" : "添加"}“${tag.label}”`}
                checked={checked ? true : count > 0 ? "indeterminate" : false}
                disabled={disabled}
                onCheckedChange={() => onChange(tag.value)}
                className="size-6 rounded-full border-0"
                style={{ backgroundColor: tag.color }}
              />
            );
          })}
        </div>
      </PopoverContent>
    </Popover>
  );
}

export function CanvasColorTagMarks({ tags }: { tags?: CanvasColorTag[] }) {
  if (!tags?.length) return null;
  return (
    <span
      className="flex items-center gap-1"
      aria-label={`标签：${canvasColorTags
        .filter((tag) => tags.includes(tag.value))
        .map((tag) => tag.label)
        .join("、")}`}
    >
      {canvasColorTags
        .filter((tag) => tags.includes(tag.value))
        .map((tag) => (
          <span
            key={tag.value}
            className="size-2 rounded-full"
            style={{ backgroundColor: tag.color }}
          />
        ))}
    </span>
  );
}
