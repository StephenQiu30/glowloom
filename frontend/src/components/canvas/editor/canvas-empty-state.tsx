import { MousePointer2 } from "lucide-react";
import { ContentState } from "@/components/layout/content-state";

export function CanvasEmptyState() {
  return (
    <div className="pointer-events-none absolute inset-0 flex items-center justify-center text-sm text-muted-foreground">
      <ContentState
        className="flex-none"
        icon={MousePointer2}
        title="双击画布，创建节点"
      />
    </div>
  );
}
