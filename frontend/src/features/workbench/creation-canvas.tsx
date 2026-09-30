"use client";

import Link from "next/link";
import {
  Background,
  Controls,
  Handle,
  Position,
  ReactFlow,
  useNodesState,
  type NodeProps,
  type Node,
} from "@xyflow/react";
import "@xyflow/react/dist/style.css";
import { useState } from "react";
import { useTheme } from "next-themes";
import { Button } from "@/components/ui/button";
import { PageHeading, usePreviewQuery } from "./poc-components";

type CreationNode = Node<
  { title: string; detail: string; href: string },
  "creation"
>;
function CreationCard({ data }: NodeProps<CreationNode>) {
  return (
    <div className="w-52 rounded-xl bg-background p-5">
      <Handle type="target" position={Position.Left} />
      <p className="text-sm font-medium">{data.title}</p>
      <p className="mt-2 text-xs leading-6 text-muted-foreground">
        {data.detail}
      </p>
      <Link
        className="nodrag nopan mt-3 inline-block rounded text-xs underline focus-visible:ring-2 focus-visible:ring-blue-500"
        href={data.href}
      >
        打开关联页面
      </Link>
      <Handle type="source" position={Position.Right} />
    </div>
  );
}
const nodeTypes = { creation: CreationCard };
export function CreationCanvas({ projectId }: { projectId: string }) {
  const { readOnly } = usePreviewQuery();
  const { resolvedTheme } = useTheme();
  const root = `/projects/${projectId}`;
  const [initial] = useState<CreationNode[]>(() => [
    {
      id: "script",
      type: "creation",
      position: { x: 0, y: 140 },
      data: {
        title: "第一集 · 剧本",
        detail: "样例 v2 / 3 个场景",
        href: `${root}/script`,
      },
    },
    {
      id: "character",
      type: "creation",
      position: { x: 0, y: -50 },
      data: {
        title: "林夏 · 角色资产",
        detail: "样例 v3 / 已锁定",
        href: `${root}/bible`,
      },
    },
    {
      id: "shot",
      type: "creation",
      position: { x: 310, y: 80 },
      data: {
        title: "镜头 01 · 港口",
        detail: "引用角色与场景 / 5 秒",
        href: `${root}/episodes/ep-01/shots/shot-01`,
      },
    },
    {
      id: "media",
      type: "creation",
      position: { x: 620, y: 0 },
      data: {
        title: "关键帧候选 A、B",
        detail: "内置样例 / 非真实生成",
        href: `${root}/media`,
      },
    },
    {
      id: "task",
      type: "creation",
      position: { x: 620, y: 200 },
      data: {
        title: "生成任务",
        detail: "样例任务 / 可查看状态",
        href: "/tasks/task-01",
      },
    },
  ]);
  const [nodes, , onNodesChange] = useNodesState(initial);
  const edges = [
    { id: "e1", source: "script", target: "shot" },
    { id: "e2", source: "character", target: "shot" },
    { id: "e3", source: "shot", target: "media" },
    { id: "e4", source: "shot", target: "task" },
  ];
  return (
    <div className="space-y-8">
      <PageHeading
        title="把创作关系放在一张画布上。"
        description="拖动画布和节点，查看剧本、资产、镜头、候选与任务的关系。位置只在本页保留。"
        action={
          <Button variant="secondary" asChild>
            <Link href="/poc/canvas">独立性能画布</Link>
          </Button>
        }
      />
      <div
        className="creation-canvas h-[540px] overflow-hidden rounded-2xl bg-muted/40"
        role="region"
        aria-label="创作关系画布"
      >
        <ReactFlow
          colorMode={resolvedTheme === "dark" ? "dark" : "light"}
          nodes={nodes}
          edges={edges}
          nodeTypes={nodeTypes}
          onNodesChange={onNodesChange}
          nodesDraggable={!readOnly}
          nodesConnectable={false}
          deleteKeyCode={null}
          fitView
          minZoom={0.35}
          maxZoom={1.5}
          ariaLabelConfig={{
            "controls.ariaLabel": "",
            "controls.zoomIn.ariaLabel": "放大画布",
            "controls.zoomOut.ariaLabel": "缩小画布",
            "controls.fitView.ariaLabel": "适应画布",
          }}
        >
          <Background color="var(--muted-foreground)" gap={24} />
          <div role="group" aria-label="画布缩放控制">
            <Controls showInteractive={false} />
          </div>
        </ReactFlow>
      </div>
      <p className="text-xs text-muted-foreground">
        当前 5 节点 / 4 连线。此页面验证创作关系与导航；500 / 2,000
        节点性能样本仍由独立页面测量。
      </p>
    </div>
  );
}
