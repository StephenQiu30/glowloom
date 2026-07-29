"use client";

import { ArrowRight, FileUp, ImagePlus, LayoutTemplate, PenLine, Sparkles, WandSparkles } from "lucide-react";
import Link from "next/link";
import { useState } from "react";

import { ProjectCard } from "@/components/studio/project-card";
import { StudioShell } from "@/components/studio/studio-shell";
import { Button } from "@/components/ui/button";
import { mockProjects } from "@/lib/mock-studio-data";

const quickActions = [
  { label: "导入剧本", icon: FileUp },
  { label: "角色定妆", icon: ImagePlus },
  { label: "智能分镜", icon: LayoutTemplate },
  { label: "续写故事", icon: PenLine },
];

export default function Home() {
  const [prompt, setPrompt] = useState("");
  const [created, setCreated] = useState(false);

  return (
    <StudioShell active="create" topAction={<Button asChild className="h-10 bg-[#079db3] px-4 text-white hover:bg-[#078da0]"><Link href="/projects">全部项目<ArrowRight aria-hidden="true" /></Link></Button>}>
      <div className="mx-auto max-w-[1280px] px-5 py-10 md:px-8 lg:py-14">
        <section className="mx-auto max-w-5xl text-center">
          <div className="inline-flex items-center gap-2 rounded-full border border-cyan-100 bg-cyan-50/70 px-3 py-1.5 text-sm text-[#087f91]"><Sparkles className="size-4" aria-hidden="true" />AI 漫剧工作流已就绪</div>
          <h1 className="mt-6 text-4xl leading-tight font-semibold tracking-[-0.04em] md:text-5xl">今天，想把什么故事变成漫剧？</h1>
          <p className="mx-auto mt-4 max-w-2xl text-base leading-7 text-slate-500">从一句灵感或完整剧本开始，Lanverse 会帮你组织角色、场景、分镜与生成任务。</p>

          <div className="mt-9 rounded-3xl border border-slate-200 bg-white p-4 text-left shadow-xl shadow-slate-950/[0.035]">
            <textarea
              aria-label="漫剧创作描述"
              className="min-h-32 w-full resize-none bg-transparent px-3 py-2 text-base leading-7 outline-none placeholder:text-slate-400"
              placeholder="输入故事灵感，或粘贴一段剧本开始创作"
              value={prompt}
              onChange={(event) => setPrompt(event.target.value)}
            />
            <div className="mt-3 flex flex-wrap items-center gap-2 border-t border-slate-100 pt-4">
              {['AI 导演', '9:16 竖屏', '水墨幻想'].map((item) => <button className="rounded-lg border border-slate-200 bg-white px-3 py-2 text-sm text-slate-600 transition hover:bg-slate-50" key={item} type="button">{item}</button>)}
              <Button className="ml-auto h-10 bg-[#079db3] px-5 text-white hover:bg-[#078da0]" disabled={!prompt.trim()} onClick={() => setCreated(true)}><WandSparkles aria-hidden="true" />开始创作</Button>
            </div>
          </div>
          {created ? <p className="mt-3 text-sm text-emerald-700" role="status">项目草稿已生成，下一步可以确认角色与视觉风格。</p> : null}

          <div className="mt-4 grid grid-cols-2 gap-3 text-left md:grid-cols-4">
            {quickActions.map((item) => (
              <button className="flex items-center justify-center gap-2 rounded-xl border border-slate-200 bg-white px-4 py-3 text-sm text-slate-600 transition hover:border-cyan-200 hover:text-[#078fa5]" key={item.label} type="button"><item.icon className="size-4" aria-hidden="true" />{item.label}</button>
            ))}
          </div>
        </section>

        <section className="mt-16">
          <div className="mb-5 flex items-end justify-between gap-4">
            <div><h2 className="text-xl font-semibold">继续创作</h2><p className="mt-1 text-sm text-slate-500">回到最近更新的项目。</p></div>
            <Button asChild variant="ghost"><Link href="/projects">查看全部<ArrowRight aria-hidden="true" /></Link></Button>
          </div>
          <div className="grid gap-5 md:grid-cols-3">{mockProjects.map((project) => <ProjectCard compact key={project.id} project={project} />)}</div>
        </section>
      </div>
    </StudioShell>
  );
}
