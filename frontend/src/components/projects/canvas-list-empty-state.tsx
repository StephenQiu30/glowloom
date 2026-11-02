import type { ReactNode } from "react";
import { PanelsTopLeft } from "lucide-react";
import { ContentState } from "@/components/layout/content-state";

export type CanvasListEmptyKind =
  "search" | "filtered" | "active" | "archived" | "deleted";

export const canvasListEmptyCopy: Record<
  CanvasListEmptyKind,
  { title: string; description: string }
> = {
  search: {
    title: "没有匹配的画布",
    description: "试试其他名称，或清空搜索查看所有画布。",
  },
  filtered: {
    title: "没有匹配的画布",
    description: "试试调整关键词或筛选条件。",
  },
  active: {
    title: "还没有画布",
    description: "新建一张画布，开始创作。",
  },
  archived: {
    title: "没有已归档的画布",
    description: "已归档的画布会显示在这里。",
  },
  deleted: {
    title: "回收站为空",
    description: "已删除的画布会显示在这里。",
  },
};

export function CanvasListEmptyState({
  kind,
  actions,
  className,
  showDescription = true,
}: {
  kind: CanvasListEmptyKind;
  actions?: ReactNode;
  className?: string;
  showDescription?: boolean;
}) {
  const copy = canvasListEmptyCopy[kind];
  return (
    <ContentState
      className={className}
      icon={PanelsTopLeft}
      title={copy.title}
      description={showDescription ? copy.description : undefined}
      actions={actions}
    />
  );
}
