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
import { Card } from "@/components/ui/card";
import { Toolbar } from "@/components/ui/toolbar";
import { Tabs, TabsContent, TabsList, TabsTrigger } from "@/components/ui/tabs";

import { StudioComposer } from "./studio-composer";
import { saveStudioInput, type StudioInput } from "./studio-input";

const modes = [
  { id: "upload", label: "上传剧本", icon: FileText },
  { id: "create", label: "剧本创作", icon: Sparkles },
  { id: "redraw", label: "转绘", icon: Video },
] as const;

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
    <div className="flex w-full min-w-0 flex-col gap-8 pb-8 md:gap-10">
      <section
        aria-labelledby="short-drama-title"
        className="flex w-full flex-col gap-4"
      >
        <h1 id="short-drama-title" className="sr-only">
          短剧工作室
        </h1>
        <Card className="gap-0 p-0">
          <Tabs value={mode} onValueChange={setMode} className="min-w-0 gap-0">
            <Toolbar className="min-w-0 flex-wrap justify-between px-3 pt-3 md:px-4 md:pt-4">
              <TabsList aria-label="短剧创作方式">
                {modes.map(({ id, label, icon: Icon }) => (
                  <TabsTrigger key={id} value={id}>
                    <Icon aria-hidden="true" className="hidden sm:block" />
                    {label}
                  </TabsTrigger>
                ))}
              </TabsList>
              <Button
                type="button"
                variant="ghost"
                size="toolbar"
                className="ml-auto shrink-0"
                disabled={createDisabled}
                onClick={() => (session ? setCreating("canvas") : login())}
              >
                <ArrowUpRight data-icon="inline-start" aria-hidden="true" />
                短剧画布
              </Button>
            </Toolbar>
            {modes.map(({ id }) => (
              <TabsContent key={id} value={id} forceMount hidden={mode !== id}>
                <StudioComposer
                  mode={id}
                  disabled={createDisabled}
                  owner={
                    session && selected
                      ? { actorId: session.actor.id, scopeId: selected.id }
                      : undefined
                  }
                  onAuthenticate={login}
                  onStart={(input) => (session ? setCreating(input) : login())}
                />
              </TabsContent>
            ))}
          </Tabs>
        </Card>
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
          <Link href="/explore?category=work.drama">
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
