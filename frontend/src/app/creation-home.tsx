"use client";

import {
  ArrowRight,
  Blocks,
  FolderPlus,
  LoaderCircle,
  Settings2,
  ShieldCheck,
  Sparkles,
} from "lucide-react";
import Link from "next/link";

import { ProjectServerCard } from "@/app/projects/project-server-card";
import { StudioShell } from "@/components/studio/studio-shell";
import { Alert, AlertDescription, AlertTitle } from "@/components/ui/alert";
import { Button } from "@/components/ui/button";
import { useAuthSessionState } from "@/hooks/use-auth-session";
import { appApiErrorMessage, useMeQuery, useProjectsQuery } from "@/lib/server-state";

const quickActions = [
  { label: "创建或管理项目", description: "固定画幅、风格与单集规格", icon: FolderPlus, href: "/projects" },
  { label: "准备项目资产", description: "管理角色、场景与声音版本", icon: Blocks, href: "/studio" },
  { label: "登记授权", description: "处理媒体与剧本的权利门禁", icon: ShieldCheck, href: "/governance" },
  { label: "账户与空间", description: "维护个人资料和工作空间", icon: Settings2, href: "/workspaces" },
];

export function CreationHome() {
  const sessionState = useAuthSessionState();
  const authenticated = sessionState === "authenticated";
  const me = useMeQuery(undefined, { skip: !authenticated });
  const projectsQuery = useProjectsQuery(me.data?.workspace.id ?? "", {
    skip: !me.data?.workspace.id,
  });
  const projects = projectsQuery.data?.items.filter((project) => project.status === "active").slice(0, 3) ?? [];

  if (sessionState === "checking") {
    return <div className="grid min-h-screen place-items-center"><LoaderCircle aria-label="正在读取登录状态" className="animate-spin text-[#079db3]" /></div>;
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
      <div className="mx-auto max-w-[1280px] px-5 py-10 md:px-8 lg:py-14">
        <section className="mx-auto max-w-4xl text-center">
          <div className="inline-flex items-center gap-2 rounded-full border border-cyan-100 bg-cyan-50/70 px-3 py-1.5 text-sm text-[#087f91]"><Sparkles className="size-4" aria-hidden="true" />AI 漫剧生产工作流</div>
          <h1 className="mt-6 text-4xl leading-tight font-semibold tracking-[-0.04em] md:text-5xl">从剧本到可复用资产，按真实阶段推进每一集</h1>
          <p className="mx-auto mt-4 max-w-2xl text-base leading-7 text-slate-500">Lanverse 把项目、不可变剧本版本、结构提取、媒体、授权与资产准备度组合在同一个生产入口。</p>
          <div className="mt-8 flex flex-wrap justify-center gap-3">
            <Button asChild size="lg"><Link href={authenticated ? "/projects" : "/register"}>{authenticated ? "进入项目库" : "创建账户"}<ArrowRight aria-hidden="true" /></Link></Button>
            {!authenticated ? <Button asChild size="lg" variant="outline"><Link href="/login">已有账户，登录</Link></Button> : null}
          </div>
        </section>

        <section className="mt-14 grid gap-3 sm:grid-cols-2 xl:grid-cols-4" aria-label="快速入口">
          {quickActions.map((item) => (
            <Link className="rounded-2xl border border-slate-200 bg-white p-5 transition hover:border-cyan-200 hover:shadow-md hover:shadow-cyan-950/5" href={item.href} key={item.href}>
              <item.icon className="size-5 text-[#079db3]" aria-hidden="true" />
              <h2 className="mt-4 font-medium">{item.label}</h2>
              <p className="mt-1 text-sm leading-6 text-slate-500">{item.description}</p>
            </Link>
          ))}
        </section>

        {authenticated ? (
          <section className="mt-14">
            <div className="mb-5 flex items-end justify-between gap-4">
              <div><h2 className="text-xl font-semibold">继续创作</h2><p className="mt-1 text-sm text-slate-500">读取当前工作空间最近更新的项目。</p></div>
              <Button asChild variant="ghost"><Link href="/projects">查看全部<ArrowRight aria-hidden="true" /></Link></Button>
            </div>
            {me.error || projectsQuery.error ? (
              <Alert variant="destructive"><AlertTitle>最近项目暂时无法读取</AlertTitle><AlertDescription>{appApiErrorMessage(me.error ?? projectsQuery.error)}</AlertDescription></Alert>
            ) : !projectsQuery.data ? (
              <div className="grid min-h-40 place-items-center"><LoaderCircle aria-label="正在加载最近项目" className="animate-spin text-[#079db3]" /></div>
            ) : projects.length ? (
              <div className="grid gap-5 md:grid-cols-3">{projects.map((project) => <ProjectServerCard key={project.id} project={project} />)}</div>
            ) : (
              <div className="rounded-2xl border border-dashed border-slate-200 bg-white px-6 py-12 text-center"><p className="font-medium">还没有制作中的项目</p><Button asChild className="mt-4"><Link href="/projects">创建第一个项目</Link></Button></div>
            )}
          </section>
        ) : null}
      </div>
    </StudioShell>
  );
}
