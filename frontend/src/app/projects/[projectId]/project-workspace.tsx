"use client";

import {
  AlertCircle,
  ArrowRight,
  Check,
  ChevronRight,
  Clock3,
  FileText,
  Film,
  LayoutTemplate,
  MoreHorizontal,
  Play,
  Plus,
  Users,
} from "lucide-react";
import Image from "next/image";
import Link from "next/link";
import { useState } from "react";

import { StudioShell } from "@/components/studio/studio-shell";
import { Badge } from "@/components/ui/badge";
import { Button } from "@/components/ui/button";
import { mockEpisodes, mockProjects } from "@/lib/mock-studio-data";

const overviewItems = [
  { label: "剧本", value: "第 08 集 v4", detail: "已确认", icon: FileText, ready: true },
  { label: "角色与场景", value: "18 项资产", detail: "2 项待确认", icon: Users, warning: true },
  { label: "分镜", value: "18 / 26 镜头", detail: "69% 就绪", icon: LayoutTemplate },
  { label: "生成素材", value: "42 个候选", detail: "6 个处理中", icon: Film },
];

export function ProjectWorkspace({ projectId }: { projectId: string }) {
  const project = mockProjects.find((item) => item.id === projectId) ?? mockProjects[0];
  const [activeTab, setActiveTab] = useState("制作概览");
  const [episodeCreated, setEpisodeCreated] = useState(false);

  return (
    <StudioShell
      active="projects"
      currentStep={1}
      projectName={project.name}
      topAction={<Button asChild className="h-10 bg-[#079db3] px-4 text-white hover:bg-[#078da0]"><Link href="/studio">继续制作<ArrowRight aria-hidden="true" /></Link></Button>}
    >
      <div className="mx-auto max-w-[1280px] px-5 py-8 md:px-8">
        <div className="flex flex-wrap items-start gap-6">
          <div className="relative aspect-[3/4] w-32 shrink-0 overflow-hidden rounded-2xl border border-slate-200 bg-slate-100 shadow-sm md:w-40"><Image alt={`${project.name}封面`} fill priority sizes="160px" src={project.cover} className="object-cover" /></div>
          <div className="min-w-0 flex-1 py-1">
            <div className="flex flex-wrap items-center gap-2"><Badge className="border-cyan-100 bg-cyan-50 text-[#087f91]" variant="outline">{project.currentStage}</Badge><Badge variant="outline">{project.style}</Badge><Badge variant="outline">{project.ratio}</Badge></div>
            <h1 className="mt-4 text-3xl font-semibold tracking-[-0.035em]">{project.name}</h1>
            <p className="mt-2 max-w-2xl text-sm leading-6 text-slate-500">{project.tagline}</p>
            <div className="mt-6 flex flex-wrap gap-7 text-sm"><div><p className="font-semibold">第 {project.currentEpisode} 集</p><p className="mt-1 text-xs text-slate-500">当前制作</p></div><div><p className="font-semibold">{project.episodes} 集</p><p className="mt-1 text-xs text-slate-500">总计划</p></div><div><p className="font-semibold">{project.progress}%</p><p className="mt-1 text-xs text-slate-500">项目进度</p></div></div>
          </div>
          <Button aria-label="更多项目操作" size="icon" variant="outline"><MoreHorizontal aria-hidden="true" /></Button>
        </div>

        <div className="mt-8 flex gap-7 border-b border-slate-200">
          {["制作概览", "单集管理", "生成任务", "交付版本"].map((tab) => <button className={`relative pb-3 text-sm ${activeTab === tab ? "font-medium text-[#078fa5]" : "text-slate-500"}`} key={tab} onClick={() => setActiveTab(tab)} type="button">{tab}{activeTab === tab ? <span className="absolute inset-x-0 bottom-0 h-0.5 rounded-full bg-[#079db3]" /> : null}</button>)}
        </div>

        {activeTab === "制作概览" ? (
          <div className="mt-6 grid gap-6 xl:grid-cols-[minmax(0,1fr)_340px]">
            <section>
              <div className="grid gap-3 sm:grid-cols-2">
                {overviewItems.map((item) => (
                  <button className="flex items-center gap-4 rounded-2xl border border-slate-200 bg-white p-4 text-left transition hover:border-cyan-200" key={item.label} type="button">
                    <span className="grid size-11 shrink-0 place-items-center rounded-xl bg-slate-100 text-[#078fa5]"><item.icon className="size-5" aria-hidden="true" /></span>
                    <span className="min-w-0 flex-1"><span className="block text-xs text-slate-500">{item.label}</span><span className="mt-1 block truncate font-medium">{item.value}</span></span>
                    <span className={`text-xs ${item.warning ? "text-amber-600" : item.ready ? "text-emerald-600" : "text-slate-500"}`}>{item.detail}</span>
                  </button>
                ))}
              </div>

              <div className="mt-6 rounded-2xl border border-slate-200 bg-white">
                <div className="flex items-center justify-between border-b border-slate-100 px-5 py-4"><div><h2 className="font-semibold">最近单集</h2><p className="mt-1 text-xs text-slate-500">按制作顺序排列</p></div><Button size="sm" variant="outline" onClick={() => setEpisodeCreated(true)}><Plus aria-hidden="true" />创建单集</Button></div>
                {episodeCreated ? <div className="border-b border-emerald-100 bg-emerald-50 px-5 py-3 text-sm text-emerald-800" role="status">第 09 集草稿已创建。</div> : null}
                <div className="divide-y divide-slate-100">
                  {mockEpisodes.map((episode) => (
                    <button className="grid w-full grid-cols-[auto_1fr_auto] items-center gap-4 px-5 py-4 text-left transition hover:bg-slate-50" key={episode.id} type="button">
                      <span className="grid size-10 place-items-center rounded-xl bg-slate-100 text-sm font-semibold">{String(episode.index).padStart(2, "0")}</span>
                      <span><span className="block font-medium">{episode.title}</span><span className="mt-1 flex flex-wrap gap-3 text-xs text-slate-500"><span>{episode.duration}</span><span>{episode.shots} 镜头</span><span>{episode.ready}/{episode.shots} 就绪</span></span></span>
                      <span className="flex items-center gap-3"><Badge variant={episode.status === "已交付" ? "secondary" : "outline"}>{episode.status}</Badge><ChevronRight className="size-4 text-slate-400" aria-hidden="true" /></span>
                    </button>
                  ))}
                </div>
              </div>
            </section>

            <aside className="grid content-start gap-5">
              <div className="rounded-2xl border border-amber-200 bg-amber-50/70 p-5">
                <div className="flex items-center gap-2"><AlertCircle className="size-5 text-amber-500" aria-hidden="true" /><h2 className="font-semibold">下一步</h2></div>
                <p className="mt-3 text-sm font-medium">确认 2 项角色资产变更</p>
                <p className="mt-2 text-sm leading-6 text-slate-600">顾清禾 v3 已影响第 08 集的 2 个分镜镜头，确认后即可继续生成。</p>
                <Button asChild className="mt-5 h-10 w-full bg-[#079db3] text-white hover:bg-[#078da0]"><Link href="/studio">前往资产库<ArrowRight aria-hidden="true" /></Link></Button>
              </div>
              <div className="rounded-2xl border border-slate-200 bg-white p-5">
                <div className="flex items-center justify-between"><h2 className="font-semibold">任务动态</h2><button className="text-xs text-[#078fa5]" type="button">全部任务</button></div>
                <div className="mt-4 grid gap-4 text-sm">
                  <div className="flex gap-3"><span className="mt-0.5 grid size-6 place-items-center rounded-full bg-emerald-50 text-emerald-600"><Check className="size-3.5" aria-hidden="true" /></span><div><p>镜头 12 候选已生成</p><p className="mt-1 text-xs text-slate-400">8 分钟前</p></div></div>
                  <div className="flex gap-3"><span className="mt-0.5 grid size-6 place-items-center rounded-full bg-cyan-50 text-[#079db3]"><Play className="size-3.5" aria-hidden="true" /></span><div><p>镜头 13 正在生成</p><p className="mt-1 text-xs text-slate-400">预计还需 2 分钟</p></div></div>
                  <div className="flex gap-3"><span className="mt-0.5 grid size-6 place-items-center rounded-full bg-slate-100 text-slate-500"><Clock3 className="size-3.5" aria-hidden="true" /></span><div><p>声音合成等待输入</p><p className="mt-1 text-xs text-slate-400">阻塞于台词确认</p></div></div>
                </div>
              </div>
            </aside>
          </div>
        ) : (
          <div className="mt-10 rounded-2xl border border-dashed border-slate-300 bg-white px-6 py-20 text-center"><p className="font-medium">{activeTab}</p><p className="mt-2 text-sm text-slate-500">此区域使用 Mock 数据展示，后续接入相应业务模块。</p></div>
        )}
      </div>
    </StudioShell>
  );
}
