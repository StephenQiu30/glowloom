"use client";

import Link from "next/link";
import { useState } from "react";
import {
  ArrowUpRight,
  Clapperboard,
  FileText,
  Sparkles,
  Video,
} from "lucide-react";
import { AssetCanvasList } from "@/app/assets/_components/asset-canvas-list";
import { AssetCreateProject } from "@/components/projects/asset-create-project";
import { defaultAssetFilters } from "@/app/assets/_components/asset-query";
import { useOptionalSession } from "@/components/auth/session";
import { useLoginDialog } from "@/components/layout/basic-layout";
import { useSpace } from "@/components/spaces/space-context";
import { Button } from "@/components/ui/button";
import {
  Card,
  CardDescription,
  CardHeader,
  CardTitle,
} from "@/components/ui/card";
import { Tabs, TabsContent, TabsList, TabsTrigger } from "@/components/ui/tabs";

import { StudioComposer } from "./studio-composer";
import { saveStudioInput, type StudioInput } from "./studio-input";
import styles from "./studio.module.css";

const modes = [
  { id: "upload", label: "上传剧本", icon: FileText },
  { id: "create", label: "剧本创作", icon: Sparkles },
  { id: "redraw", label: "转绘", icon: Video },
] as const;

const features = [
  { label: "剧本创作", description: "从灵感到故事" },
  { label: "短剧画布", description: "在无限画布自由编排" },
  { label: "短剧导演台", description: "排演布景、动作与机位" },
  { label: "智能预演", description: "串联关键镜头" },
  { label: "片段重拍", description: "局部修改视频" },
  { label: "重制转绘", description: "从源片制作新版本" },
  { label: "超分工作流", description: "提升成片画质" },
  { label: "视频拉片", description: "拆解画面与镜头语言" },
];

const projectTabs = {
  upload: "script",
  create: "create",
  redraw: "video",
} as const;

const allDramas = "/assets?view=short_drama";
const noop = () => {};

export function ShortDramaStudio() {
  const session = useOptionalSession();
  const { selected } = useSpace();
  return (
    <StudioWorkspace
      key={`${session?.actor.id ?? "guest"}/${selected?.id ?? ""}`}
    />
  );
}

function StudioWorkspace() {
  const session = useOptionalSession();
  const { selected, pending } = useSpace();
  const login = useLoginDialog();
  const [creating, setCreating] = useState<"canvas" | StudioInput | null>(null);
  const [mode, setMode] = useState<string>("create");
  const [locked, setLocked] = useState(false);

  const owner =
    session && selected ? `${session.actor.id}/${selected.id}` : undefined;
  const createDisabled =
    locked || Boolean(session && (pending || !selected?.writable));

  return (
    <div
      className={`${styles.studio} mx-auto flex w-full max-w-6xl flex-col gap-8 pb-8 md:gap-10`}
    >
      <section
        aria-labelledby="short-drama-title"
        className={`${styles.hero} mx-auto flex w-full max-w-[1000px] flex-col gap-4 md:gap-6`}
      >
        <h1
          id="short-drama-title"
          className="text-center text-2xl font-semibold md:text-3xl"
        >
          和短剧 Agent 聊聊创作想法
        </h1>
        <Tabs value={mode} onValueChange={setMode} className="min-w-0 gap-0">
          <div className="flex min-w-0 items-center justify-between gap-1 rounded-t-2xl border bg-muted px-1.5 py-1 md:px-3">
            <TabsList
              aria-label="短剧创作方式"
              className="min-w-0 shrink-0 bg-transparent"
            >
              {modes.map(({ id, label, icon: Icon }) => (
                <TabsTrigger
                  key={id}
                  value={id}
                  className="px-1.5 text-xs sm:px-3 sm:text-sm"
                >
                  <Icon aria-hidden="true" className="hidden sm:block" />
                  {label}
                </TabsTrigger>
              ))}
            </TabsList>
            <Button
              type="button"
              variant="ghost"
              size="sm"
              className="gap-1 px-1.5 text-xs sm:px-3 sm:text-sm"
              disabled={createDisabled}
              onClick={() => (session ? setCreating("canvas") : login())}
            >
              <ArrowUpRight data-icon="inline-start" aria-hidden="true" />
              短剧画布
            </Button>
          </div>
          {modes.map(({ id }) => (
            <TabsContent key={id} value={id} forceMount hidden={mode !== id}>
              <StudioComposer
                mode={id}
                disabled={createDisabled}
                signedIn={Boolean(session)}
                onStart={(input) => (session ? setCreating(input) : login())}
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
          {features.map(({ label, description }) => (
            <Card
              key={label}
              size="sm"
              className="rounded-xl bg-muted/50 ring-0"
            >
              <CardHeader>
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
          <div className="flex flex-wrap items-center gap-3 rounded-xl bg-muted/50 p-3">
            <Clapperboard
              className="size-4 text-muted-foreground"
              aria-hidden="true"
            />
            <p className="flex-1 text-sm text-muted-foreground">
              登录查看你的短剧
            </p>
            <Button size="sm" variant="outline" onClick={login}>
              登录 / 注册
            </Button>
          </div>
        )}
      </section>
      <section
        aria-labelledby="short-drama-featured"
        className="flex flex-wrap items-center justify-between gap-2"
      >
        <h2 id="short-drama-featured" className="text-base font-semibold">
          精选作品
        </h2>
        <Button asChild variant="ghost" size="sm">
          <Link href="/explore">
            前往探索 <ArrowUpRight data-icon="inline-end" aria-hidden="true" />
          </Link>
        </Button>
      </section>
      {creating && (
        <AssetCreateProject
          kind="short_drama"
          onClose={() => setCreating(null)}
          onLock={setLocked}
          destination={
            creating === "canvas" || !owner
              ? undefined
              : (projectID) => {
                  saveStudioInput(owner, projectID, creating);
                  return `/short-drama/${projectID}?tab=${projectTabs[creating.kind]}`;
                }
          }
        />
      )}
    </div>
  );
}
