"use client";

import {
  Background,
  BackgroundVariant,
  Handle,
  MiniMap,
  Position,
  ReactFlow,
  ReactFlowProvider,
  useReactFlow,
  type NodeProps,
} from "@xyflow/react";
import {
  Focus,
  Hand,
  ImageIcon,
  Minus,
  Play,
  Plus,
  Type,
  Video,
} from "lucide-react";
import Link from "next/link";
import {
  createContext,
  memo,
  useContext,
  useEffect,
  useMemo,
  useRef,
  useState,
} from "react";

import { makePocGraph, type PocNode } from "./poc-data";

const scenarios = [
  { label: "500 节点 · 600 连线", nodes: 500, edges: 600 },
  { label: "500 节点 · 800 连线", nodes: 500, edges: 800 },
  { label: "2,000 节点 · 3,200 连线", nodes: 2000, edges: 3200 },
] as const;

const CanvasMoving = createContext(false);

const PocCard = memo(function PocCard({ data, selected }: NodeProps<PocNode>) {
  const Icon =
    data.kind === "image" ? ImageIcon : data.kind === "video" ? Video : Type;
  const moving = useContext(CanvasMoving);
  const videoRef = useRef<HTMLVideoElement>(null);

  useEffect(() => {
    if (!data.playing || moving) {
      videoRef.current?.pause();
      return;
    }
    void videoRef.current?.play().catch(() => {
      // If autoplay is denied, the poster remains visible.
    });
  }, [data.playing, moving]);
  return (
    <div className="group relative w-[280px] select-none">
      <div className="absolute -top-7 left-3 flex max-w-[260px] items-center gap-1.5 text-[11px] font-medium text-slate-600">
        <Icon aria-hidden="true" className="size-3" />
        <span className="truncate">{data.title}</span>
      </div>
      <div
        className={`relative h-[170px] overflow-hidden rounded-3xl border-2 bg-white shadow-[0_14px_40px_rgba(28,40,49,.13)] transition-[border-color,box-shadow] ${selected ? "border-[#2f80ff] shadow-[0_16px_44px_rgba(47,128,255,.24)]" : "border-white group-hover:border-slate-300"}`}
      >
        {data.kind === "image" ? (
          // The shared local fixture isolates DOM and layout costs from network variability.
          // eslint-disable-next-line @next/next/no-img-element
          <img
            src="/poc/scene.svg"
            alt="山间场景样例"
            width={280}
            height={170}
            className="h-full w-full object-cover"
            draggable={false}
          />
        ) : data.kind === "video" ? (
          <div className="relative h-full w-full bg-slate-900">
            <video
              ref={videoRef}
              className="h-full w-full object-cover"
              poster="/poc/scene.svg"
              src="/poc/preview.mp4"
              preload="none"
              muted
              loop
              playsInline
              aria-label={`${data.title}样例视频`}
            />
            {!data.playing && (
              <Play
                aria-hidden="true"
                className="pointer-events-none absolute top-1/2 left-1/2 size-8 -translate-x-1/2 -translate-y-1/2 fill-white text-white drop-shadow-lg"
              />
            )}
          </div>
        ) : (
          <div className="flex h-full flex-col justify-between bg-[#f2f0e9] p-5 text-slate-700">
            <span className="text-[11px] font-semibold tracking-[.18em] text-slate-500">
              STORY NOTE
            </span>
            <p className="max-w-52 text-base leading-snug font-medium">
              光线落在山脊上，镜头缓缓推进。
            </p>
            <span className="text-[11px] text-slate-500">
              场景节奏 / 分镜参考
            </span>
          </div>
        )}
      </div>
      <Handle
        type="target"
        position={Position.Left}
        className="!size-2.5 !border-2 !border-white !bg-[#2f80ff]"
      />
      <Handle
        type="source"
        position={Position.Right}
        className="!size-2.5 !border-2 !border-white !bg-[#2f80ff]"
      />
    </div>
  );
});

const nodeTypes = { pocCard: PocCard };

function CanvasDock() {
  const { zoomIn, zoomOut, fitView } = useReactFlow();
  return (
    <div className="absolute bottom-6 left-1/2 z-10 flex -translate-x-1/2 items-center gap-1 rounded-2xl border border-slate-200/80 bg-white/95 p-1.5 shadow-[0_12px_40px_rgba(28,40,49,.16)] backdrop-blur">
      <span className="flex items-center gap-2 rounded-xl bg-slate-900 px-3 py-2 text-xs font-medium text-white">
        <Hand aria-hidden="true" className="size-3.5" /> 平移画布
      </span>
      <span className="mx-1 h-5 w-px bg-slate-200" />
      <button
        type="button"
        onClick={() => void zoomOut()}
        aria-label="缩小"
        className="rounded-lg p-2 hover:bg-slate-100 focus-visible:outline-2 focus-visible:outline-blue-500"
      >
        <Minus aria-hidden="true" className="size-4" />
      </button>
      <button
        type="button"
        onClick={() => void zoomIn()}
        aria-label="放大"
        className="rounded-lg p-2 hover:bg-slate-100 focus-visible:outline-2 focus-visible:outline-blue-500"
      >
        <Plus aria-hidden="true" className="size-4" />
      </button>
      <button
        type="button"
        onClick={() => void fitView({ duration: 250 })}
        aria-label="适配全部节点"
        className="rounded-lg p-2 hover:bg-slate-100 focus-visible:outline-2 focus-visible:outline-blue-500"
      >
        <Focus aria-hidden="true" className="size-4" />
      </button>
    </div>
  );
}

function Scene({
  nodeCount,
  edgeCount,
}: {
  nodeCount: number;
  edgeCount: number;
}) {
  const graph = useMemo(
    () => makePocGraph(nodeCount, edgeCount),
    [nodeCount, edgeCount],
  );
  const [selected, setSelected] = useState<string | null>(null);
  const [moving, setMoving] = useState(false);
  return (
    <div
      className="relative h-full min-h-[560px] overflow-hidden rounded-[28px] border border-slate-200 bg-[#f8faf9]"
      data-scene={`${nodeCount}-${edgeCount}`}
    >
      <CanvasMoving.Provider value={moving}>
        <ReactFlow
          defaultNodes={graph.nodes}
          defaultEdges={graph.edges}
          nodeTypes={nodeTypes}
          defaultViewport={{ x: 70, y: 70, zoom: 0.85 }}
          minZoom={0.15}
          maxZoom={1.8}
          onlyRenderVisibleElements
          panOnDrag
          selectionOnDrag={false}
          onNodeClick={(_, node) => setSelected(node.data.title)}
          onPaneClick={() => setSelected(null)}
          onMoveStart={() => setMoving(true)}
          onMoveEnd={() => setMoving(false)}
          onInit={() => {
            requestAnimationFrame(() =>
              requestAnimationFrame(() =>
                performance.mark(
                  `lanverse-canvas-ready-${nodeCount}-${edgeCount}`,
                ),
              ),
            );
          }}
          colorMode="light"
          className="!bg-[#f8faf9]"
        >
          <Background
            variant={BackgroundVariant.Dots}
            color="#cbd5d1"
            gap={24}
            size={1}
          />
          <MiniMap
            position="bottom-right"
            pannable
            zoomable
            nodeColor="#7d9d9a"
            maskColor="rgba(245,248,246,.65)"
            className="!right-5 !bottom-5 !rounded-xl !border !border-slate-200 !bg-white !shadow-sm"
          />
          <CanvasDock />
        </ReactFlow>
      </CanvasMoving.Provider>
      <aside
        aria-live="polite"
        className="pointer-events-none absolute top-5 right-5 z-10 w-48 rounded-2xl border border-slate-200 bg-white/95 p-4 text-xs shadow-sm backdrop-blur"
      >
        <p className="font-semibold text-slate-900">节点属性</p>
        <p className="mt-2 leading-5 text-slate-500">
          {selected ?? "选择节点后查看标题"}
        </p>
      </aside>
    </div>
  );
}

export function PocCanvas() {
  const [scenario, setScenario] = useState(1);
  const current = scenarios[scenario];
  return (
    <main className="flex min-h-screen flex-col bg-[#f3f5f3] text-slate-900">
      <header className="flex flex-wrap items-center justify-between gap-4 border-b border-slate-200 bg-white px-5 py-4 lg:px-8">
        <div>
          <p className="font-mono text-[10px] font-semibold tracking-[.22em] text-slate-500">
            LANVERSE / CANVAS POC
          </p>
          <h1 className="mt-1 text-lg font-semibold tracking-tight">
            无限画布性能验证
          </h1>
        </div>
        <div
          className="flex flex-wrap items-center gap-2"
          role="group"
          aria-label="测试场景"
        >
          {scenarios.map((item, index) => (
            <button
              key={item.label}
              type="button"
              onClick={() => setScenario(index)}
              aria-pressed={scenario === index}
              className={`rounded-xl border px-3 py-2 text-xs font-medium transition-colors focus-visible:outline-2 focus-visible:outline-blue-500 ${scenario === index ? "border-slate-900 bg-slate-900 text-white" : "border-slate-200 bg-white text-slate-600 hover:border-slate-400"}`}
            >
              {item.label}
            </button>
          ))}
        </div>
        <Link
          href="/licenses"
          className="text-xs text-slate-500 underline-offset-4 hover:text-slate-900 hover:underline"
        >
          开源许可
        </Link>
      </header>
      <div className="flex min-h-0 flex-1 gap-4 p-4 lg:p-6">
        <aside className="hidden w-44 shrink-0 rounded-2xl border border-slate-200 bg-white p-4 lg:block">
          <p className="text-xs font-semibold text-slate-900">节点库</p>
          <div className="mt-5 space-y-2 text-xs text-slate-600">
            <p className="flex items-center gap-2 rounded-lg bg-slate-50 p-2">
              <ImageIcon aria-hidden="true" className="size-4" /> 图像节点
            </p>
            <p className="flex items-center gap-2 rounded-lg bg-slate-50 p-2">
              <Video aria-hidden="true" className="size-4" /> 视频节点
            </p>
            <p className="flex items-center gap-2 rounded-lg bg-slate-50 p-2">
              <Type aria-hidden="true" className="size-4" /> 文本节点
            </p>
          </div>
          <p className="mt-8 text-[11px] leading-5 text-slate-500">
            固定数据集，仅用于技术验证。拖动画布并用滚轮缩放。
          </p>
        </aside>
        <div className="h-[calc(100vh-112px)] min-h-[560px] min-w-0 flex-1">
          <ReactFlowProvider key={`${current.nodes}-${current.edges}`}>
            <Scene nodeCount={current.nodes} edgeCount={current.edges} />
          </ReactFlowProvider>
        </div>
      </div>
    </main>
  );
}
