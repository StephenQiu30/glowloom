"use client";

import Link from "next/link";
import { useQuery } from "@tanstack/react-query";
import {
  ArrowUpRight,
  AudioLines,
  Clapperboard,
  Image,
  LayoutDashboard,
  Plus,
  Video,
} from "lucide-react";
import { PROJECTS_KEY, listProjects } from "@/components/project/queries";
import { Alert, AlertDescription, AlertTitle } from "@/components/ui/alert";
import { Button } from "@/components/ui/button";
import { Skeleton } from "@/components/ui/skeleton";

const capabilities = [
  { label: "图片创作", mode: "image", icon: Image },
  { label: "视频创作", mode: "video", icon: Video },
  { label: "音频创作", mode: "audio", icon: AudioLines },
  { label: "文本创作", mode: "text", icon: Clapperboard },
];

export function HomeDashboard() {
  const recent = useQuery({
    queryKey: [...PROJECTS_KEY, "recent"],
    queryFn: ({ signal }) =>
      listProjects({ limit: 8, deleted: false, status: "active" }, signal),
    retry: false,
  });
  return (
    <div className="mx-auto max-w-6xl">
      <section className="flex min-h-80 flex-col justify-center gap-7 py-10">
        <p className="text-xs tracking-widest text-muted-foreground">
          AI VIDEO WORKSPACE
        </p>
        <h1 className="max-w-3xl text-4xl leading-tight font-medium tracking-tighter md:text-6xl">
          把创意，
          <br />
          连接成故事。
        </h1>
        <div className="flex flex-wrap gap-3">
          <Button asChild size="lg">
            <Link href="/projects?create=true">
              <Plus />
              新建项目
            </Link>
          </Button>
          <Button asChild variant="outline" size="lg">
            <Link href="/canvas">
              <LayoutDashboard />
              打开无限画布
            </Link>
          </Button>
        </div>
      </section>
      <nav
        aria-label="创作能力"
        className="grid grid-cols-2 gap-3 py-6 md:grid-cols-4"
      >
        {capabilities.map(({ label, mode, icon: Icon }) => (
          <Link
            key={mode}
            href={`/create?mode=${mode}`}
            className="flex items-center justify-between rounded-xl bg-muted/50 px-5 py-5 text-sm transition-colors hover:bg-muted focus-visible:outline-2"
          >
            <span className="flex items-center gap-3">
              <Icon className="size-5" aria-hidden />
              {label}
            </span>
            <ArrowUpRight className="size-4" aria-hidden />
          </Link>
        ))}
      </nav>
      <section className="mt-8" aria-labelledby="recent-projects">
        <div className="mb-6 flex items-center justify-between">
          <h2 id="recent-projects" className="text-lg font-medium">
            最近项目
          </h2>
          <Link
            href="/projects"
            className="text-sm text-muted-foreground hover:text-foreground"
          >
            查看全部 →
          </Link>
        </div>
        {recent.isPending ? (
          <div className="grid gap-5 md:grid-cols-3">
            {Array.from({ length: 3 }, (_, i) => (
              <Skeleton key={i} className="h-40 rounded-xl" />
            ))}
          </div>
        ) : recent.isError ? (
          <Alert variant="destructive">
            <AlertTitle>项目读取失败</AlertTitle>
            <AlertDescription>
              {recent.error.message}
              <Button variant="ghost" onClick={() => void recent.refetch()}>
                重试
              </Button>
            </AlertDescription>
          </Alert>
        ) : recent.data.items.length ? (
          <div className="grid gap-5 md:grid-cols-3">
            {recent.data.items.map((project) => (
              <Link
                href={`/projects/${project.id}/canvas`}
                key={project.id}
                className="group flex flex-col gap-4 rounded-xl bg-muted/40 p-5 transition-colors hover:bg-muted focus-visible:outline-2"
              >
                <div className="flex h-24 items-center justify-center rounded-lg bg-background">
                  <Clapperboard
                    className="size-8 text-muted-foreground"
                    aria-hidden
                  />
                </div>
                <div className="flex items-center justify-between">
                  <span className="truncate font-medium">{project.name}</span>
                  <ArrowUpRight
                    className="size-4 text-muted-foreground"
                    aria-hidden
                  />
                </div>
                <p className="text-xs text-muted-foreground">
                  {project.aspect_ratio} ·{" "}
                  {project.style_type === "realistic" ? "写实" : "风格化"}
                </p>
              </Link>
            ))}
          </div>
        ) : (
          <Link
            href="/projects?create=true"
            className="flex min-h-40 flex-col items-center justify-center gap-3 rounded-xl bg-muted/40 text-muted-foreground"
          >
            <Plus className="size-6" />
            <span>创建第一个项目</span>
          </Link>
        )}
      </section>
    </div>
  );
}
