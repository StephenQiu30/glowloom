"use client";

import { useRef, useState, type ReactNode } from "react";
import Link from "next/link";
import {
  ArrowLeft,
  ArrowUp,
  Bell,
  Check,
  Clapperboard,
  Download,
  Folder,
  Grid2X2,
  Hand,
  ImageIcon,
  Maximize,
  Minus,
  MousePointer2,
  Music2,
  Plus,
  Sparkles,
  Type,
  UserRound,
  Video,
} from "lucide-react";
import { cn } from "cn";
import { Button } from "@/components/ui/button";
import { Badge } from "@/components/ui/badge";
import { Avatar, AvatarFallback } from "@/components/ui/avatar";
import { Card, CardContent } from "@/components/ui/card";
import { InputGroup, InputGroupTextarea } from "@/components/ui/input-group";
import { TooltipProvider } from "@/components/ui/tooltip";
import { ToggleGroup, ToggleGroupItem } from "@/components/ui/toggle-group";
import {
  Table,
  TableBody,
  TableCell,
  TableHead,
  TableHeader,
  TableRow,
} from "@/components/ui/table";
import {
  Brand,
  Choice,
  IconButton,
  Placeholder,
  demoNotice,
  LocalDialog,
} from "./shared";
import { previewHref } from "./screens";
import { shotFixtures } from "./storyboard-pages";

function CanvasNode({
  id,
  title,
  x,
  y,
  width,
  children,
  selected,
  onSelect,
  offset,
  onMove,
}: {
  id: string;
  title: string;
  x: number;
  y: number;
  width: number;
  children: ReactNode;
  selected: boolean;
  onSelect: (id: string) => void;
  offset: { x: number; y: number };
  onMove: (id: string, delta: { x: number; y: number }) => void;
}) {
  const origin = useRef({ x: 0, y: 0 });
  return (
    <section
      className="absolute"
      style={{ left: x + offset.x, top: y + offset.y, width }}
      aria-label={title}
    >
      <button
        type="button"
        draggable
        onDragStart={(e) => {
          origin.current = { x: e.clientX, y: e.clientY };
          e.dataTransfer.effectAllowed = "move";
        }}
        onDragEnd={(e) => {
          if (e.clientX || e.clientY)
            onMove(id, {
              x: e.clientX - origin.current.x,
              y: e.clientY - origin.current.y,
            });
        }}
        onClick={() => onSelect(id)}
        onKeyDown={(e) => {
          const delta: Record<string, { x: number; y: number }> = {
            ArrowLeft: { x: -10, y: 0 },
            ArrowRight: { x: 10, y: 0 },
            ArrowUp: { x: 0, y: -10 },
            ArrowDown: { x: 0, y: 10 },
          };
          if (delta[e.key]) {
            e.preventDefault();
            onMove(id, delta[e.key]);
          }
        }}
        className="mb-2 flex w-full cursor-grab items-center gap-2 text-left text-xs text-muted-foreground focus-visible:outline focus-visible:outline-ring"
      >
        <Grid2X2 className="size-3" />
        {title}
      </button>
      <div
        className={cn(
          "rounded-xl bg-surface-2 p-3 shadow-sm",
          selected && "outline outline-offset-4 outline-primary",
        )}
      >
        {children}
      </div>
    </section>
  );
}
const zero = { x: 0, y: 0 };
export function CanvasPreviewPage() {
  const [zoom, setZoom] = useState(72);
  const [tool, setTool] = useState("选择");
  const [selected, setSelected] = useState("shot");
  const [prompt, setPrompt] = useState("");
  const [mode, setMode] = useState("视频");
  const [model, setModel] = useState("[视频模型]");
  const [ratio, setRatio] = useState("9:16");
  const [duration, setDuration] = useState("4s");
  const [offsets, setOffsets] = useState<
    Record<string, { x: number; y: number }>
  >({});
  const [agent, setAgent] = useState(false);
  const [playing, setPlaying] = useState(false);
  const [generated, setGenerated] = useState(false);
  function move(id: string, delta: { x: number; y: number }) {
    setOffsets((current) => ({
      ...current,
      [id]: {
        x: (current[id]?.x ?? 0) + (delta.x * 72) / zoom,
        y: (current[id]?.y ?? 0) + (delta.y * 72) / zoom,
      },
    }));
  }
  const nodeProps = { selected: false, onSelect: setSelected, onMove: move };
  return (
    <TooltipProvider>
      <main
        className="relative h-svh min-h-[720px] overflow-hidden bg-background"
        aria-label="无限画布"
      >
        <h1 className="sr-only">无限画布 · 雾港来信</h1>
        <header className="absolute top-4 right-4 left-4 z-10 flex flex-wrap justify-between gap-3">
          <div className="flex items-center gap-3 rounded-lg bg-surface-1 p-2">
            <IconButton
              icon={ArrowLeft}
              label="返回项目"
              href={previewHref("home")}
              size="icon-sm"
            />
            <Brand />
            <Link href={previewHref("analytics")} className="text-sm">
              雾港来信
            </Link>
            <span className="text-muted-foreground">/</span>
            <Button variant="ghost" size="sm" asChild>
              <Link href={previewHref("storyboard")}>第 3 集</Link>
            </Button>
          </div>
          <div className="flex items-center gap-3 rounded-lg bg-surface-1 p-2">
            <span className="hidden items-center gap-1 text-xs text-muted-foreground sm:flex">
              <Check className="size-3" />
              所有更改已保存
            </span>
            <Button size="sm" onClick={() => setAgent(true)}>
              <Sparkles data-icon="inline-start" />
              Agent
            </Button>
            <IconButton
              label="任务通知"
              icon={Bell}
              onClick={() => demoNotice("2 个任务待处理")}
            />
            <Avatar>
              <AvatarFallback>陈</AvatarFallback>
            </Avatar>
          </div>
        </header>
        <div className="absolute top-1/2 left-4 z-10 -translate-y-1/2 rounded-lg bg-surface-1 p-1.5">
          <ToggleGroup
            type="single"
            value={tool}
            onValueChange={(value) => {
              if (value) {
                setTool(value);
                if (!["选择", "移动"].includes(value))
                  demoNotice(`已选择${value}工具`);
              }
            }}
            orientation="vertical"
            aria-label="画布工具"
          >
            {[
              ["选择", MousePointer2],
              ["移动", Hand],
              ["文本", Type],
              ["图片", ImageIcon],
              ["视频", Video],
              ["音频", Music2],
              ["分镜", Clapperboard],
              ["角色", UserRound],
              ["容器", Folder],
              ["导出", Download],
            ].map(([label, Icon]) => {
              const ToolIcon = Icon as typeof Hand;
              return (
                <ToggleGroupItem
                  key={label as string}
                  value={label as string}
                  aria-label={label as string}
                  className="size-9 p-0"
                >
                  <ToolIcon />
                </ToggleGroupItem>
              );
            })}
          </ToggleGroup>
        </div>
        <div
          className="absolute inset-0 overflow-auto"
          onDragOver={(e) => e.preventDefault()}
        >
          <div
            className="relative min-h-[800px] min-w-[1380px] origin-top-left"
            style={{ transform: `scale(${zoom / 72})` }}
          >
            <svg
              className="pointer-events-none absolute inset-0 h-[800px] w-[1380px]"
              aria-hidden="true"
            >
              <path
                d="M380 235 C470 235 565 260 730 260 M380 558 C530 558 580 370 730 370"
                stroke="var(--border)"
                fill="none"
                strokeDasharray="4 4"
              />
              <path
                d="M650 224 C720 224 658 326 730 326 M650 515 C724 515 674 360 730 360 M1040 258 C1090 258 1090 210 1120 210 M1040 470 C1090 470 1080 558 1120 558"
                stroke="var(--muted-foreground)"
                fill="none"
              />
              {[
                [650, 224],
                [650, 515],
                [730, 326],
                [730, 360],
                [1040, 258],
                [1040, 470],
              ].map(([cx, cy]) => (
                <circle
                  key={`${cx}-${cy}`}
                  cx={cx}
                  cy={cy}
                  r="4"
                  fill="var(--background)"
                  stroke="var(--muted-foreground)"
                />
              ))}
            </svg>
            <CanvasNode
              {...nodeProps}
              id="table"
              title="镜头表 · 第 3 集 场 2"
              x={92}
              y={130}
              width={285}
              offset={offsets.table ?? zero}
              selected={selected === "table"}
            >
              <Table>
                <TableHeader>
                  <TableRow>
                    <TableHead>编号</TableHead>
                    <TableHead>画面</TableHead>
                    <TableHead>时长</TableHead>
                  </TableRow>
                </TableHeader>
                <TableBody>
                  {shotFixtures.slice(0, 5).map((shot) => (
                    <TableRow
                      key={shot.id}
                      data-state={shot.id === "02-04" ? "selected" : undefined}
                    >
                      <TableCell className="font-mono text-[10px]">
                        {shot.id}
                      </TableCell>
                      <TableCell className="max-w-36 truncate text-[10px]">
                        {shot.text}
                      </TableCell>
                      <TableCell className="text-[10px]">
                        {shot.duration}
                      </TableCell>
                    </TableRow>
                  ))}
                </TableBody>
              </Table>
              <p className="mt-2 text-[10px] text-muted-foreground">
                拖动一个镜头到画布以生产
              </p>
            </CanvasNode>
            <CanvasNode
              {...nodeProps}
              id="text"
              title="文本 · 氛围提示词"
              x={92}
              y={515}
              width={285}
              offset={offsets.text ?? zero}
              selected={selected === "text"}
            >
              <p className="text-xs leading-6 text-muted-foreground">
                冷蓝夜色，细密雨幕，港口的灯在积水里拉出长长的倒影，镜头缓慢推进保持稳定。
              </p>
            </CanvasNode>
            <div className="absolute top-[108px] left-[420px] h-[526px] w-[250px] rounded-xl bg-surface-1/60">
              <p className="p-3 text-xs text-muted-foreground">
                设定 · 场 2 本场所需
              </p>
            </div>
            <CanvasNode
              {...nodeProps}
              id="character"
              title="角色 · 林舟 雨夜造型"
              x={442}
              y={133}
              width={206}
              offset={offsets.character ?? zero}
              selected={selected === "character"}
            >
              <Placeholder className="h-[210px]" label="[角色定稿图]">
                <UserRound className="size-6" />
              </Placeholder>
            </CanvasNode>
            <CanvasNode
              {...nodeProps}
              id="scene"
              title="场景 · 旧码头 夜"
              x={442}
              y={432}
              width={206}
              offset={offsets.scene ?? zero}
              selected={selected === "scene"}
            >
              <Placeholder className="h-[154px]" label="[场景参考图]" />
            </CanvasNode>
            <div className="absolute top-[98px] left-[718px] flex gap-1 rounded-lg bg-surface-2 p-1">
              <Button size="sm" onClick={() => demoNotice("演示镜头已运行")}>
                <PlayIcon />
                运行
              </Button>
              <Button variant="ghost" size="sm" asChild>
                <Link href={previewHref("shot")}>镜头详情</Link>
              </Button>
              <Button variant="ghost" size="sm" asChild>
                <Link href={previewHref("storyboard")}>在分镜中定位</Link>
              </Button>
            </div>
            <CanvasNode
              {...nodeProps}
              id="shot"
              title="镜头 S03-02-04"
              x={718}
              y={154}
              width={306}
              offset={offsets.shot ?? zero}
              selected={selected === "shot"}
            >
              <Placeholder className="h-[170px]" label="[关键帧]" />
              <p className="mt-4 text-xs leading-6">
                中景推进，林舟在雨中抬头回望，冷色深水，背景灯塔虚焦。
              </p>
              <div className="mt-3 flex flex-wrap gap-1">
                {["中景", "推镜", "4.0s", "图生视频"].map((value) => (
                  <Badge key={value} variant="muted">
                    {value}
                  </Badge>
                ))}
              </div>
              <p className="mt-4 text-center text-[10px] text-muted-foreground">
                参考 2 · 图片 2/9
              </p>
            </CanvasNode>
            <CanvasNode
              {...nodeProps}
              id="candidates"
              title="视频候选　　　　　　　　2 / 2"
              x={1100}
              y={132}
              width={274}
              offset={offsets.candidates ?? zero}
              selected={selected === "candidates"}
            >
              <div className="grid grid-cols-2 gap-3">
                {["A", "B"].map((candidate) => (
                  <Link
                    key={candidate}
                    href={previewHref("shot")}
                    aria-label={`打开视频候选 ${candidate}`}
                  >
                    <Placeholder
                      className="h-[202px]"
                      kind="video"
                      label={`${candidate} · 4.0s`}
                    />
                  </Link>
                ))}
              </div>
            </CanvasNode>
            <CanvasNode
              {...nodeProps}
              id="audio"
              title="台词 · 林舟"
              x={1100}
              y={480}
              width={274}
              offset={offsets.audio ?? zero}
              selected={selected === "audio"}
            >
              <p className="mb-2 text-xs">“没封好，本来就寄不到吧。”</p>
              <Button
                variant="ghost"
                size="sm"
                onClick={() => setPlaying(!playing)}
              >
                <Music2 data-icon="inline-start" />
                {playing ? "暂停演示" : "▂ ▃ ▆ ▅ ▂ ▅ ▇ ▃ ▆ ▅ ▂"}
                <span className="ml-3 text-[10px]">2.8s</span>
              </Button>
            </CanvasNode>
          </div>
        </div>
        <div className="absolute right-5 bottom-5 left-20 flex justify-center md:left-5">
          <Card variant="composer" className="w-full max-w-[720px]">
            <CardContent className="p-4">
              <div className="mb-3 flex items-center gap-2">
                <Badge variant="secondary">林舟 雨夜</Badge>
                <Badge variant="secondary">旧码头 夜</Badge>
                <IconButton
                  label="添加引用"
                  icon={Plus}
                  size="icon-xs"
                  onClick={() => demoNotice("参考选择器已打开")}
                />
                <span className="ml-auto hidden text-[10px] text-muted-foreground sm:block">
                  作用于选中 · S03-02-04
                </span>
              </div>
              <InputGroup variant="quiet">
                <InputGroupTextarea
                  aria-label="画布提示词"
                  placeholder="描述镜头画面，输入 @ 引用设定集或本地素材…"
                  value={prompt}
                  onChange={(e) => setPrompt(e.target.value)}
                  className="min-h-14"
                />
              </InputGroup>
              <div className="mt-3 flex flex-wrap items-center gap-2">
                <ToggleGroup
                  type="single"
                  value={mode}
                  onValueChange={(value) => value && setMode(value)}
                  size="sm"
                  aria-label="生成类型"
                >
                  {["图片", "视频", "配音"].map((value) => (
                    <ToggleGroupItem key={value} value={value}>
                      {value}
                    </ToggleGroupItem>
                  ))}
                </ToggleGroup>
                <Choice
                  value={model}
                  onChange={setModel}
                  options={["[视频模型]", "[视频模型 A]", "[视频模型 B]"]}
                  label="画布模型"
                />
                <Choice
                  value={ratio}
                  onChange={setRatio}
                  options={["9:16", "16:9", "1:1"]}
                  label="画布画幅"
                />
                <Choice
                  value={duration}
                  onChange={setDuration}
                  options={["4s", "5s", "10s"]}
                  label="生成时长"
                />
                <Badge variant="secondary">×2</Badge>
                <Button
                  size="icon"
                  aria-label="生成候选"
                  className="ml-auto"
                  onClick={() => {
                    setGenerated(true);
                    demoNotice("2 个演示候选已生成");
                  }}
                >
                  {generated ? (
                    <Check data-icon="inline-start" />
                  ) : (
                    <ArrowUp data-icon="inline-start" />
                  )}
                </Button>
              </div>
            </CardContent>
          </Card>
        </div>
        <div className="absolute bottom-5 left-4 hidden items-center gap-1 rounded-lg bg-surface-1 p-1 md:flex">
          <IconButton
            label="缩小画布"
            icon={Minus}
            size="icon-xs"
            onClick={() => setZoom(Math.max(30, zoom - 10))}
          />
          <span className="w-10 text-center font-mono text-xs">{zoom}%</span>
          <IconButton
            label="放大画布"
            icon={Plus}
            size="icon-xs"
            onClick={() => setZoom(Math.min(150, zoom + 10))}
          />
          <IconButton
            label="适应画布"
            icon={Maximize}
            size="icon-xs"
            onClick={() => {
              setZoom(72);
              setOffsets({});
            }}
          />
        </div>
        <div
          className="absolute right-5 bottom-5 hidden h-28 w-44 rounded-lg bg-surface-1 p-4 xl:block"
          aria-label="画布缩略图"
        >
          <div className="grid h-full grid-cols-4 gap-2">
            {Array.from({ length: 8 }, (_, i) => (
              <span
                key={i}
                className={cn(
                  "rounded-sm bg-surface-3",
                  i === 2 && "row-span-2 bg-primary",
                )}
              />
            ))}
          </div>
        </div>
        <LocalDialog
          open={agent}
          onOpenChange={setAgent}
          title="Agent 创作助手"
          confirmLabel="生成建议"
          onConfirm={() => demoNotice("已为当前镜头生成构图建议")}
        >
          <p className="text-sm leading-6 text-muted-foreground">
            已读取当前镜头、林舟雨夜造型和旧码头场景。演示建议：保持中景缓慢推进，用冷色光线突出人物回望的情绪。
          </p>
        </LocalDialog>
      </main>
    </TooltipProvider>
  );
}
function PlayIcon() {
  return <Clapperboard data-icon="inline-start" />;
}
