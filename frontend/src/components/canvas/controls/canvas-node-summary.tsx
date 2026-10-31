"use client";
import { useState } from "react";
import dynamic from "next/dynamic";
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
import { canvasNodeStatistics } from "../information/node-statistics";
const InformationDialog = dynamic(
  () =>
    import("../information/canvas-information-dialog").then(
      (m) => m.CanvasInformationDialog,
    ),
  { loading: () => <span role="status">正在加载项目信息…</span> },
);
const icons = {
  text: Type,
  image: Image,
  video: Video,
  audio: AudioLines,
  timeline: Clapperboard,
  subject: Contact,
  external: Box,
  other: Layers,
};
export function CanvasNodeSummary({
  nodes,
  canvasID,
  projectID,
}: {
  nodes: CanvasNodeData[];
  canvasID?: string;
  projectID?: string;
}) {
  const [open, setOpen] = useState(false),
    [information, setInformation] = useState(false);
  const categories = canvasNodeStatistics(nodes).filter(
    (item) => item.count > 0,
  );
  return (
    <>
      <Popover open={open} onOpenChange={setOpen}>
        <PopoverTrigger asChild>
          <Button
            variant="ghost"
            className="canvas-node-summary-trigger shrink-0 tabular-nums"
            aria-label={`节点统计，共 ${nodes.length} 个节点`}
          >
            节点 {nodes.length}
          </Button>
        </PopoverTrigger>
        <PopoverContent
          align="start"
          className="w-[200px] gap-0 p-2"
          data-canvas-no-zoom
          data-canvas-shortcuts-ignore
          onPointerDown={(e) => e.stopPropagation()}
          onMouseDown={(e) => e.stopPropagation()}
        >
          <PopoverTitle className="sr-only">节点统计</PopoverTitle>
          <dl>
            {categories.map(({ type, label, count }) => {
              const Icon = icons[type];
              return (
                <div
                  key={type}
                  className="flex h-9 items-center justify-between gap-3 px-2"
                >
                  <dt className="flex items-center gap-2">
                    <Icon className="size-4" aria-hidden="true" />
                    {label}
                  </dt>
                  <dd className="tabular-nums">{count}</dd>
                </div>
              );
            })}
          </dl>
          {nodes.length === 0 && (
            <p className="px-2 py-3 text-muted-foreground">暂无节点</p>
          )}
          {canvasID && projectID && (
            <>
              <div className="h-3 border-t" />
              <Button
                variant="ghost"
                className="h-9 justify-start px-2"
                onClick={() => {
                  setOpen(false);
                  setInformation(true);
                }}
              >
                查看项目信息
              </Button>
            </>
          )}
        </PopoverContent>
      </Popover>
      {information && canvasID && projectID && (
        <InformationDialog
          key={canvasID + projectID}
          canvasID={canvasID}
          projectID={projectID}
          nodes={nodes}
          onClose={() => setInformation(false)}
        />
      )}
    </>
  );
}
