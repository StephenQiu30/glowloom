"use client";

import Link from "next/link";
import { useState } from "react";
import {
  Aperture,
  ArrowUpRight,
  Clapperboard,
  FileText,
  Film,
  PanelsTopLeft,
  ScanLine,
  Sparkles,
  Video,
} from "lucide-react";
import { AssetCanvasList } from "@/app/assets/_components/asset-canvas-list";
import { AssetCreateProject } from "@/app/assets/_components/asset-create-project";
import { defaultAssetFilters } from "@/app/assets/_components/asset-query";
import { useOptionalSession } from "@/components/auth/session";
import { useLoginDialog } from "@/components/layout/basic-layout";
import { ContentState } from "@/components/layout/content-state";
import { useSpace } from "@/components/spaces/space-context";
import { Button } from "@/components/ui/button";
import {
  Card,
  CardDescription,
  CardHeader,
  CardTitle,
} from "@/components/ui/card";
import { Tabs, TabsContent, TabsList, TabsTrigger } from "@/components/ui/tabs";

const modes = [
  {
    id: "upload",
    label: "上传剧本",
    title: "从已有剧本开始",
    description: "上传剧本文件或粘贴文本，整理为可继续创作的剧集。",
    icon: FileText,
  },
  {
    id: "create",
    label: "剧本创作",
    title: "从创作想法开始",
    description: "用创作要求、小说或参考素材构思短剧剧本。",
    icon: Sparkles,
  },
  {
    id: "redraw",
    label: "转绘",
    title: "从现有视频开始",
    description: "基于源视频，按风格、目标地区和画面比例制作新版本。",
    icon: Video,
  },
] as const;

const features = [
  { label: "剧本创作", description: "从灵感到故事", icon: FileText },
  { label: "短剧画布", description: "在无限画布自由编排", icon: PanelsTopLeft },
  { label: "短剧导演台", description: "排演布景、动作与机位", icon: Aperture },
  { label: "智能预演", description: "串联关键镜头", icon: Film },
  { label: "片段重拍", description: "局部修改视频", icon: Clapperboard },
  { label: "重制转绘", description: "从源片制作新版本", icon: Video },
  { label: "超分工作流", description: "提升成片画质", icon: Sparkles },
  { label: "视频拉片", description: "拆解画面与镜头语言", icon: ScanLine },
];

const allDramas = "/assets?view=short_drama";
const noop = () => {};

export function ShortDramaStudio() {
  const session = useOptionalSession();
  const { selected, pending } = useSpace();
  const login = useLoginDialog();
  const [creating, setCreating] = useState<
    "canvas" | "upload" | "create" | "redraw" | null
  >(null);
  const [locked, setLocked] = useState(false);

  const createDisabled =
    locked || Boolean(session && (pending || !selected?.writable));

  return (
    <div className="mx-auto flex w-full max-w-6xl flex-col gap-8 pb-8">
      <section
        aria-labelledby="short-drama-title"
        className="mx-auto flex w-full max-w-5xl flex-col gap-5"
      >
        <h1
          id="short-drama-title"
          className="text-center text-2xl font-semibold md:text-3xl"
        >
          和短剧 Agent 聊聊创作想法
        </h1>
        <Tabs
          defaultValue="create"
          className="gap-0 overflow-hidden rounded-xl bg-card"
        >
          <div className="flex min-w-0 flex-wrap items-center justify-between gap-1 bg-muted p-2">
            <TabsList aria-label="短剧创作方式" className="grow md:grow-0">
              {modes.map(({ id, label }) => (
                <TabsTrigger
                  key={id}
                  value={id}
                  className="min-h-9 md:min-w-28"
                >
                  {label}
                </TabsTrigger>
              ))}
            </TabsList>
            <Button
              type="button"
              variant="ghost"
              disabled={createDisabled}
              onClick={() => (session ? setCreating("canvas") : login())}
            >
              <ArrowUpRight data-icon="inline-start" aria-hidden="true" />
              短剧画布
            </Button>
          </div>
          {modes.map(({ id, title, description, icon }) => (
            <TabsContent key={id} value={id} className="px-4 py-6 md:px-6">
              <ContentState
                icon={icon}
                title={title}
                description={description}
                actions={
                  <Button
                    disabled={createDisabled}
                    onClick={() => (session ? setCreating(id) : login())}
                  >
                    {id === "upload"
                      ? "开始导入"
                      : id === "redraw"
                        ? "开始转绘"
                        : "开始创作"}
                  </Button>
                }
              />
            </TabsContent>
          ))}
        </Tabs>
      </section>

      <section
        aria-labelledby="short-drama-features"
        className="flex flex-col gap-3"
      >
        <h2 id="short-drama-features" className="text-base font-semibold">
          亮点功能
        </h2>
        <div className="grid grid-cols-2 gap-2 lg:grid-cols-4">
          {features.map(({ label, description, icon: Icon }) => (
            <Card key={label} size="sm">
              <CardHeader>
                <Icon
                  className="size-4 text-muted-foreground"
                  aria-hidden="true"
                />
                <CardTitle>{label}</CardTitle>
                <CardDescription>{description}</CardDescription>
              </CardHeader>
            </Card>
          ))}
        </div>
      </section>

      <section
        aria-labelledby="short-drama-projects"
        className="flex flex-col gap-3"
      >
        <div className="flex items-center justify-between gap-2">
          <h2 id="short-drama-projects" className="text-base font-semibold">
            我的短剧
          </h2>
          <Button asChild variant="ghost" size="sm">
            <Link href={allDramas}>全部</Link>
          </Button>
        </div>
        {session && selected ? (
          <AssetCanvasList
            key={selected.id}
            kind="short_drama"
            collection="current"
            filters={defaultAssetFilters}
            recent
            onClear={noop}
            onLock={setLocked}
            onBatchMode={noop}
            toolbarTarget={null}
          />
        ) : (
          <ContentState
            icon={Clapperboard}
            title="登录查看你的短剧"
            actions={<Button onClick={login}>登录 / 注册</Button>}
          />
        )}
      </section>
      {creating && (
        <AssetCreateProject
          kind="short_drama"
          onClose={() => setCreating(null)}
          onLock={setLocked}
          destination={
            creating === "canvas"
              ? undefined
              : (projectID) =>
                  `/short-drama/${projectID}?tab=${creating === "upload" ? "script" : creating === "redraw" ? "video" : "create"}`
          }
        />
      )}
    </div>
  );
}
