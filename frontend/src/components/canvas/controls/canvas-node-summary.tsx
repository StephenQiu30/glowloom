import {
  Type,
  Image,
  Video,
  AudioLines,
  Clapperboard,
  Contact,
  Box,
  Layers,
} from "lucide-react";
import { Button } from "@/components/ui/button";
import {
  Popover,
  PopoverContent,
  PopoverTitle,
  PopoverTrigger,
} from "@/components/ui/popover";
import type { CanvasNodeData } from "../types/canvas";

const categories = [
  { type: "text", label: "文本", icon: Type },
  { type: "image", label: "图片", icon: Image },
  { type: "video", label: "视频", icon: Video },
  { type: "audio", label: "音频", icon: AudioLines },
  { type: "timeline", label: "时间线", icon: Clapperboard },
  { type: "subject", label: "主体", icon: Contact },
  { type: "director", label: "导演台", icon: Box },
] as const;

export function CanvasNodeSummary({ nodes }: { nodes: CanvasNodeData[] }) {
  const counts = new Map<string, number>();
  for (const node of nodes)
    counts.set(node.type, (counts.get(node.type) ?? 0) + 1);
  const known = categories.reduce(
    (sum, item) => sum + (counts.get(item.type) ?? 0),
    0,
  );
  return (
    <Popover>
      <PopoverTrigger asChild>
        <Button
          variant="ghost"
          className="canvas-node-summary-trigger shrink-0 tabular-nums"
          aria-label={`节点统计，共 ${nodes.length} 个节点`}
        >
          节点 {nodes.length}
        </Button>
      </PopoverTrigger>
      <PopoverContent align="start" className="w-60" data-canvas-no-zoom>
        <PopoverTitle>节点统计</PopoverTitle>
        <dl className="mt-2">
          {categories.map(({ type, label, icon: Icon }) => (
            <div
              key={type}
              className="flex min-h-11 items-center justify-between gap-3"
            >
              <dt className="flex items-center gap-2">
                <Icon className="size-4" aria-hidden="true" />
                {label}
              </dt>
              <dd className="tabular-nums">{counts.get(type) ?? 0}</dd>
            </div>
          ))}
          {nodes.length > known && (
            <div className="flex min-h-11 items-center justify-between gap-3">
              <dt className="flex items-center gap-2">
                <Layers className="size-4" aria-hidden="true" />
                分组与其他节点
              </dt>
              <dd className="tabular-nums">{nodes.length - known}</dd>
            </div>
          )}
        </dl>
      </PopoverContent>
    </Popover>
  );
}
