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
  useSyncExternalStore,
} from "react";

import { Button } from "@/components/ui/button";
import {
  makePocGraph,
  mediaSelectionError,
  type PocMedia,
  type PocNode,
} from "./poc-data";
import { PocRecorder } from "./poc-recorder";

const scenarios = [
  { label: "500 节点 · 600 连线", nodes: 500, edges: 600 },
  { label: "500 节点 · 800 连线", nodes: 500, edges: 800 },
  { label: "2,000 节点 · 3,200 连线", nodes: 2000, edges: 3200 },
] as const;

const CanvasMoving = createContext(false);
const CanvasPlaybackPaused = createContext(false);
const CanvasOverview = createContext(false);

function subscribeReducedMotion(listener: () => void) {
  const query = window.matchMedia("(prefers-reduced-motion: reduce)");
  query.addEventListener("change", listener);
  return () => query.removeEventListener("change", listener);
}
const getReducedMotion = () =>
  window.matchMedia("(prefers-reduced-motion: reduce)").matches;

const PocCard = memo(function PocCard({ data, selected }: NodeProps<PocNode>) {
  const Icon =
    data.kind === "image" ? ImageIcon : data.kind === "video" ? Video : Type;
  const moving = useContext(CanvasMoving);
  const paused = useContext(CanvasPlaybackPaused);
  const overview = useContext(CanvasOverview);
  const videoRef = useRef<HTMLVideoElement>(null);

  useEffect(() => {
    if (!data.playing || moving || paused || overview) {
      videoRef.current?.pause();
      return;
    }
    void videoRef.current?.play().catch(() => {
      // If autoplay is denied, the poster remains visible.
    });
  }, [data.playing, moving, paused, overview]);
  return (
    <div className="group relative w-[280px] select-none">
      <div className="absolute -top-7 left-3 flex max-w-[260px] items-center gap-1.5 text-[11px] font-medium text-slate-600">
        <Icon aria-hidden="true" className="size-3" />
        <span className="truncate">{data.title}</span>
      </div>
      <div
        className={`relative h-[170px] overflow-hidden rounded-3xl border-2 bg-white shadow-[0_14px_40px_rgba(28,40,49,.13)] transition-[border-color,box-shadow] ${selected ? "border-[#2f80ff] shadow-[0_16px_44px_rgba(47,128,255,.24)]" : "border-white group-hover:border-slate-300"}`}
      >
        {overview ? (
          <div className="flex h-full items-center justify-center bg-slate-100 text-sm text-slate-600">
            <Icon aria-hidden="true" className="mr-2 size-5" />
            {data.title}
          </div>
        ) : data.kind === "image" ? (
          // The shared local fixture isolates DOM and layout costs from network variability.
          // eslint-disable-next-line @next/next/no-img-element
          <img
            src={data.src ?? "/poc/scene.svg"}
            alt={data.localMedia ? data.title : "山间场景样例"}
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
              poster={data.localMedia ? undefined : "/poc/scene.svg"}
              src={data.src ?? "/poc/preview.mp4"}
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
  media,
  mediaSummary,
  paused,
}: {
  nodeCount: number;
  edgeCount: number;
  media: PocMedia[];
  mediaSummary: { images: number; videos: number; bytes: number };
  paused: boolean;
}) {
  const graph = useMemo(
    () => makePocGraph(nodeCount, edgeCount, media),
    [nodeCount, edgeCount, media],
  );
  const [selected, setSelected] = useState<string | null>(null);
  const [moving, setMoving] = useState(false);
  const [overview, setOverview] = useState(false);
  const movingRef = useRef(false);
  const lastMotion = useRef(0);
  const inputCount = useRef(0);
  const region = useRef<HTMLDivElement>(null);
  function setMotion(value: boolean) {
    movingRef.current = value;
    setMoving(value);
  }
  return (
    <div className="flex h-full min-h-[560px] flex-col">
      <PocRecorder
        moving={movingRef}
        lastMotion={lastMotion}
        inputs={inputCount}
        region={region}
        nodes={nodeCount}
        edges={edgeCount}
        media={mediaSummary}
      />
      <div
        ref={region}
        className="relative min-h-[460px] flex-1 overflow-hidden rounded-[28px] border border-slate-200 bg-[#f8faf9]"
        data-scene={`${nodeCount}-${edgeCount}`}
        onWheelCapture={(event) => {
          if (event.isTrusted) inputCount.current += 1;
        }}
        onPointerMoveCapture={(event) => {
          if (event.isTrusted && event.buttons !== 0) inputCount.current += 1;
        }}
        onKeyDownCapture={(event) => {
          if (
            event.isTrusted &&
            ["ArrowUp", "ArrowDown", "ArrowLeft", "ArrowRight"].includes(
              event.key,
            )
          )
            inputCount.current += 1;
        }}
      >
        <CanvasMoving.Provider value={moving}>
          <CanvasPlaybackPaused.Provider value={paused}>
            <CanvasOverview.Provider value={overview}>
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
                onMoveStart={() => setMotion(true)}
                onMove={(_, viewport) => {
                  lastMotion.current = performance.now();
                  setOverview(viewport.zoom < 0.35);
                }}
                onMoveEnd={() => setMotion(false)}
                onNodeDragStart={() => setMotion(true)}
                onNodeDrag={() => {
                  lastMotion.current = performance.now();
                }}
                onNodeDragStop={() => setMotion(false)}
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
            </CanvasOverview.Provider>
          </CanvasPlaybackPaused.Provider>
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
    </div>
  );
}

export function PocCanvas() {
  const [scenario, setScenario] = useState(1);
  const [media, setMedia] = useState<PocMedia[]>([]);
  const [mediaBytes, setMediaBytes] = useState(0);
  const [mediaError, setMediaError] = useState<string | null>(null);
  const [paused, setPaused] = useState(false);
  const ownedURLs = useRef<string[]>([]);
  const reducedMotion = useSyncExternalStore(
    subscribeReducedMotion,
    getReducedMotion,
    () => true,
  );
  useEffect(
    () => () => ownedURLs.current.forEach((url) => URL.revokeObjectURL(url)),
    [],
  );
  function selectMedia(files: File[]) {
    const error = mediaSelectionError(files);
    setMediaError(error);
    if (error) return;
    const next = files.map((file): PocMedia => ({
      kind: file.type.startsWith("image/") ? "image" : "video",
      url: URL.createObjectURL(file),
    }));
    ownedURLs.current.forEach((url) => URL.revokeObjectURL(url));
    ownedURLs.current = next.map((item) => item.url);
    setMedia(next);
    setMediaBytes(files.reduce((sum, file) => sum + file.size, 0));
  }
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
        <div className="flex items-center gap-3">
          <Link href="/canvas" className="text-xs underline underline-offset-4">
            真实备注画布
          </Link>
          <Link
            href="/licenses"
            className="text-xs underline underline-offset-4"
          >
            开源许可
          </Link>
        </div>
      </header>
      <div className="flex flex-wrap items-center gap-3 bg-white px-5 py-3 text-xs">
        <label className="flex items-center gap-2" htmlFor="poc-media">
          自选差异素材
          <input
            id="poc-media"
            name="poc-media"
            type="file"
            accept="image/png,image/jpeg,image/webp,image/avif,video/mp4,video/webm"
            multiple
            onChange={(event) => {
              selectMedia(Array.from(event.currentTarget.files ?? []));
              event.currentTarget.value = "";
            }}
            className="max-w-64 rounded file:mr-2 file:rounded file:border-0 file:bg-slate-100 file:px-3 file:py-2 focus-visible:outline-2 focus-visible:outline-blue-500"
          />
        </label>
        <Button
          variant="outline"
          size="sm"
          disabled={media.length === 0}
          onClick={() => selectMedia([])}
        >
          恢复固定样本
        </Button>
        <Button
          variant="outline"
          size="sm"
          disabled={reducedMotion}
          onClick={() => setPaused(!paused)}
        >
          {paused || reducedMotion ? "播放视频预览" : "暂停视频预览"}
        </Button>
        <p className="min-w-0 flex-1 leading-5" role="status">
          {mediaError ??
            (media.length
              ? `已选择 ${media.filter((item) => item.kind === "image").length} 张图 / ${media.filter((item) => item.kind === "video").length} 个视频；未提供的类型仍使用固定样本。`
              : "固定样本作为对照；自选素材仅在本页内存中使用，不上传。")}
          {reducedMotion ? " 已按减少动态效果偏好暂停视频。" : ""}
        </p>
      </div>
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
          <ReactFlowProvider
            key={`${current.nodes}-${current.edges}-${media.map((item) => item.url).join()}`}
          >
            <Scene
              nodeCount={current.nodes}
              edgeCount={current.edges}
              media={media}
              mediaSummary={{
                images: media.filter((item) => item.kind === "image").length,
                videos: media.filter((item) => item.kind === "video").length,
                bytes: mediaBytes,
              }}
              paused={paused || reducedMotion}
            />
          </ReactFlowProvider>
        </div>
      </div>
    </main>
  );
}
