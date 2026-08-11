"use client";

import {
  ArrowRight,
  AudioLines,
  CircleCheck,
  CirclePlay,
  Clapperboard,
  FileText,
  LoaderCircle,
  Upload,
  UsersRound,
} from "lucide-react";
import Image from "next/image";
import Link from "next/link";

import { StudioShell } from "@/components/studio/studio-shell";
import { Alert, AlertDescription, AlertTitle } from "@/components/ui/alert";
import { Badge } from "@/components/ui/badge";
import { Button } from "@/components/ui/button";
import { Separator } from "@/components/ui/separator";
import {
  Table,
  TableBody,
  TableCell,
  TableHead,
  TableHeader,
  TableRow,
} from "@/components/ui/table";
import { useAuthSessionState } from "@/hooks/use-auth-session";
import { appApiErrorMessage, useMeQuery, useProjectsQuery } from "@/lib/server-state";

const productionStages = [
  { label: "剧本解析", detail: "场次、对白与实体", icon: FileText },
  { label: "视觉开发", detail: "角色、场景与风格", icon: UsersRound },
  { label: "镜头生产", detail: "分镜、候选与选用", icon: Clapperboard },
  { label: "声音时间线", detail: "声音、字幕与编排", icon: AudioLines },
  { label: "审核交付", detail: "返工、标识与成片", icon: CirclePlay },
];

export function CreationHome() {
  const sessionState = useAuthSessionState();
  const authenticated = sessionState === "authenticated";
  const me = useMeQuery(undefined, { skip: !authenticated });
  const projectsQuery = useProjectsQuery(me.data?.workspace.id ?? "", {
    skip: !me.data?.workspace.id,
  });
  const projects = projectsQuery.data?.items.filter((project) => project.status === "active").slice(0, 4) ?? [];
  const currentProject = projects[0];

  if (sessionState === "checking") {
    return (
      <div className="grid min-h-screen place-items-center">
        <LoaderCircle aria-label="正在读取登录状态" className="animate-spin text-primary" />
      </div>
    );
  }

  return (
    <StudioShell
      active="create"
      topAction={authenticated ? <Button asChild><Link href="/projects">全部项目<ArrowRight aria-hidden="true" /></Link></Button> : <Button asChild><Link href="/login">登录</Link></Button>}
      viewer={me.data ? {
        displayName: me.data.user.display_name?.trim() || me.data.user.email,
        workspaceName: me.data.workspace.name,
      } : undefined}
    >
      <div className="mx-auto max-w-[1440px] px-5 md:px-8">
        <section className="grid min-h-[560px] items-center gap-12 border-b py-14 lg:grid-cols-[1fr_0.6fr] lg:py-16 xl:grid-cols-[0.95fr_0.62fr_1.15fr]">
          <div>
            <p className="text-sm font-medium">AI 竖屏短剧生产系统</p>
            <h1 className="mt-5 max-w-xl text-5xl leading-[1.04] font-semibold tracking-[-0.055em] sm:text-6xl lg:text-5xl xl:text-6xl">
              把剧本，变成<br />可追踪的成片。
            </h1>
            <p className="mt-6 max-w-lg text-base leading-7 text-muted-foreground">
              从固定剧本版本到 9:16 成片，每个资产、镜头、候选、费用与审核决定都可确认、可返工、可追溯。
            </p>
            <div className="mt-8 flex flex-wrap gap-3">
              <Button asChild className="h-11 px-5" size="lg">
                <Link href={authenticated ? "/projects" : "/register"}><Upload aria-hidden="true" />导入剧本</Link>
              </Button>
              <Button asChild className="h-11 px-5" size="lg" variant="outline">
                <Link href={authenticated ? (currentProject ? `/projects/${currentProject.id}` : "/projects") : "/login"}>
                  <CirclePlay aria-hidden="true" />继续制作
                </Link>
              </Button>
            </div>
            <Link className="mt-6 inline-flex items-center gap-2 text-sm text-muted-foreground hover:text-foreground" href="/projects">
              查看真实生产阶段<ArrowRight className="size-4" aria-hidden="true" />
            </Link>
          </div>

          <div className="mx-auto w-full max-w-[280px]">
            <div className="relative aspect-[9/16] overflow-hidden rounded-xl bg-muted">
              <Image
                alt="长安夜航项目封面"
                className="object-cover grayscale"
                fill
                priority
                sizes="280px"
                src="/assets/lanverse-studio/changan-night-cover.png"
              />
              <div className="absolute inset-x-0 bottom-0 bg-black/75 p-5 text-white">
                <p className="font-mono text-xs text-white/60">9:16 · EPISODE</p>
                <p className="mt-1 text-lg font-medium">从故事到交付</p>
              </div>
            </div>
            <p className="mt-2 text-center font-mono text-xs text-muted-foreground">1080 × 1920</p>
          </div>

          <div className="lg:col-span-2 xl:col-span-1">
            <p className="text-sm font-medium">一条可以恢复的生产链</p>
            <div className="mt-6 divide-y">
              {productionStages.map((stage, index) => (
                <div className="grid grid-cols-[2rem_1fr_auto] items-center gap-3 py-4" key={stage.label}>
                  <span className="font-mono text-xs text-muted-foreground">0{index + 1}</span>
                  <div>
                    <div className="flex items-center gap-2 text-sm font-medium"><stage.icon className="size-4" aria-hidden="true" />{stage.label}</div>
                    <p className="mt-1 text-sm text-muted-foreground">{stage.detail}</p>
                  </div>
                  <CircleCheck className="size-4 text-muted-foreground" aria-hidden="true" />
                </div>
              ))}
            </div>
            <p className="mt-5 text-xs leading-5 text-muted-foreground">AI 结果先成为候选，只有人工确认后才进入下游事实。</p>
          </div>
        </section>

        {authenticated ? (
          <>
            {me.error || projectsQuery.error ? (
              <Alert className="my-10" variant="destructive">
                <AlertTitle>项目事实暂时无法读取</AlertTitle>
                <AlertDescription>{appApiErrorMessage(me.error ?? projectsQuery.error)}</AlertDescription>
              </Alert>
            ) : !projectsQuery.data ? (
              <div className="grid min-h-64 place-items-center"><LoaderCircle aria-label="正在加载最近项目" className="animate-spin" /></div>
            ) : currentProject ? (
              <>
                <section className="grid gap-8 border-b py-10 lg:grid-cols-[1.15fr_0.8fr_1fr_auto] lg:items-center" aria-label="当前项目下一步">
                  <div>
                    <p className="text-xs font-medium text-muted-foreground">当前项目 · 下一步</p>
                    <div className="mt-3 flex items-center gap-3">
                      <h2 className="text-2xl font-semibold tracking-tight">{currentProject.name}</h2>
                      <Badge variant="secondary">制作中</Badge>
                    </div>
                    <p className="mt-2 text-sm text-muted-foreground">{currentProject.description || "继续完成当前单集的真实生产阶段。"}</p>
                  </div>
                  <div>
                    <p className="text-xs text-muted-foreground">目标规格</p>
                    <p className="mt-2 font-mono text-lg">{currentProject.aspect_ratio} · {Math.round(currentProject.target_duration_ms / 1_000)}s</p>
                  </div>
                  <div>
                    <p className="text-xs text-muted-foreground">视觉风格</p>
                    <p className="mt-2 text-sm font-medium">{currentProject.visual_style || "等待项目设定"}</p>
                  </div>
                  <Button asChild><Link href={`/projects/${currentProject.id}`}>处理下一步<ArrowRight aria-hidden="true" /></Link></Button>
                </section>

                <section className="py-10">
                  <div className="flex items-end justify-between gap-4">
                    <div><h2 className="text-xl font-semibold">最近项目</h2><p className="mt-1 text-sm text-muted-foreground">继续工作空间内仍在制作的项目。</p></div>
                    <Button asChild variant="ghost"><Link href="/projects">查看全部<ArrowRight aria-hidden="true" /></Link></Button>
                  </div>
                  <Separator className="mt-5" />
                  <Table>
                    <TableHeader>
                      <TableRow><TableHead>项目</TableHead><TableHead>规格</TableHead><TableHead>风格</TableHead><TableHead>状态</TableHead><TableHead className="text-right">操作</TableHead></TableRow>
                    </TableHeader>
                    <TableBody>
                      {projects.map((project) => (
                        <TableRow key={project.id}>
                          <TableCell><div className="font-medium">{project.name}</div><div className="mt-1 max-w-md truncate text-xs text-muted-foreground">{project.description || "尚未填写项目简介"}</div></TableCell>
                          <TableCell className="font-mono text-xs">{project.aspect_ratio} · {Math.round(project.target_duration_ms / 1_000)}s</TableCell>
                          <TableCell>{project.visual_style || "未设置"}</TableCell>
                          <TableCell><span className="inline-flex items-center gap-2"><span className="size-1.5 rounded-full bg-foreground" />制作中</span></TableCell>
                          <TableCell className="text-right"><Button asChild size="sm" variant="ghost"><Link aria-label={`打开项目 ${project.name}`} href={`/projects/${project.id}`}>打开<ArrowRight aria-hidden="true" /></Link></Button></TableCell>
                        </TableRow>
                      ))}
                    </TableBody>
                  </Table>
                </section>
              </>
            ) : (
              <section className="py-16 text-center">
                <h2 className="text-xl font-semibold">从第一份剧本开始</h2>
                <p className="mt-2 text-sm text-muted-foreground">创建项目和单集后，再导入不可变剧本版本。</p>
                <Button asChild className="mt-5"><Link href="/projects">创建第一个项目</Link></Button>
              </section>
            )}
          </>
        ) : null}
      </div>
    </StudioShell>
  );
}
